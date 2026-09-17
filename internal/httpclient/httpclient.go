package httpclient

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	connectTimeout       = 10 * time.Second
	responseHeaderTimeout = 60 * time.Second
	streamIdleTimeout    = 30 * time.Second
	maxBodyRead          = 32 << 20 // 32MB cap for buffered reads
)

// ProxyConfig is the global network proxy setting shared by all AI requests.
type ProxyConfig struct {
	Mode string `json:"mode"` // "none" | "proxy"
	URL  string `json:"url"`
}

// Manager builds and caches a shared *http.Client from the current proxy
// settings. All provider requests (models / quota / test / warmup) go through
// Manager.Do so proxy changes apply globally without restart.
type Manager struct {
	mu     sync.RWMutex
	client *http.Client
	cfg    ProxyConfig
}

func NewManager() *Manager {
	m := &Manager{}
	m.rebuild(ProxyConfig{Mode: "none"})
	return m
}

func (m *Manager) Config() ProxyConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) Update(cfg ProxyConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rebuild(cfg)
}

func (m *Manager) rebuild(cfg ProxyConfig) error {
	transport := &http.Transport{
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   connectTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ForceAttemptHTTP2:     true,
	}
	if cfg.Mode == "proxy" && cfg.URL != "" {
		u, err := url.Parse(cfg.URL)
		if err != nil {
			return err
		}
		transport.Proxy = http.ProxyURL(u)
	}
	// No overall client timeout: streaming responses rely on the idle-timeout
	// wrapper plus caller-level context deadlines.
	m.client = &http.Client{Transport: &idleTimeoutTransport{Base: transport}}
	m.cfg = cfg
	return nil
}

// Do executes the request with the shared client. A 5-minute hard cap is
// applied for non-streaming requests via context so quota/model queries can
// never hang forever.
func (m *Manager) Do(req *http.Request) (*http.Response, error) {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if req.Context() == context.Background() {
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Minute)
		defer cancel()
		req = req.Clone(ctx)
	}
	return client.Do(req)
}

// idleTimeoutTransport wraps every response body with an idle watchdog: if no
// bytes arrive within streamIdleTimeout, the body is closed and pending reads
// fail — this is the "stream idle timeout" for SSE connections.
type idleTimeoutTransport struct {
	Base http.RoundTripper
}

func (t *idleTimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.Base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = newIdleBody(resp.Body, streamIdleTimeout)
	return resp, nil
}

type idleBody struct {
	inner     readCloser
	timer     *time.Timer
	idleLimit time.Duration
	closeOnce sync.Once
}

type readCloser = interface {
	Read(p []byte) (int, error)
	Close() error
}

func newIdleBody(inner readCloser, idle time.Duration) *idleBody {
	b := &idleBody{inner: inner, idleLimit: idle}
	b.timer = time.AfterFunc(idle, func() {
		// No data for the whole window: kill the connection so a blocked
		// Read returns promptly.
		_ = inner.Close()
	})
	return b
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.inner.Read(p)
	if n > 0 {
		b.timer.Reset(b.idleLimit)
	}
	return n, err
}

func (b *idleBody) Close() error {
	var err error
	b.closeOnce.Do(func() {
		b.timer.Stop()
		err = b.inner.Close()
	})
	return err
}
