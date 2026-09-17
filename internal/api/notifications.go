package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ai-subscription-keeper/internal/app"
	"ai-subscription-keeper/internal/notify"
	"ai-subscription-keeper/internal/store"
)

// channelRequest secrets follow the same blank-means-keep rule as services.
type channelRequest struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"` // dingtalk | webhook | feishu | telegram | wecom | ntfy | email
	Webhook  string            `json:"webhook"`
	Secret   string            `json:"secret"`
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
	BotToken string            `json:"bot_token"`
	ChatID   string            `json:"chat_id"`
	// ntfy / bark / gotify
	Topic     string `json:"topic"`
	Server    string `json:"server"`
	DeviceKey string `json:"device_key"`
	AppToken  string `json:"app_token"`
	// email
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	From         string `json:"from"`
	To           string `json:"to"`
	UseTLS       bool   `json:"use_tls"`
	Enabled      *bool  `json:"enabled"`
}

type channelResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Webhook   string    `json:"webhook,omitempty"`
	URL       string    `json:"url,omitempty"`
	Method    string    `json:"method,omitempty"`
	SecretSet bool      `json:"secret_set"`
	BotToken  string    `json:"bot_token,omitempty"`
	ChatID    string    `json:"chat_id,omitempty"`
	Topic     string    `json:"topic,omitempty"`
	Server    string    `json:"server,omitempty"`
	DeviceKey string    `json:"device_key,omitempty"`
	AppToken  string    `json:"app_token,omitempty"`
	SMTPHost  string    `json:"smtp_host,omitempty"`
	SMTPPort  int       `json:"smtp_port,omitempty"`
	SMTPUser  string    `json:"smtp_username,omitempty"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toChannelResponse(a *app.App, c *store.NotificationChannel) *channelResponse {
	resp := &channelResponse{
		ID: c.ID, Name: c.Name, Type: c.Type,
		Enabled: c.Enabled, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if c.Config != "" {
		if plain, err := a.Enc.Decrypt(c.Config); err == nil {
			switch c.Type {
			case store.NotifyTypeDingTalk:
				var cfg notify.DingTalkConfig
				if json.Unmarshal([]byte(plain), &cfg) == nil {
					resp.Webhook = cfg.Webhook
					resp.SecretSet = cfg.Secret != ""
				}
			case store.NotifyTypeWebhook:
				var cfg notify.WebhookConfig
				if json.Unmarshal([]byte(plain), &cfg) == nil {
					resp.URL = cfg.URL
					resp.Method = cfg.Method
				}
			case store.NotifyTypeFeishu:
				var cfg notify.FeishuConfig
				if json.Unmarshal([]byte(plain), &cfg) == nil {
					resp.Webhook = cfg.Webhook
					resp.SecretSet = cfg.Secret != ""
				}
			case store.NotifyTypeTelegram:
				var cfg notify.TelegramConfig
				if json.Unmarshal([]byte(plain), &cfg) == nil {
					resp.BotToken = cfg.BotToken
					resp.ChatID = cfg.ChatID
				}
			case store.NotifyTypeWeCom:
				var cfg notify.WeComConfig
				if json.Unmarshal([]byte(plain), &cfg) == nil {
					resp.Webhook = cfg.Webhook
				}
			case store.NotifyTypeNtfy:
				var cfg notify.NtfyConfig
				if json.Unmarshal([]byte(plain), &cfg) == nil {
					resp.Topic = cfg.Topic
					resp.Server = cfg.Server
				}
			case store.NotifyTypeBark:
				var cfg notify.BarkConfig
				if json.Unmarshal([]byte(plain), &cfg) == nil {
					resp.DeviceKey = cfg.DeviceKey
					resp.Server = cfg.Server
				}
			case store.NotifyTypeGotify:
				var cfg notify.GotifyConfig
				if json.Unmarshal([]byte(plain), &cfg) == nil {
					resp.Server = cfg.Server
					resp.SecretSet = cfg.AppToken != "" // 令牌本身不回传
				}
			case store.NotifyTypeEmail:
				var cfg notify.EmailConfig
				if json.Unmarshal([]byte(plain), &cfg) == nil {
					resp.SMTPHost = cfg.Host
					resp.SMTPPort = cfg.Port
					resp.SMTPUser = cfg.Username
					resp.From = cfg.From
					resp.To = cfg.To
				}
			}
		}
	}
	return resp
}

func buildChannelConfig(a *app.App, existing *store.NotificationChannel, req channelRequest) (string, error) {
	switch req.Type {
	case store.NotifyTypeDingTalk:
		cfg := notify.DingTalkConfig{Webhook: req.Webhook, Secret: req.Secret}
		if existing != nil && existing.Type == store.NotifyTypeDingTalk {
			if plain, err := a.Enc.Decrypt(existing.Config); err == nil {
				var old notify.DingTalkConfig
				if json.Unmarshal([]byte(plain), &old) == nil {
					if cfg.Secret == "" {
						cfg.Secret = old.Secret
					}
				}
			}
		}
		b, _ := json.Marshal(cfg)
		return a.Enc.Encrypt(string(b))
	case store.NotifyTypeWebhook:
		cfg := notify.WebhookConfig{URL: req.URL, Method: req.Method, Headers: req.Headers}
		b, _ := json.Marshal(cfg)
		return a.Enc.Encrypt(string(b))
	case store.NotifyTypeFeishu:
		cfg := notify.FeishuConfig{Webhook: req.Webhook, Secret: req.Secret}
		if existing != nil && existing.Type == store.NotifyTypeFeishu {
			if plain, err := a.Enc.Decrypt(existing.Config); err == nil {
				var old notify.FeishuConfig
				if json.Unmarshal([]byte(plain), &old) == nil {
					if cfg.Secret == "" {
						cfg.Secret = old.Secret
					}
				}
			}
		}
		b, _ := json.Marshal(cfg)
		return a.Enc.Encrypt(string(b))
	case store.NotifyTypeTelegram:
		cfg := notify.TelegramConfig{BotToken: req.BotToken, ChatID: req.ChatID}
		if existing != nil && existing.Type == store.NotifyTypeTelegram {
			if plain, err := a.Enc.Decrypt(existing.Config); err == nil {
				var old notify.TelegramConfig
				if json.Unmarshal([]byte(plain), &old) == nil {
					if cfg.BotToken == "" {
						cfg.BotToken = old.BotToken
					}
				}
			}
		}
		b, _ := json.Marshal(cfg)
		return a.Enc.Encrypt(string(b))
	case store.NotifyTypeWeCom:
		cfg := notify.WeComConfig{Webhook: req.Webhook}
		b, _ := json.Marshal(cfg)
		return a.Enc.Encrypt(string(b))
	case store.NotifyTypeNtfy:
		cfg := notify.NtfyConfig{Topic: req.Topic, Server: req.Server}
		b, _ := json.Marshal(cfg)
		return a.Enc.Encrypt(string(b))
	case store.NotifyTypeBark:
		cfg := notify.BarkConfig{DeviceKey: req.DeviceKey, Server: req.Server}
		b, _ := json.Marshal(cfg)
		return a.Enc.Encrypt(string(b))
	case store.NotifyTypeGotify:
		cfg := notify.GotifyConfig{Server: req.Server, AppToken: req.AppToken}
		if existing != nil && existing.Type == store.NotifyTypeGotify {
			if plain, err := a.Enc.Decrypt(existing.Config); err == nil {
				var old notify.GotifyConfig
				if json.Unmarshal([]byte(plain), &old) == nil {
					if cfg.AppToken == "" {
						cfg.AppToken = old.AppToken
					}
				}
			}
		}
		b, _ := json.Marshal(cfg)
		return a.Enc.Encrypt(string(b))
	case store.NotifyTypeEmail:
		cfg := notify.EmailConfig{
			Host: req.SMTPHost, Port: req.SMTPPort,
			Username: req.SMTPUsername, Password: req.SMTPPassword,
			From: req.From, To: req.To, UseTLS: req.UseTLS,
		}
		if existing != nil && existing.Type == store.NotifyTypeEmail {
			if plain, err := a.Enc.Decrypt(existing.Config); err == nil {
				var old notify.EmailConfig
				if json.Unmarshal([]byte(plain), &old) == nil {
					if cfg.Password == "" {
						cfg.Password = old.Password
					}
				}
			}
		}
		b, _ := json.Marshal(cfg)
		return a.Enc.Encrypt(string(b))
	default:
		return "", errStr("通知类型必须是 dingtalk / webhook / feishu / telegram / wecom / ntfy / bark / gotify / email")
	}
}

func handleListChannels(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channels, err := a.Store.ListNotificationChannels()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out := make([]*channelResponse, 0, len(channels))
		for _, c := range channels {
			out = append(out, toChannelResponse(a, c))
		}
		writeJSON(w, http.StatusOK, map[string]any{"channels": out})
	}
}

func handleCreateChannel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req channelRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, errStr("名称不能为空"))
			return
		}
		cfgJSON, err := buildChannelConfig(a, nil, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		c := &store.NotificationChannel{
			Name: strings.TrimSpace(req.Name), Type: req.Type,
			Config: cfgJSON, Enabled: enabled,
		}
		if err := a.Store.CreateNotificationChannel(c); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, toChannelResponse(a, c))
	}
}

func handleUpdateChannel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := a.Store.GetNotificationChannel(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		var req channelRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Name) != "" {
			c.Name = strings.TrimSpace(req.Name)
		}
		if req.Type != "" {
			c.Type = req.Type
		}
		cfgJSON, err := buildChannelConfig(a, c, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		c.Config = cfgJSON
		if req.Enabled != nil {
			c.Enabled = *req.Enabled
		}
		if err := a.Store.UpdateNotificationChannel(c); err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, toChannelResponse(a, c))
	}
}

func handleDeleteChannel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DeleteNotificationChannel(r.PathValue("id")); err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "deleted"})
	}
}

func handleTestChannel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := a.Store.GetNotificationChannel(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		cfgJSON, err := a.Enc.Decrypt(c.Config)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		now := time.Now().Format("2006-01-02 15:04:05")
		msg := notify.Message{
			Event:   "test",
			Title:   "🔔 AI 订阅管家测试通知",
			Content: "这是一条测试通知。\n渠道：" + c.Name + "\n时间：" + now,
			Task:    "test",
			At:      time.Now().Format(time.RFC3339),
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := a.Notifier.Send(ctx, c.Type, cfgJSON, msg); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
