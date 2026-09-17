// Package kimi implements Kimi For Coding. Chat rides the Anthropic-compatible
// endpoint (Bearer token); quota uses:
//
//	GET {base}/v1/usages     Authorization: Bearer <token>
//	→ { "limits": [ { "detail": { limit, remaining, resetTime } } ],   // 5 小时桶
//	    "usage":  { limit, remaining, resetTime } }                    // 周桶
//
// 已用百分比 = (limit - remaining) / limit × 100。数值可能是字符串或数字，
// resetTime 可能是秒/毫秒时间戳。
package kimi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/claude"
	"ai-subscription-keeper/internal/provider/openai"
)

const defaultBase = "https://api.kimi.com/coding"

type Client struct {
	cred *provider.Credential
	cla  *claude.Client
	oa   *openai.Client
}

func New(cred *provider.Credential, hc provider.HTTPClient) provider.Provider {
	bearer := *cred
	if bearer.AuthMode == "" {
		bearer.AuthMode = "bearer"
	}
	if bearer.BaseURL == "" {
		bearer.BaseURL = defaultBase
	}
	cla := claude.NewWithOptions(&bearer, hc, claude.Options{
		FallbackModels: []string{"kimi-k2.7-code", "kimi-k2-thinking-preview", "kimi-k2-turbo-preview"},
	})
	return &Client{cred: cred, cla: cla, oa: openai.New(openai.Options{}, cred, hc)}
}

func (c *Client) GetModels(ctx context.Context) ([]provider.Model, error) {
	return c.cla.GetModels(ctx)
}

func (c *Client) ChatStream(ctx context.Context, req provider.ModelRequest, onDelta provider.DeltaFunc) (*provider.StreamResult, error) {
	return c.cla.ChatStream(ctx, req, onDelta)
}

func (c *Client) SupportsQuota() bool { return true }

func (c *Client) GetQuota(ctx context.Context) (*provider.QuotaInfo, *provider.RawResponse, error) {
	if c.cred.APIKey == "" {
		return nil, nil, errors.New("未配置 API Token")
	}
	base := c.cred.BaseURL
	if base == "" {
		base = defaultBase
	}
	quotaURL := strings.TrimRight(base, "/") + "/v1/usages"
	if c.cred.QuotaURL != "" {
		quotaURL = c.cred.QuotaURL
	}
	raw, body, err := c.oa.GetQuotaRawWithHeaders(ctx, quotaURL, "Bearer "+c.cred.APIKey, nil)
	if err != nil {
		return nil, raw, err
	}
	info, err := parseKimiUsages(body)
	return info, raw, err
}

type usageBucket struct {
	Limit     json.RawMessage `json:"limit"`
	Remaining json.RawMessage `json:"remaining"`
	ResetTime json.RawMessage `json:"resetTime"`
}

type usagesResponse struct {
	Limits []struct {
		Detail usageBucket `json:"detail"`
	} `json:"limits"`
	Usage *usageBucket `json:"usage"`
}

// parseKimiUsages converts the usages payload into quota dimensions.
func parseKimiUsages(body []byte) (*provider.QuotaInfo, error) {
	var ur usagesResponse
	if err := json.Unmarshal(body, &ur); err != nil {
		return nil, fmt.Errorf("解析额度响应失败: %w", err)
	}
	info := &provider.QuotaInfo{}
	for _, l := range ur.Limits {
		if dim, ok := bucketToDim(&l.Detail, "5h", "5小时限额"); ok {
			info.Dimensions = append(info.Dimensions, dim)
			break
		}
	}
	if ur.Usage != nil {
		if dim, ok := bucketToDim(ur.Usage, "weekly", "周限额"); ok {
			info.Dimensions = append(info.Dimensions, dim)
		}
	}
	if len(info.Dimensions) == 0 {
		return nil, errors.New("响应中未找到额度数据")
	}
	return info, nil
}

func bucketToDim(b *usageBucket, code, display string) (provider.QuotaDimension, bool) {
	limit, ok1 := rawToFloat(b.Limit)
	remaining, ok2 := rawToFloat(b.Remaining)
	if !ok1 || !ok2 || limit <= 0 {
		return provider.QuotaDimension{}, false
	}
	used := (limit - remaining) / limit * 100
	if used < 0 {
		used = 0
	}
	if used > 100 {
		used = 100
	}
	dim := provider.QuotaDimension{Code: code, DisplayName: display, UsedPercentage: &used}
	if t, ok := rawToTime(b.ResetTime); ok {
		dim.ResetAt = &t
	}
	return dim, true
}

// rawToFloat accepts JSON numbers or numeric strings.
func rawToFloat(raw json.RawMessage) (float64, bool) {
	s := strings.TrimSpace(string(raw))
	s = strings.Trim(s, `"`)
	if s == "" || s == "null" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// rawToTime accepts second/millisecond epochs (number or string) and RFC3339.
func rawToTime(raw json.RawMessage) (time.Time, bool) {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" || s == "0" {
		return time.Time{}, false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if f <= 0 {
			return time.Time{}, false
		}
		if f < 1e12 { // seconds
			return time.Unix(int64(f), 0).UTC(), true
		}
		return time.UnixMilli(int64(f)).UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
