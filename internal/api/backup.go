package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-subscription-keeper/internal/app"
	"ai-subscription-keeper/internal/httpclient"
	"ai-subscription-keeper/internal/store"
	"ai-subscription-keeper/internal/version"
	"ai-subscription-keeper/internal/webdav"
)

// ---------------------------------------------------------------------------
// Backup payload
// ---------------------------------------------------------------------------

type backupModelService struct {
	store.ModelService
	// CredentialPlain 是解密后的凭证 JSON，仅存在于导出文件中。
	CredentialPlain string `json:"credential_plain"`
}

type backupChannel struct {
	store.NotificationChannel
	ConfigPlain string `json:"config_plain"`
}

type backupPayload struct {
	AppVersion           string               `json:"app_version"`
	ExportedAt           time.Time            `json:"exported_at"`
	ModelServices        []backupModelService `json:"model_services"`
	Tasks                []*store.Task        `json:"tasks"`
	NotificationChannels []backupChannel      `json:"notification_channels"`
	Proxy                string               `json:"proxy,omitempty"`
}

const backupFileName = "ai-subscription-keeper-backup.json"
const settingKeyWebdavLastSync = "webdav_last_sync"

func buildBackupPayload(a *app.App) ([]byte, error) {
	payload := backupPayload{
		AppVersion:           version.Version,
		ExportedAt:           time.Now().UTC(),
		ModelServices:        []backupModelService{},
		Tasks:                []*store.Task{},
		NotificationChannels: []backupChannel{},
	}

	services, err := a.Store.ListModelServices()
	if err != nil {
		return nil, err
	}
	for _, ms := range services {
		b := backupModelService{ModelService: *ms}
		if ms.Credential != "" {
			if plain, err := a.Enc.Decrypt(ms.Credential); err == nil {
				b.CredentialPlain = plain
			}
		}
		payload.ModelServices = append(payload.ModelServices, b)
	}

	channels, err := a.Store.ListNotificationChannels()
	if err != nil {
		return nil, err
	}
	for _, c := range channels {
		b := backupChannel{NotificationChannel: *c}
		if c.Config != "" {
			if plain, err := a.Enc.Decrypt(c.Config); err == nil {
				b.ConfigPlain = plain
			}
		}
		payload.NotificationChannels = append(payload.NotificationChannels, b)
	}

	tasks, err := a.Store.ListTasks()
	if err != nil {
		return nil, err
	}
	payload.Tasks = append(payload.Tasks, tasks...)

	if proxy, err := a.Store.GetProxySetting(); err == nil {
		payload.Proxy = proxy
	}

	return json.MarshalIndent(payload, "", "  ")
}

// importBackup merges a backup payload into the local store. 已存在的 ID 跳过
// （幂等导入），凭证用本机密钥重新加密。
func importBackup(a *app.App, data []byte) (map[string]int, error) {
	var payload backupPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("备份文件格式不正确: %w", err)
	}
	out := map[string]int{}

	for _, b := range payload.ModelServices {
		ms := b.ModelService
		if _, err := a.Store.GetModelService(ms.ID); err == nil {
			continue // 已存在，跳过
		}
		if b.CredentialPlain != "" {
			enc, err := a.Enc.Encrypt(b.CredentialPlain)
			if err != nil {
				return nil, fmt.Errorf("加密服务凭证失败: %w", err)
			}
			ms.Credential = enc
		}
		ms.CreatedAt, ms.UpdatedAt = time.Time{}, time.Time{}
		if err := a.Store.CreateModelService(&ms); err != nil {
			return nil, err
		}
		out["services"]++
	}

	for _, b := range payload.NotificationChannels {
		c := b.NotificationChannel
		if _, err := a.Store.GetNotificationChannel(c.ID); err == nil {
			continue
		}
		if b.ConfigPlain != "" {
			enc, err := a.Enc.Encrypt(b.ConfigPlain)
			if err != nil {
				return nil, fmt.Errorf("加密通知配置失败: %w", err)
			}
			c.Config = enc
		}
		c.CreatedAt, c.UpdatedAt = time.Time{}, time.Time{}
		if err := a.Store.CreateNotificationChannel(&c); err != nil {
			return nil, err
		}
		out["channels"]++
	}

	for _, t := range payload.Tasks {
		if _, err := a.Store.GetTask(t.ID); err == nil {
			continue
		}
		t.CreatedAt, t.UpdatedAt = time.Time{}, time.Time{}
		if err := a.Store.CreateTask(t); err != nil {
			return nil, err
		}
		if t.Enabled {
			_ = a.Sched.Schedule(t)
		}
		out["tasks"]++
	}

	if payload.Proxy != "" {
		var cfg httpclient.ProxyConfig
		if json.Unmarshal([]byte(payload.Proxy), &cfg) == nil {
			_ = a.HC.Update(cfg)
			_ = a.Store.SetProxySetting(payload.Proxy)
			out["proxy"] = 1
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Handlers: export / import
// ---------------------------------------------------------------------------

func handleExportConfig(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := buildBackupPayload(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		name := "ai-subscription-keeper-backup-" + time.Now().Format("20060102-150405") + ".json"
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}

func handleImportConfig(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := make([]byte, 0, 32<<10)
		buf := make([]byte, 8<<10)
		for {
			n, err := r.Body.Read(buf)
			data = append(data, buf[:n]...)
			if len(data) > 32<<20 {
				writeError(w, http.StatusRequestEntityTooLarge, errStr("备份文件过大"))
				return
			}
			if err != nil {
				break
			}
		}
		out, err := importBackup(a, data)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// ---------------------------------------------------------------------------
// WebDAV
// ---------------------------------------------------------------------------

type webdavCfg struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password"`
	Path     string `json:"path"`
}

// loadWebdavCfg decrypts the stored WebDAV config; returns zero config when unset.
func loadWebdavCfg(a *app.App) (webdavCfg, error) {
	var cfg webdavCfg
	blob, err := a.Store.GetWebdavSetting()
	if err != nil || blob == "" {
		return cfg, err
	}
	plain, err := a.Enc.Decrypt(blob)
	if err != nil {
		return cfg, fmt.Errorf("解密 WebDAV 配置失败: %w", err)
	}
	err = json.Unmarshal([]byte(plain), &cfg)
	return cfg, err
}

func (c webdavCfg) client() *webdav.Client {
	return &webdav.Client{Server: c.Server, Username: c.Username, Password: c.Password}
}

func (c webdavCfg) remoteFile() string {
	return strings.TrimRight(c.Path, "/") + "/" + backupFileName
}

func handleGetWebdavConfig(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := loadWebdavCfg(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		lastSync, _ := a.Store.GetSetting(settingKeyWebdavLastSync)
		writeJSON(w, http.StatusOK, map[string]any{
			"server":       cfg.Server,
			"username":     cfg.Username,
			"path":         cfg.Path,
			"password_set": cfg.Password != "",
			"last_sync":    lastSync,
		})
	}
}

func handlePutWebdavConfig(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req webdavCfg
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Server) == "" {
			writeError(w, http.StatusBadRequest, errStr("服务器地址不能为空"))
			return
		}
		if req.Path == "" {
			req.Path = "/"
		}
		if !strings.HasPrefix(req.Path, "/") {
			req.Path = "/" + req.Path
		}
		// 留空保持原密码
		if req.Password == "" {
			if old, err := loadWebdavCfg(a); err == nil {
				req.Password = old.Password
			}
		}
		b, _ := json.Marshal(req)
		enc, err := a.Enc.Encrypt(string(b))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if err := a.Store.SetWebdavSetting(enc); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"server": req.Server, "username": req.Username,
			"path": req.Path, "password_set": req.Password != "",
		})
	}
}

func handleTestWebdav(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := loadWebdavCfg(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if cfg.Server == "" || cfg.Password == "" {
			writeError(w, http.StatusBadRequest, errStr("请先保存 WebDAV 配置"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		start := time.Now()
		err = cfg.client().Test(ctx)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         err == nil,
			"error":      errStrOrNil(err),
			"latency_ms": time.Since(start).Milliseconds(),
		})
	}
}

func errStrOrNil(err error) any {
	if err == nil {
		return nil
	}
	return err.Error()
}

// handleWebdavSync builds a full backup and uploads it to WebDAV.
func handleWebdavSync(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := loadWebdavCfg(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if cfg.Server == "" || cfg.Password == "" {
			writeError(w, http.StatusBadRequest, errStr("请先保存 WebDAV 配置"))
			return
		}
		data, err := buildBackupPayload(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		cli := cfg.client()
		if err := cli.EnsureDir(ctx, cfg.Path); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if err := cli.Put(ctx, cfg.remoteFile(), data); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_ = a.Store.SetSetting(settingKeyWebdavLastSync, now)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "size": len(data), "synced_at": now})
	}
}

// handleWebdavRestore downloads the backup from WebDAV and imports it.
func handleWebdavRestore(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := loadWebdavCfg(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if cfg.Server == "" || cfg.Password == "" {
			writeError(w, http.StatusBadRequest, errStr("请先保存 WebDAV 配置"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		data, err := cfg.client().Get(ctx, cfg.remoteFile())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		out, err := importBackup(a, data)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
