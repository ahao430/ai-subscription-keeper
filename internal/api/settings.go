package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"ai-subscription-keeper/internal/app"
	"ai-subscription-keeper/internal/httpclient"
)

// proxyTestURL is fetched through the configured proxy to verify it works.
const proxyTestURL = "https://www.gstatic.com/generate_204"

func handleGetProxy(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, a.HC.Config())
	}
}

func handlePutProxy(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg httpclient.ProxyConfig
		if err := decodeBody(r, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if cfg.Mode != "none" && cfg.Mode != "proxy" {
			writeError(w, http.StatusBadRequest, errStr("代理模式必须是 none 或 proxy"))
			return
		}
		if cfg.Mode == "proxy" && cfg.URL == "" {
			writeError(w, http.StatusBadRequest, errStr("使用代理时必须填写代理地址"))
			return
		}
		if err := a.HC.Update(cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		b, _ := json.Marshal(cfg)
		if err := a.Store.SetProxySetting(string(b)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	}
}

func handleTestProxy(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg httpclient.ProxyConfig
		if err := decodeBody(r, &cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if cfg.Mode != "proxy" || cfg.URL == "" {
			writeError(w, http.StatusBadRequest, errStr("请先配置代理地址"))
			return
		}
		if err := a.HC.Update(cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, proxyTestURL, nil)
		resp, err := a.HC.Do(req)
		elapsed := time.Since(start).Milliseconds()
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "latency_ms": elapsed})
			return
		}
		resp.Body.Close()
		writeJSON(w, http.StatusOK, map[string]any{"ok": resp.StatusCode == 204, "status": resp.StatusCode, "latency_ms": elapsed})
	}
}
