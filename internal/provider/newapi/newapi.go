// Package newapi implements NewAPI/OneAPI style gateways. Chat and models use
// the OpenAI-compatible API with the sk- key; quota uses the console API:
//
//	GET {base}/api/user/self
//	Authorization: <system token>    New-Api-User: <user id>
//	→ data.quota / data.used_quota / data.request_count  (500000 units = $1)
package newapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/openai"
)

const quotaPerUSD = 500000.0

type Client struct {
	cred *provider.Credential
	oa   *openai.Client
}

func New(cred *provider.Credential, hc provider.HTTPClient) *Client {
	return &Client{cred: cred, oa: openai.New(openai.Options{IncludeStreamUsage: true}, cred, hc)}
}

func (c *Client) GetModels(ctx context.Context) ([]provider.Model, error) {
	return c.oa.GetModels(ctx)
}

func (c *Client) ChatStream(ctx context.Context, req provider.ModelRequest, onDelta provider.DeltaFunc) (*provider.StreamResult, error) {
	return c.oa.ChatStream(ctx, req, onDelta)
}

func (c *Client) SupportsQuota() bool { return true }

type selfResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ID           int64  `json:"id"`
		Username     string `json:"username"`
		Quota        int64  `json:"quota"`
		UsedQuota    int64  `json:"used_quota"`
		RequestCount int64  `json:"request_count"`
		Group        string `json:"group"`
		Status       int    `json:"status"`
	} `json:"data"`
	Message string `json:"message"`
}

func (c *Client) GetQuota(ctx context.Context) (*provider.QuotaInfo, *provider.RawResponse, error) {
	base := c.oa.BaseURL()
	if base == "" {
		return nil, nil, errors.New("未配置 NewAPI 地址")
	}
	if c.cred.SystemToken == "" {
		return nil, nil, errors.New("未配置 System Token")
	}
	raw, body, err := c.getSelf(ctx, base+"/api/user/self", c.cred.SystemToken)
	if err != nil && raw != nil && raw.HTTPStatus == 401 {
		// Some builds expect the Bearer prefix; retry once.
		raw2, body2, err2 := c.getSelf(ctx, base+"/api/user/self", "Bearer "+c.cred.SystemToken)
		if err2 == nil {
			raw, body, err = raw2, body2, nil
		}
	}
	if err != nil {
		return nil, raw, err
	}
	var sr selfResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, raw, fmt.Errorf("解析用户信息失败: %w", err)
	}
	if !sr.Success {
		msg := sr.Message
		if msg == "" {
			msg = "接口返回 success=false"
		}
		return nil, raw, errors.New(msg)
	}
	info := &provider.QuotaInfo{
		Balance: &provider.BalanceInfo{
			Amount:   float64(sr.Data.Quota) / quotaPerUSD,
			Currency: "USD",
		},
	}
	reqCount := sr.Data.RequestCount
	info.Usage = &provider.UsageInfo{Requests: &reqCount}
	usedUSD := float64(sr.Data.UsedQuota) / quotaPerUSD
	info.Extra = append(info.Extra,
		provider.QuotaMetric{Label: "已用额度", Value: formatUSD(usedUSD)},
	)
	if sr.Data.Group != "" {
		info.Extra = append(info.Extra, provider.QuotaMetric{Label: "用户组", Value: sr.Data.Group})
	}
	if c.cred.UserID != "" {
		info.Extra = append(info.Extra, provider.QuotaMetric{Label: "用户", Value: sr.Data.Username + " (#" + c.cred.UserID + ")"})
	}
	return info, raw, nil
}

func (c *Client) getSelf(ctx context.Context, url, auth string) (*provider.RawResponse, []byte, error) {
	headers := map[string]string{}
	if c.cred.UserID != "" {
		// new-api consoles require the user id header on admin APIs.
		headers["New-Api-User"] = c.cred.UserID
	}
	return c.oa.GetQuotaRawWithHeaders(ctx, url, auth, headers)
}

func formatUSD(v float64) string {
	return "$" + strconv.FormatFloat(v, 'f', 2, 64)
}
