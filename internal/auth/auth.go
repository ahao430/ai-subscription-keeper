// Package auth implements optional built-in login: session management with
// in-memory tokens, bcrypt password verification and per-IP brute-force
// lockout. Auth is DISABLED when AUTH_PASSWORD is unset (local/intranet use).
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// CookieName is the session cookie key.
	CookieName = "keeper_session"
	// SessionTTL 会话有效期（登录后 7 天）。
	SessionTTL = 7 * 24 * time.Hour
	// MaxFailures 连续失败次数上限（按 IP），超过后锁定。
	MaxFailures = 5
	// LockoutDuration 锁定时长。
	LockoutDuration = 15 * time.Minute
)

var (
	ErrLocked  = errors.New("失败次数过多，已临时锁定，请稍后再试")
	ErrBadCred = errors.New("用户名或密码错误")
	ErrNoAuth  = errors.New("未启用登录认证")
)

// Manager holds auth state. Zero-value usable (disabled).
type Manager struct {
	enabled bool
	user    string
	hash    []byte

	mu       sync.Mutex
	sessions map[string]time.Time // token → expiry
	fails    map[string]*failState
}

type failState struct {
	count int
	until time.Time
}

// New builds a Manager. Empty password disables authentication entirely.
func New(username, password string) *Manager {
	if password == "" {
		return &Manager{}
	}
	if username == "" {
		username = "admin"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// bcrypt 仅在 cost 非法时报错；DefaultCost 下不可能失败。
		return &Manager{}
	}
	return &Manager{enabled: true, user: username, hash: hash, sessions: map[string]time.Time{}, fails: map[string]*failState{}}
}

// Enabled reports whether built-in auth is active.
func (m *Manager) Enabled() bool { return m.enabled }

// Login verifies credentials (with lockout) and returns a session token.
func (m *Manager) Login(username, password, clientIP string) (string, error) {
	if !m.enabled {
		return "", ErrNoAuth
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if fs := m.fails[clientIP]; fs != nil && time.Now().Before(fs.until) {
		return "", ErrLocked
	}
	if bcrypt.CompareHashAndPassword(m.hash, []byte(password)) != nil || username != m.user {
		fs := m.fails[clientIP]
		if fs == nil {
			fs = &failState{}
			m.fails[clientIP] = fs
		}
		fs.count++
		if fs.count >= MaxFailures {
			fs.until = time.Now().Add(LockoutDuration)
			fs.count = 0 // 锁定期结束后重新计数
		}
		return "", ErrBadCred
	}
	delete(m.fails, clientIP)

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	m.cleanupLocked()
	m.sessions[token] = time.Now().Add(SessionTTL)
	return token, nil
}

// Logout removes a session token.
func (m *Manager) Logout(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

// Valid checks a token and slides its expiry forward.
func (m *Manager) Valid(token string) bool {
	if token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	exp, ok := m.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(m.sessions, token)
		return false
	}
	m.sessions[token] = time.Now().Add(SessionTTL)
	return true
}

func (m *Manager) cleanupLocked() {
	now := time.Now()
	for t, exp := range m.sessions {
		if now.After(exp) {
			delete(m.sessions, t)
		}
	}
	for ip, fs := range m.fails {
		if now.After(fs.until) {
			delete(m.fails, ip)
		}
	}
}

// TokenFromRequest extracts the session token cookie.
func TokenFromRequest(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// SetCookie writes the session cookie; Secure 在 HTTPS（含反代终止 TLS）下自动开启。
func SetCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}

// ClientIP 提取客户端 IP（信任反代传入的首个 X-Forwarded-For；
// RemoteAddr 形如 ip:port，需去掉随机源端口，否则失败计数无法按 IP 累积）。
func ClientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		for i := 0; i < len(xf); i++ {
			if xf[i] == ',' {
				return strings.TrimSpace(xf[:i])
			}
		}
		return strings.TrimSpace(xf)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
