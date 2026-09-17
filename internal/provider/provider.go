// Package provider defines the unified abstraction over AI vendors. Each
// vendor implementation converts its native protocol (OpenAI Chat Completions
// or Anthropic Messages) into these shared types; callers never pick a wire
// protocol — the provider type decides it.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

var ErrQuotaUnsupported = errors.New("该供应商不支持额度查询")

// HTTPClient is satisfied by *httpclient.Manager.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Credential is the decrypted credential/config payload stored per model
// service. Fields unused by a provider are simply left empty.
type Credential struct {
	APIKey      string `json:"api_key,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`  // override provider default
	SystemToken string `json:"system_token,omitempty"` // NewAPI
	UserID      string `json:"user_id,omitempty"`      // NewAPI
	AuthMode    string `json:"auth_mode,omitempty"`    // claude: "api_key" | "bearer"
	QuotaURL    string `json:"quota_url,omitempty"`    // optional quota endpoint override
}

type Model struct {
	ID string `json:"id"`
}

type ModelRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// QuotaDimension is one quota axis (e.g. 5-hour token limit). UsedPercentage
// is what the vendor reports as consumed; the dashboard renders
// 100 - UsedPercentage as "剩余".
type QuotaDimension struct {
	Code           string     `json:"code"`
	DisplayName    string     `json:"display_name"`
	UsedPercentage *float64   `json:"used_percentage"`
	ResetAt        *time.Time `json:"reset_at,omitempty"`
}

type BalanceInfo struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type UsageInfo struct {
	TodaySpend *float64 `json:"today_spend,omitempty"`
	MonthSpend *float64 `json:"month_spend,omitempty"`
	Requests   *int64   `json:"requests,omitempty"`
	Tokens     *int64   `json:"tokens,omitempty"`
}

type QuotaMetric struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// QuotaInfo is the vendor-agnostic quota model. Vendors fill what they have.
type QuotaInfo struct {
	Dimensions []QuotaDimension `json:"dimensions,omitempty"`
	Balance    *BalanceInfo     `json:"balance,omitempty"`
	Usage      *UsageInfo       `json:"usage,omitempty"`
	Extra      []QuotaMetric    `json:"extra,omitempty"`
}

// RawResponse captures the (sanitized) last quota HTTP exchange for the
// dashboard "原始数据" drawer and provider debugging.
type RawResponse struct {
	URL        string          `json:"url"`
	Method     string          `json:"method"`
	HTTPStatus int             `json:"http_status"`
	QueriedAt  time.Time       `json:"queried_at"`
	DurationMS int64           `json:"duration_ms"`
	Body       json.RawMessage `json:"body"`
	Error      string          `json:"error,omitempty"`
}

type StreamUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type StreamResult struct {
	Model      string       `json:"model"`
	Content    string       `json:"content"`
	DurationMS int64        `json:"duration_ms"`
	Usage      *StreamUsage `json:"usage,omitempty"`
}

// DeltaFunc receives incremental text as the SSE stream progresses.
type DeltaFunc func(delta string)

type Provider interface {
	// GetModels lists models callable with this credential.
	GetModels(ctx context.Context) ([]Model, error)
	// SupportsQuota reports whether the vendor exposes a quota/balance API.
	SupportsQuota() bool
	// GetQuota queries vendor quota. When unsupported it returns
	// ErrQuotaUnsupported. The RawResponse (sanitized) is always returned when
	// an HTTP call was made, even on error.
	GetQuota(ctx context.Context) (*QuotaInfo, *RawResponse, error)
	// ChatStream performs a streaming chat completion, invoking onDelta for
	// every content chunk. Used for both model testing and warmup.
	ChatStream(ctx context.Context, req ModelRequest, onDelta DeltaFunc) (*StreamResult, error)
}

// Field describes one credential form input for the frontend.
type Field struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "text" | "password" | "number"
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
}

// ProviderType is the static descriptor of a vendor.
type ProviderType struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Fields        []Field `json:"fields"`
	SupportsQuota bool    `json:"supports_quota"`
	AuthNote      string  `json:"auth_note,omitempty"`
	Official      bool    `json:"official"` // true = 官方预设, baseURL 不可修改
	Logo          string  `json:"logo,omitempty"` // logo 图标路径，如 /logos/zhipu.svg
}
