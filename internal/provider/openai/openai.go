// Package openai implements the OpenAI Chat Completions protocol with SSE
// streaming. Most vendors in this project are OpenAI-compatible, so they embed
// this generic client and only add their quota endpoints / defaults.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/sensitize"
)

// Options tunes the generic client per vendor.
type Options struct {
	// DefaultBaseURL is used when the credential has no base_url override.
	DefaultBaseURL string
	// IncludeStreamUsage adds stream_options.include_usage — supported by
	// OpenAI/DeepSeek/NewAPI style endpoints but rejected by a few stricter ones.
	IncludeStreamUsage bool
	// FallbackModels is returned by GetModels when the /models endpoint is
	// unavailable (some coding-plan endpoints don't expose it).
	FallbackModels []string
	// MaxTokens, when > 0, is sent with chat requests (Anthropic-style vendors
	// require it even in OpenAI-compatible mode).
	MaxTokens int
}

type Client struct {
	opts Options
	cred *provider.Credential
	hc   provider.HTTPClient
}

func New(opts Options, cred *provider.Credential, hc provider.HTTPClient) *Client {
	return &Client{opts: opts, cred: cred, hc: hc}
}

func (c *Client) BaseURL() string {
	if c.cred.BaseURL != "" {
		return strings.TrimRight(c.cred.BaseURL, "/")
	}
	return strings.TrimRight(c.opts.DefaultBaseURL, "/")
}

func (c *Client) APIKey() string { return c.cred.APIKey }

func (c *Client) validate() error {
	if c.BaseURL() == "" {
		return errors.New("未配置 Base URL")
	}
	if c.cred.APIKey == "" {
		return errors.New("未配置 API Token")
	}
	return nil
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (c *Client) GetModels(ctx context.Context) ([]provider.Model, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL()+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cred.APIKey)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
	}
	var mr modelsResponse
	if err := json.Unmarshal(body, &mr); err != nil {
		return nil, fmt.Errorf("解析模型列表失败: %w", err)
	}
	ids := make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 && len(c.opts.FallbackModels) > 0 {
		ids = append(ids, c.opts.FallbackModels...)
	}
	sort.Strings(ids)
	models := make([]provider.Model, len(ids))
	for i, id := range ids {
		models[i] = provider.Model{ID: id}
	}
	return models, nil
}

func (c *Client) SupportsQuota() bool { return false }

func (c *Client) GetQuota(ctx context.Context) (*provider.QuotaInfo, *provider.RawResponse, error) {
	return nil, nil, provider.ErrQuotaUnsupported
}

// ChatStream POSTs {base}/chat/completions with stream=true and reads SSE
// until [DONE]. The overall request is capped at 5 minutes.
func (c *Client) ChatStream(ctx context.Context, req provider.ModelRequest, onDelta provider.DeltaFunc) (*provider.StreamResult, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if req.Model == "" {
		return nil, errors.New("未指定模型")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	payload := map[string]any{
		"model":    req.Model,
		"stream":   true,
		"messages": []map[string]string{{"role": "user", "content": req.Prompt}},
	}
	if c.opts.IncludeStreamUsage {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}
	if c.opts.MaxTokens > 0 {
		payload["max_tokens"] = c.opts.MaxTokens
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL()+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cred.APIKey)
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
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break scan
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // tolerate keep-alive noise
		}
		if len(chunk.Choices) > 0 {
			if content := chunk.Choices[0].Delta.Content; content != "" {
				sb.WriteString(content)
				if onDelta != nil {
					onDelta(content)
				}
			}
		}
		if chunk.Usage.TotalTokens > 0 {
			u := chunk.Usage
			usage = &u
		}
	}
	if err := scanner.Err(); err != nil {
		if sb.Len() > 0 {
			// We already streamed partial content; surface what we have with
			// a note instead of losing it.
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

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage provider.StreamUsage `json:"usage"`
}

// GetQuotaRaw performs a GET against an arbitrary quota URL with Bearer auth
// and returns the sanitized raw exchange. Vendors with custom quotas build on
// this so they all share logging / error semantics.
func (c *Client) GetQuotaRaw(ctx context.Context, url, authHeader string) (*provider.RawResponse, []byte, error) {
	return c.GetQuotaRawWithHeaders(ctx, url, authHeader, nil)
}

// GetQuotaRawWithHeaders is GetQuotaRaw plus arbitrary request headers.
func (c *Client) GetQuotaRawWithHeaders(ctx context.Context, url, authHeader string, headers map[string]string) (*provider.RawResponse, []byte, error) {
	raw := &provider.RawResponse{URL: url, Method: http.MethodGet, QueriedAt: time.Now().UTC()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		raw.Error = err.Error()
		return raw, nil, err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	raw.DurationMS = time.Since(raw.QueriedAt).Milliseconds()
	if err != nil {
		raw.Error = err.Error()
		return raw, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	raw.HTTPStatus = resp.StatusCode
	sanitized := sensitize.JSON(body)
	raw.Body = json.RawMessage(sanitized)
	if resp.StatusCode/100 != 2 {
		err := fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
		raw.Error = err.Error()
		return raw, body, err
	}
	return raw, body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
