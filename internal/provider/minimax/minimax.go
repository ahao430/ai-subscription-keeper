// Package minimax implements MiniMax Coding Plan. Chat rides the
// Anthropic-compatible endpoint; quota uses the OpenAPI remains endpoint:
//
//	GET https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains
//	Authorization: Bearer <token>
//	→ model_remains[] 中 model_name=="general" 的条目：
//	  current_interval_remaining_percent（5h 剩余%）、end_time（ms）、
//	  current_weekly_status==1 时周桶 current_weekly_remaining_percent/weekly_end_time
package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/claude"
	"ai-subscription-keeper/internal/provider/openai"
)

const (
	defaultAPIBase  = "https://api.minimaxi.com/anthropic"
	defaultQuotaURL = "https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains"
)

type Client struct {
	cred *provider.Credential
	cla  *claude.Client
	oa   *openai.Client
}

func New(cred *provider.Credential, hc provider.HTTPClient) provider.Provider {
	// MiniMax coding plan speaks the Anthropic protocol with a Bearer token.
	bearer := *cred
	if bearer.AuthMode == "" {
		bearer.AuthMode = "bearer"
	}
	if bearer.BaseURL == "" {
		bearer.BaseURL = defaultAPIBase
	}
	cla := claude.NewWithOptions(&bearer, hc, claude.Options{
		FallbackModels: []string{"MiniMax-M2.1", "MiniMax-M2", "MiniMax-M1"},
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
	url := defaultQuotaURL
	if c.cred.QuotaURL != "" {
		url = c.cred.QuotaURL
	}
	raw, body, err := c.oa.GetQuotaRawWithHeaders(ctx, url, "Bearer "+c.cred.APIKey, nil)
	if err != nil {
		return nil, raw, err
	}
	info, err := parseMiniMaxRemains(body)
	return info, raw, err
}

type remainsEntry struct {
	ModelName                         string   `json:"model_name"`
	CurrentIntervalRemainingPercent   *float64 `json:"current_interval_remaining_percent"`
	EndTime                           int64    `json:"end_time"`
	CurrentWeeklyStatus               *int     `json:"current_weekly_status"`
	CurrentWeeklyRemainingPercent     *float64 `json:"current_weekly_remaining_percent"`
	WeeklyEndTime                     int64    `json:"weekly_end_time"`
}

type remainsResponse struct {
	ModelRemains []remainsEntry `json:"model_remains"`
}

// parseMiniMaxRemains converts the remains payload into quota dimensions.
// MiniMax signals failures via a base_resp envelope inside HTTP 200; payload
// may also be wrapped in a data envelope, so both shapes are tried.
func parseMiniMaxRemains(body []byte) (*provider.QuotaInfo, error) {
	var envelope struct {
		BaseResp *struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.BaseResp != nil && envelope.BaseResp.StatusCode != 0 {
		msg := envelope.BaseResp.StatusMsg
		if msg == "" {
			msg = fmt.Sprintf("接口返回 status_code=%d", envelope.BaseResp.StatusCode)
		}
		return nil, errors.New(msg)
	}
	var rr remainsResponse
	if err := json.Unmarshal(body, &rr); err != nil || len(rr.ModelRemains) == 0 {
		var wrapped struct {
			Data remainsResponse `json:"data"`
		}
		if werr := json.Unmarshal(body, &wrapped); werr != nil {
			return nil, fmt.Errorf("解析额度响应失败: %w", err)
		}
		rr = wrapped.Data
	}
	var general *remainsEntry
	for i := range rr.ModelRemains {
		if strings.EqualFold(rr.ModelRemains[i].ModelName, "general") {
			general = &rr.ModelRemains[i]
			break
		}
	}
	if general == nil {
		return nil, errors.New("响应中缺少 general 额度条目")
	}
	info := &provider.QuotaInfo{}
	if general.CurrentIntervalRemainingPercent != nil {
		dim := provider.QuotaDimension{
			Code:           "5h",
			DisplayName:    "5小时限额",
			UsedPercentage: ptr(clampUsed(100 - *general.CurrentIntervalRemainingPercent)),
		}
		if t, ok := epochMs(general.EndTime); ok {
			dim.ResetAt = &t
		}
		info.Dimensions = append(info.Dimensions, dim)
	}
	// 周桶仅在 current_weekly_status==1 时存在（3 等值表示无周限额）。
	if general.CurrentWeeklyStatus != nil && *general.CurrentWeeklyStatus == 1 {
		dim := provider.QuotaDimension{Code: "weekly", DisplayName: "周限额"}
		if general.CurrentWeeklyRemainingPercent != nil {
			dim.UsedPercentage = ptr(clampUsed(100 - *general.CurrentWeeklyRemainingPercent))
		}
		if t, ok := epochMs(general.WeeklyEndTime); ok {
			dim.ResetAt = &t
		}
		info.Dimensions = append(info.Dimensions, dim)
	}
	return info, nil
}

func clampUsed(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func ptr(v float64) *float64 { return &v }

func epochMs(ms int64) (time.Time, bool) {
	if ms <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(ms).UTC(), true
}
