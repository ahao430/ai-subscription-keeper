// Package zhipu implements Zhipu GLM Coding Plan. Chat/models ride on the
// OpenAI-compatible coding endpoint; quota uses the monitor API that the
// official glm-plan-usage plugin consumes:
//
//	GET {monitor}/api/monitor/usage/quota/limit   Authorization: <raw token>
//	→ {"data":{"limits":[{"type":"TOKENS_LIMIT","percentage":18,...},...]}}
package zhipu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/openai"
)

// Endpoints picks between 智谱国内 and 智谱海外 (z.ai).
type Endpoints struct {
	CodingBase   string // OpenAI-compatible base, e.g. https://open.bigmodel.cn/api/coding/paas/v4
	AnthropicBase string // Anthropic-compatible base, for model listing fallback
	MonitorBase  string // e.g. https://open.bigmodel.cn
}

var defaultCodingModels = []string{
	"glm-4.6", "glm-4.5", "glm-4.5-air", "glm-4.5-flash",
}

type Zhipu struct {
	ep   Endpoints
	cred *provider.Credential
	hc   provider.HTTPClient
	oa   *openai.Client
}

func New(ep Endpoints, cred *provider.Credential, hc provider.HTTPClient) *Zhipu {
	oaOpts := openai.Options{
		DefaultBaseURL: ep.CodingBase,
		FallbackModels: defaultCodingModels,
	}
	// Zhipu coding endpoints reject unknown stream_options in some builds.
	return &Zhipu{ep: ep, cred: cred, hc: hc, oa: openai.New(oaOpts, cred, hc)}
}

func (z *Zhipu) GetModels(ctx context.Context) ([]provider.Model, error) {
	models, err := z.oa.GetModels(ctx)
	if err == nil && len(models) > 0 {
		return models, nil
	}
	// Coding endpoints may not expose /models; try the Anthropic-compatible one.
	models, err2 := z.anthropicModels(ctx)
	if err2 == nil && len(models) > 0 {
		return models, nil
	}
	if err == nil && err2 != nil && len(z.oa.BaseURL()) > 0 {
		// Fall back to the curated list so 预热/测试 can still be configured.
		out := make([]provider.Model, 0, len(defaultCodingModels))
		for _, id := range defaultCodingModels {
			out = append(out, provider.Model{ID: id})
		}
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, err2
}

func (z *Zhipu) anthropicModels(ctx context.Context) ([]provider.Model, error) {
	base := z.ep.AnthropicBase
	if z.cred.BaseURL != "" {
		// A custom base most likely targets the OpenAI-compatible endpoint.
		base = strings.TrimRight(z.cred.BaseURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+z.cred.APIKey)
	resp, err := z.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	models := make([]provider.Model, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			models = append(models, provider.Model{ID: m.ID})
		}
	}
	return models, nil
}

func (z *Zhipu) SupportsQuota() bool { return true }

type quotaResponse struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Data    struct {
		Level  string       `json:"level"`
		Limits []quotaLimit `json:"limits"`
	} `json:"data"`
}

// quotaLimit is one entry of data.limits. Window semantics follow the
// official monitor API: type TOKENS_LIMIT/CREDIT_LIMIT with unit 3 = 5 小时
// 滚动窗、unit 6 = 每周窗；TIME_LIMIT 是 MCP 工具月度窗口。
type quotaLimit struct {
	Type          string      `json:"type"`
	Unit          *int        `json:"unit"`
	Percentage    json.Number `json:"percentage"`
	NextResetTime json.Number `json:"nextResetTime"`
}

func (z *Zhipu) GetQuota(ctx context.Context) (*provider.QuotaInfo, *provider.RawResponse, error) {
	monitor := z.ep.MonitorBase
	if z.cred.QuotaURL != "" {
		monitor = strings.TrimRight(z.cred.QuotaURL, "/")
	}
	if z.cred.APIKey == "" {
		return nil, nil, fmt.Errorf("未配置令牌")
	}
	url := monitor + "/api/monitor/usage/quota/limit"
	// The monitor API expects the raw token in Authorization (no Bearer).
	raw, body, err := z.oa.GetQuotaRawWithHeaders(ctx, url, z.cred.APIKey, nil)
	if err != nil {
		return nil, raw, err
	}
	info, err := parseZhipuQuota(body)
	return info, raw, err
}

// parseZhipuQuota validates the business envelope and converts limits into
// the unified quota model.
func parseZhipuQuota(body []byte) (*provider.QuotaInfo, error) {
	var qr quotaResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, fmt.Errorf("解析额度响应失败: %w", err)
	}
	// The monitor API signals auth/business failures inside a HTTP 200.
	if !qr.Success || (qr.Code != 0 && qr.Code != 200) {
		msg := qr.Msg
		if msg == "" {
			msg = fmt.Sprintf("接口返回 code=%d", qr.Code)
		}
		return nil, errors.New(msg)
	}
	info := &provider.QuotaInfo{}
	seen := map[string]bool{}
	for _, l := range qr.Data.Limits {
		code, display := classifyZhipuLimit(strings.ToUpper(l.Type), l.Unit, seen)
		if seen[code] {
			continue // 同一窗口只保留第一条
		}
		seen[code] = true
		dim := provider.QuotaDimension{Code: code, DisplayName: display}
		if pct, ok := numberToFloat(l.Percentage); ok {
			p := pct
			dim.UsedPercentage = &p
		}
		if t, ok := epochToTime(l.NextResetTime); ok {
			dim.ResetAt = &t
		}
		info.Dimensions = append(info.Dimensions, dim)
	}
	if qr.Data.Level != "" {
		info.Extra = append(info.Extra, provider.QuotaMetric{Label: "套餐等级", Value: qr.Data.Level})
	}
	return info, nil
}

// classifyZhipuLimit anchors the window on `unit` (3=五小时, 6=每周) since the
// same type can appear for both windows; old single-limit plans fall back to
// positional assignment.
func classifyZhipuLimit(t string, unit *int, seen map[string]bool) (string, string) {
	switch {
	case t == "TIME_LIMIT":
		return "mcp_monthly", "MCP月限额"
	case unit != nil && *unit == 3:
		return "5h", "5小时限额"
	case unit != nil && *unit == 6:
		return "weekly", "周限额"
	default:
		if !seen["5h"] {
			return "5h", "5小时限额"
		}
		return "weekly", "周限额"
	}
}

func (z *Zhipu) ChatStream(ctx context.Context, req provider.ModelRequest, onDelta provider.DeltaFunc) (*provider.StreamResult, error) {
	return z.oa.ChatStream(ctx, req, onDelta)
}

func numberToFloat(n json.Number) (float64, bool) {
	if n == "" {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

// epochToTime accepts millisecond or second epochs.
func epochToTime(n json.Number) (time.Time, bool) {
	f, ok := numberToFloat(n)
	if !ok || f <= 0 {
		return time.Time{}, false
	}
	if f >= 1e11 { // millisecond scale
		return time.UnixMilli(int64(f)).UTC(), true
	}
	return time.Unix(int64(f), 0).UTC(), true
}
