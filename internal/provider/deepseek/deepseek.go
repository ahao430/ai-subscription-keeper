// Package deepseek implements DeepSeek: OpenAI-compatible chat plus the
// account balance endpoint GET /user/balance.
package deepseek

import (
	"context"
	"encoding/json"
	"strconv"

	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/openai"
)

type Client struct {
	oa *openai.Client
}

func New(cred *provider.Credential, hc provider.HTTPClient) *Client {
	return &Client{oa: openai.New(openai.Options{
		DefaultBaseURL:     "https://api.deepseek.com",
		IncludeStreamUsage: true,
	}, cred, hc)}
}

func (c *Client) GetModels(ctx context.Context) ([]provider.Model, error) {
	return c.oa.GetModels(ctx)
}

func (c *Client) ChatStream(ctx context.Context, req provider.ModelRequest, onDelta provider.DeltaFunc) (*provider.StreamResult, error) {
	return c.oa.ChatStream(ctx, req, onDelta)
}

func (c *Client) SupportsQuota() bool { return true }

type balanceResponse struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

func (c *Client) GetQuota(ctx context.Context) (*provider.QuotaInfo, *provider.RawResponse, error) {
	raw, body, err := c.oa.GetQuotaRaw(ctx, c.oa.BaseURL()+"/user/balance", "Bearer "+c.oa.APIKey())
	if err != nil {
		return nil, raw, err
	}
	var br balanceResponse
	if jerr := json.Unmarshal(body, &br); jerr == nil && len(br.BalanceInfos) > 0 {
		info := &provider.QuotaInfo{}
		b := br.BalanceInfos[0]
		if amt, perr := strconv.ParseFloat(b.TotalBalance, 64); perr == nil {
			currency := b.Currency
			if currency == "CNY" {
				currency = "¥"
			}
			info.Balance = &provider.BalanceInfo{Amount: amt, Currency: currency}
		}
		if !br.IsAvailable {
			info.Extra = append(info.Extra, provider.QuotaMetric{Label: "账户状态", Value: "不可用"})
		}
		return info, raw, nil
	}
	return &provider.QuotaInfo{}, raw, nil
}
