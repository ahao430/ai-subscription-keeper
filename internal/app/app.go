// Package app wires the runtime together: store, crypto, HTTP client,
// notifier, executor and scheduler. It also implements the quota refresh
// pipeline shared by the dashboard and warmup verification.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ai-subscription-keeper/internal/auth"
	"ai-subscription-keeper/internal/config"
	"ai-subscription-keeper/internal/crypto"
	"ai-subscription-keeper/internal/httpclient"
	"ai-subscription-keeper/internal/notify"
	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/registry"
	"ai-subscription-keeper/internal/scheduler"
	"ai-subscription-keeper/internal/store"
	"ai-subscription-keeper/internal/task"
)

type App struct {
	Cfg     config.Config
	Store   *store.Store
	Enc     *crypto.Encryptor
	HC      *httpclient.Manager
	Notifier *notify.Sender
	Exec    *task.Executor
	Sched   *scheduler.Scheduler
	Auth    *auth.Manager
}

func New(cfg config.Config) (*App, error) {
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	secret := cfg.EncryptionKey
	if secret == "" {
		// No env key: generate one and persist it in the data dir so secrets
		// survive restarts. Env var still takes precedence when present.
		keyPath := filepath.Join(cfg.DataDir, ".secret.key")
		data, err := os.ReadFile(keyPath)
		if err != nil {
			buf := make([]byte, 32)
			if _, err := rand.Read(buf); err != nil {
				st.Close()
				return nil, err
			}
			data = []byte(hex.EncodeToString(buf))
			if err := os.WriteFile(keyPath, data, 0o600); err != nil {
				st.Close()
				return nil, fmt.Errorf("写入密钥文件: %w", err)
			}
		}
		secret = string(data)
	}
	enc, err := crypto.New(secret)
	if err != nil {
		st.Close()
		return nil, err
	}

	hc := httpclient.NewManager()
	if proxyJSON, err := st.GetProxySetting(); err == nil && proxyJSON != "" {
		var pc httpclient.ProxyConfig
		if err := json.Unmarshal([]byte(proxyJSON), &pc); err == nil {
			_ = hc.Update(pc)
		}
	}

	notifier := notify.NewSender(&http.Client{Timeout: 30 * time.Second})
	executor := task.NewExecutor(st, enc, hc, notifier)
	sched := scheduler.New(st, executor)
	authMgr := auth.New(cfg.AuthUsername, cfg.AuthPassword)

	a := &App{Cfg: cfg, Store: st, Enc: enc, HC: hc, Notifier: notifier, Exec: executor, Sched: sched, Auth: authMgr}
	if err := sched.Start(); err != nil {
		st.Close()
		return nil, err
	}
	if authMgr.Enabled() {
		fmt.Printf("内置登录已启用（用户 %s）；未设置 AUTH_PASSWORD 时为免认证模式\n", cfg.AuthUsername)
	}
	return a, nil
}

func (a *App) Close() {
	if a.Sched != nil {
		a.Sched.Stop()
	}
	if a.Store != nil {
		a.Store.Close()
	}
}

// ProviderFor instantiates the provider bound to a model service.
func (a *App) ProviderFor(ms *store.ModelService) (provider.Provider, error) {
	cred, err := a.DecryptCredential(ms.Credential)
	if err != nil {
		return nil, err
	}
	return registry.Get(ms.ProviderType, cred, a.HC)
}

func (a *App) DecryptCredential(encBlob string) (*provider.Credential, error) {
	if encBlob == "" {
		return &provider.Credential{}, nil
	}
	plain, err := a.Enc.Decrypt(encBlob)
	if err != nil {
		return nil, err
	}
	var cred provider.Credential
	if err := json.Unmarshal([]byte(plain), &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (a *App) EncryptCredential(cred *provider.Credential) (string, error) {
	plain, err := json.Marshal(cred)
	if err != nil {
		return "", err
	}
	return a.Enc.Encrypt(string(plain))
}

// QuotaPayload is the cached JSON stored in model_services.last_quota.
type QuotaPayload struct {
	Quota       *provider.QuotaInfo `json:"quota"`
	Raw         *provider.RawResponse `json:"raw,omitempty"`
	Unsupported bool                `json:"unsupported,omitempty"`
}

// RefreshQuota queries the provider and caches the sanitized result.
// It always records last_quota_at / last_quota_error so the dashboard can show
// both success and failure states.
func (a *App) RefreshQuota(ctx context.Context, ms *store.ModelService) {
	if !ms.Enabled {
		return
	}
	pv, err := a.ProviderFor(ms)
	if err != nil {
		_ = a.Store.SaveQuotaResult(ms.ID, "", time.Time{}, err.Error())
		return
	}
	if !pv.SupportsQuota() {
		payload, _ := json.Marshal(QuotaPayload{Unsupported: true})
		_ = a.Store.SaveQuotaResult(ms.ID, string(payload), time.Now().UTC(), "")
		return
	}
	qi, raw, err := pv.GetQuota(ctx)
	payload := QuotaPayload{Quota: qi, Raw: raw}
	payloadJSON, _ := json.Marshal(payload)
	if err != nil {
		_ = a.Store.SaveQuotaResult(ms.ID, string(payloadJSON), time.Now().UTC(), err.Error())
		return
	}
	_ = a.Store.SaveQuotaResult(ms.ID, string(payloadJSON), time.Now().UTC(), "")
}
