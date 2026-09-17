// Package claude implements the Anthropic Messages protocol with SSE
// streaming. Auth is x-api-key by default; AuthMode "bearer" switches to
// Authorization: Bearer for OAuth (Claude Max) style tokens and gateways.
package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-subscription-keeper/internal/provider"
)

const (
	defaultBase        = "https://api.anthropic.com"
	anthropicVersion   = "2023-06-01"
	defaultMaxTokens   = 4096
	fallbackModelsList = "claude-sonnet-4-5,claude-opus-4-1,claude-haiku-4-5"
)

// Options tunes the Anthropic client for vendors that reuse the protocol
// (MiniMax / Kimi coding plans) with their own fallback model lists.
type Options struct {
	FallbackModels []string
}

type Client struct {
	cred    *provider.Credential
	hc      provider.HTTPClient
	falls   []string
}

func New(cred *provider.Credential, hc provider.HTTPClient) *Client {
	return NewWithOptions(cred, hc, Options{})
}

func NewWithOptions(cred *provider.Credential, hc provider.HTTPClient, opts Options) *Client {
	falls := opts.FallbackModels
	if len(falls) == 0 {
		falls = strings.Split(fallbackModelsList, ",")
	}
	return &Client{cred: cred, hc: hc, falls: falls}
}

// Falls exposes the fallback model ids (also used by embedding providers).
func (c *Client) Falls() []string { return c.falls }

func (c *Client) baseURL() string {
	if c.cred.BaseURL != "" {
		return strings.TrimRight(c.cred.BaseURL, "/")
	}
	return defaultBase
}

func (c *Client) auth(req *http.Request) error {
	if c.cred.APIKey == "" {
		return errors.New("未配置 API Token")
	}
	if c.cred.AuthMode == "bearer" {
		req.Header.Set("Authorization", "Bearer "+c.cred.APIKey)
	} else {
		req.Header.Set("x-api-key", c.cred.APIKey)
	}
	req.Header.Set("anthropic-version", anthropicVersion)
	return nil
}

func (c *Client) GetModels(ctx context.Context) ([]provider.Model, error) {
	if c.cred.APIKey == "" {
		return nil, errors.New("未配置 API Token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+"/v1/models?limit=100", nil)
	if err != nil {
		return nil, err
	}
	if err := c.auth(req); err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode/100 != 2 {
		// Some coding-plan gateways don't expose /v1/models — fall back to the
		// curated list so warmup/test can still be configured.
		if resp.StatusCode == 404 || resp.StatusCode == 405 {
			return c.fallbackModels(), nil
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return c.fallbackModels(), nil
	}
	models := make([]provider.Model, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			models = append(models, provider.Model{ID: m.ID})
		}
	}
	return models, nil
}

func (c *Client) fallbackModels() []provider.Model {
	out := make([]provider.Model, 0, len(c.falls))
	for _, id := range c.falls {
		if id != "" {
			out = append(out, provider.Model{ID: id})
		}
	}
	return out
}

func (c *Client) SupportsQuota() bool { return false }

func (c *Client) GetQuota(ctx context.Context) (*provider.QuotaInfo, *provider.RawResponse, error) {
	return nil, nil, provider.ErrQuotaUnsupported
}

func (c *Client) ChatStream(ctx context.Context, req provider.ModelRequest, onDelta provider.DeltaFunc) (*provider.StreamResult, error) {
	if req.Model == "" {
		return nil, errors.New("未指定模型")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": defaultMaxTokens,
		"stream":     true,
		"messages": []map[string]any{{
			"role":    "user",
			"content": []map[string]string{{"type": "text", "text": req.Prompt}},
		}},
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err := c.auth(httpReq); err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 512))
	}

	result := &provider.StreamResult{Model: req.Model}
	var sb strings.Builder
	usage := &provider.StreamUsage{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
scan:
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Usage struct {
				InputTokens              int64 `json:"input_tokens"`
				OutputTokens             int64 `json:"output_tokens"`
				CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			} `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Text != "" {
				sb.WriteString(ev.Delta.Text)
				if onDelta != nil {
					onDelta(ev.Delta.Text)
				}
			}
		case "message_start":
			// message_start carries a nested usage object in message.usage;
			// parsed loosely below via second pass is overkill — skip.
		case "message_delta":
			if ev.Usage.OutputTokens > 0 {
				usage.CompletionTokens = ev.Usage.OutputTokens
				usage.PromptTokens = max(usage.PromptTokens, 0)
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			}
		case "message_stop":
			break scan
		case "error":
			return nil, fmt.Errorf("模型返回错误: %s", ev.Error.Message)
		}
	}
	if err := scanner.Err(); err != nil {
		if sb.Len() > 0 {
			result.Content, result.DurationMS = sb.String(), time.Since(start).Milliseconds()
			result.Usage = usage
			return result, fmt.Errorf("流中断: %w", err)
		}
		return nil, fmt.Errorf("读取流失败: %w", err)
	}
	if sb.Len() == 0 {
		return nil, errors.New("模型未返回任何内容")
	}
	result.Content = sb.String()
	result.DurationMS = time.Since(start).Milliseconds()
	if usage.TotalTokens > 0 {
		result.Usage = usage
	}
	return result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
