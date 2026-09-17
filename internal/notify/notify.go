// Package notify sends task result notifications via DingTalk robots or
// generic webhooks.
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Configs are stored (encrypted) per channel.
type DingTalkConfig struct {
	Webhook string `json:"webhook"`
	Secret  string `json:"secret,omitempty"`
}

type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"` // default POST
	Headers map[string]string `json:"headers,omitempty"`
}

type FeishuConfig struct {
	Webhook string `json:"webhook"`
	Secret  string `json:"secret,omitempty"`
}

type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

type WeComConfig struct {
	Webhook string `json:"webhook"`
}

// NtfyConfig 推送通知。默认使用官方服务 https://ntfy.sh。
type NtfyConfig struct {
	Topic string `json:"topic"`
	// Server 为空时使用官方服务 https://ntfy.sh。
	Server string `json:"server,omitempty"`
	// Token 用于访问受保护主题（访问令牌，可选）。
	Token string `json:"token,omitempty"`
}

// BarkConfig iOS 推送。默认使用官方服务 https://api.day.app。
type BarkConfig struct {
	DeviceKey string `json:"device_key"`
	// Server 为空时使用官方服务 https://api.day.app。
	Server string `json:"server,omitempty"`
	// Sound 推送铃声名称（可选，Bark App 内置铃声）。
	Sound string `json:"sound,omitempty"`
	// Group 消息分组（可选）。
	Group string `json:"group,omitempty"`
}

// GotifyConfig 自托管 Gotify 推送。
type GotifyConfig struct {
	Server   string `json:"server"`   // Gotify 服务器地址，如 http://gotify.example.com
	AppToken string `json:"app_token"` // 应用令牌
	Priority int    `json:"priority,omitempty"` // 1-10，默认 5
}

// EmailConfig 通过 SMTP 发送邮件通知。
type EmailConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	// UseTLS 控制 465 端口隐式 TLS；587 端口走 STARTTLS。
	UseTLS bool `json:"use_tls"`
}

// Message is a rendered notification ready for any channel.
type Message struct {
	Event   string `json:"event"` // task_success | task_failure | test
	Title   string `json:"title"`
	Content string `json:"content"`
	Task    string `json:"task,omitempty"`
	At      string `json:"at"`
}

type Sender struct {
	hc *http.Client
}

func NewSender(hc *http.Client) *Sender { return &Sender{hc: hc} }

// Send dispatches to the channel identified by type + encrypted-config JSON.
func (s *Sender) Send(ctx context.Context, channelType, configJSON string, msg Message) error {
	switch channelType {
	case "dingtalk":
		return s.sendDingTalk(ctx, configJSON, msg)
	case "webhook":
		return s.sendWebhook(ctx, configJSON, msg)
	case "feishu":
		return s.sendFeishu(ctx, configJSON, msg)
	case "telegram":
		return s.sendTelegram(ctx, configJSON, msg)
	case "wecom":
		return s.sendWeCom(ctx, configJSON, msg)
	case "ntfy":
		return s.sendNtfy(ctx, configJSON, msg)
	case "bark":
		return s.sendBark(ctx, configJSON, msg)
	case "gotify":
		return s.sendGotify(ctx, configJSON, msg)
	case "email":
		return s.sendEmail(ctx, configJSON, msg)
	default:
		return fmt.Errorf("未知通知渠道类型: %s", channelType)
	}
}

func (s *Sender) sendDingTalk(ctx context.Context, configJSON string, msg Message) error {
	var cfg DingTalkConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("解析钉钉配置: %w", err)
	}
	if cfg.Webhook == "" {
		return fmt.Errorf("钉钉 Webhook 未配置")
	}
	hookURL := cfg.Webhook
	if cfg.Secret != "" {
		ts := time.Now().UnixMilli()
		sign := dingTalkSign(ts, cfg.Secret)
		hookURL += fmt.Sprintf("&timestamp=%d&sign=%s", ts, url.QueryEscape(sign))
	}
	payload := map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": msg.Title + "\n" + msg.Content},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("钉钉 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	var dr struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &dr); err == nil && dr.ErrCode != 0 {
		return fmt.Errorf("钉钉错误 %d: %s", dr.ErrCode, dr.ErrMsg)
	}
	return nil
}

func dingTalkSign(ts int64, secret string) string {
	stringToSign := strconv.FormatInt(ts, 10) + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Sender) sendWebhook(ctx context.Context, configJSON string, msg Message) error {
	var cfg WebhookConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("解析 Webhook 配置: %w", err)
	}
	if cfg.URL == "" {
		return fmt.Errorf("Webhook URL 未配置")
	}
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}
	body, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, method, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Webhook HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	return nil
}

// sendFeishu sends a text message via a Feishu/Lark custom bot webhook.
func (s *Sender) sendFeishu(ctx context.Context, configJSON string, msg Message) error {
	var cfg FeishuConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("解析飞书配置: %w", err)
	}
	if cfg.Webhook == "" {
		return fmt.Errorf("飞书 Webhook 未配置")
	}
	payload := map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": msg.Title + "\n" + msg.Content},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("飞书 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	var fr struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &fr); err == nil && fr.Code != 0 {
		return fmt.Errorf("飞书错误 %d: %s", fr.Code, fr.Msg)
	}
	return nil
}

// sendTelegram sends a text message via the Telegram Bot API.
func (s *Sender) sendTelegram(ctx context.Context, configJSON string, msg Message) error {
	var cfg TelegramConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("解析 Telegram 配置: %w", err)
	}
	if cfg.BotToken == "" || cfg.ChatID == "" {
		return fmt.Errorf("Telegram Bot Token 或 Chat ID 未配置")
	}
	apiURL := "https://api.telegram.org/bot" + cfg.BotToken + "/sendMessage"
	payload := map[string]string{
		"chat_id": cfg.ChatID,
		"text":    msg.Title + "\n" + msg.Content,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Telegram HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	var tr struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &tr); err == nil && !tr.OK {
		return fmt.Errorf("Telegram 错误: %s", tr.Description)
	}
	return nil
}

// sendWeCom sends a text message via a WeCom (企业微信) robot webhook.
func (s *Sender) sendWeCom(ctx context.Context, configJSON string, msg Message) error {
	var cfg WeComConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("解析企业微信配置: %w", err)
	}
	if cfg.Webhook == "" {
		return fmt.Errorf("企业微信 Webhook 未配置")
	}
	payload := map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": msg.Title + "\n" + msg.Content},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("企业微信 HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	var wr struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &wr); err == nil && wr.ErrCode != 0 {
		return fmt.Errorf("企业微信错误 %d: %s", wr.ErrCode, wr.ErrMsg)
	}
	return nil
}

// sendBark pushes a notification to iOS via Bark (default official server api.day.app).
func (s *Sender) sendBark(ctx context.Context, configJSON string, msg Message) error {
	var cfg BarkConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("解析 Bark 配置: %w", err)
	}
	if cfg.DeviceKey == "" {
		return fmt.Errorf("Bark 设备 Key 未配置")
	}
	server := cfg.Server
	if server == "" {
		server = "https://api.day.app"
	}
	server = strings.TrimRight(server, "/")
	payload := map[string]string{
		"device_key": cfg.DeviceKey,
		"title":      msg.Title,
		"body":       msg.Content,
		"group":      cfg.Group,
	}
	if cfg.Sound != "" {
		payload["sound"] = cfg.Sound
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := s.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Bark HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	var br struct {
		Code int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &br); err == nil && br.Code != 0 {
		return fmt.Errorf("Bark 错误 %d: %s", br.Code, br.Message)
	}
	return nil
}

// sendNtfy pushes a notification via ntfy (default official server ntfy.sh).
func (s *Sender) sendNtfy(ctx context.Context, configJSON string, msg Message) error {
	var cfg NtfyConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("解析 ntfy 配置: %w", err)
	}
	if cfg.Topic == "" {
		return fmt.Errorf("ntfy 主题未配置")
	}
	server := cfg.Server
	if server == "" {
		server = "https://ntfy.sh"
	}
	server = strings.TrimRight(server, "/")
	payload := map[string]string{
		"topic":    cfg.Topic,
		"title":    msg.Title,
		"message":  msg.Content,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ntfy HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	return nil
}

// sendGotify pushes a message to a self-hosted Gotify server.
func (s *Sender) sendGotify(ctx context.Context, configJSON string, msg Message) error {
	var cfg GotifyConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("解析 Gotify 配置: %w", err)
	}
	if cfg.Server == "" || cfg.AppToken == "" {
		return fmt.Errorf("Gotify 服务器地址或应用令牌未配置")
	}
	priority := cfg.Priority
	if priority == 0 {
		priority = 5
	}
	payload := map[string]any{
		"title":    msg.Title,
		"message":  msg.Content,
		"priority": priority,
	}
	body, _ := json.Marshal(payload)
	url := strings.TrimRight(cfg.Server, "/") + "/message?token=" + url.QueryEscape(cfg.AppToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Gotify HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	return nil
}

// sendEmail sends a plain-text email via SMTP.
func (s *Sender) sendEmail(ctx context.Context, configJSON string, msg Message) error {
	var cfg EmailConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("解析邮件配置: %w", err)
	}
	if cfg.Host == "" || cfg.To == "" || cfg.From == "" {
		return fmt.Errorf("邮件服务器 / 发件人 / 收件人未配置")
	}
	if cfg.Port == 0 {
		cfg.Port = 465
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	subject := mime.QEncoding.Encode("utf-8", msg.Title)

	headers := map[string]string{
		"From":    cfg.From,
		"To":      cfg.To,
		"Subject": subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/plain; charset=UTF-8",
	}
	var sb strings.Builder
	for k, v := range headers {
		fmt.Fprintf(&sb, "%s: %s\r\n", k, v)
	}
	sb.WriteString("\r\n")
	sb.WriteString(msg.Content)

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("连接邮件服务器失败: %w", err)
	}

	var client *smtp.Client
	if cfg.UseTLS || cfg.Port == 465 {
		tlsCfg := &tls.Config{ServerName: cfg.Host}
		conn = tls.Client(conn, tlsCfg)
	}
	client, err = smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()

	// 587 等非 SSL 端口优先尝试 STARTTLS 升级加密
	if !cfg.UseTLS && cfg.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
				return fmt.Errorf("SMTP STARTTLS 失败: %w", err)
			}
		}
	}
	if cfg.Username != "" && cfg.Password != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	// 收件人支持英文逗号分隔多个地址
	for _, rcpt := range strings.Split(cfg.To, ",") {
		rcpt = strings.TrimSpace(rcpt)
		if rcpt == "" {
			continue
		}
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("设置收件人失败: %w", err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("打开邮件数据流失败: %w", err)
	}
	if _, err := w.Write([]byte(sb.String())); err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}
	return client.Quit()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
