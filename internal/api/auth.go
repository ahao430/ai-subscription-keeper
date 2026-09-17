package api

import (
	"net/http"
	"strings"

	"ai-subscription-keeper/internal/app"
	"ai-subscription-keeper/internal/auth"
)

// handleAuthStatus 是公开端点：前端启动时据此决定是否渲染登录页。
func handleAuthStatus(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		loggedIn := !a.Auth.Enabled() || a.Auth.Valid(auth.TokenFromRequest(r))
		writeJSON(w, http.StatusOK, map[string]any{
			"auth_required": a.Auth.Enabled(),
			"logged_in":     loggedIn,
		})
	}
}

func handleLogin(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		token, err := a.Auth.Login(body.Username, body.Password, auth.ClientIP(r))
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		auth.SetCookie(w, r, token, int(auth.SessionTTL.Seconds()))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleLogout(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token := auth.TokenFromRequest(r); token != "" {
			a.Auth.Logout(token)
		}
		auth.SetCookie(w, r, "", -1) // 立即清除 cookie
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// authMiddleware 在启用登录时保护全部 /api/*（登录/状态端点与静态资源除外）。
func authMiddleware(a *app.App, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Auth.Enabled() ||
			r.URL.Path == "/api/login" ||
			r.URL.Path == "/api/auth/status" ||
			!strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if a.Auth.Valid(auth.TokenFromRequest(r)) {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录或会话已过期"})
	})
}
