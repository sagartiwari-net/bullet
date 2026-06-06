package cf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Session struct {
	Cookies   map[string]string
	UserAgent string
	ProxyURL  string
	ExpiresAt time.Time
}

func (s *Session) IsValid() bool {
	return s != nil && time.Now().Before(s.ExpiresAt)
}

func (s *Session) CookieHeader() string {
	if s == nil || len(s.Cookies) == 0 {
		return ""
	}
	var parts []string
	for k, v := range s.Cookies {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return stringsJoin(parts, "; ")
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += sep + parts[i]
	}
	return out
}

type Manager struct {
	flareURL string
	ttl      time.Duration
	sessions map[string]*Session
	mu       sync.RWMutex
	client   *http.Client
}

func NewManager(flareURL string, ttlMinutes int) *Manager {
	if ttlMinutes <= 0 {
		ttlMinutes = 25
	}
	return &Manager{
		flareURL: flareURL,
		ttl:      time.Duration(ttlMinutes) * time.Minute,
		sessions: make(map[string]*Session),
		client:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (m *Manager) GetSession(ctx context.Context, targetURL, proxyURL string) (*Session, error) {
	key := sessionKey(targetURL, proxyURL)

	m.mu.RLock()
	sess, ok := m.sessions[key]
	m.mu.RUnlock()

	if ok && sess.IsValid() {
		return sess, nil
	}

	return m.Solve(ctx, targetURL, proxyURL)
}

func (m *Manager) Solve(ctx context.Context, targetURL, proxyURL string) (*Session, error) {
	reqBody := flareRequest{
		Cmd:        "request.get",
		URL:        targetURL,
		MaxTimeout: 60000,
	}
	if proxyURL != "" {
		reqBody.Proxy = map[string]string{"url": proxyURL}
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.flareURL+"/v1", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr unreachable (is docker running?): %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var result flareResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("flaresolverr bad response: %w", err)
	}
	if result.Status != "ok" {
		return nil, fmt.Errorf("flaresolverr error: %s", result.Message)
	}

	cookies := make(map[string]string)
	for _, c := range result.Solution.Cookies {
		cookies[c.Name] = c.Value
	}

	sess := &Session{
		Cookies:   cookies,
		UserAgent: result.Solution.UserAgent,
		ProxyURL:  proxyURL,
		ExpiresAt: time.Now().Add(m.ttl),
	}

	key := sessionKey(targetURL, proxyURL)
	m.mu.Lock()
	m.sessions[key] = sess
	m.mu.Unlock()

	return sess, nil
}

func (m *Manager) Invalidate(targetURL, proxyURL string) {
	key := sessionKey(targetURL, proxyURL)
	m.mu.Lock()
	delete(m.sessions, key)
	m.mu.Unlock()
}

func sessionKey(targetURL, proxyURL string) string {
	return targetURL + "|" + proxyURL
}

type flareRequest struct {
	Cmd        string            `json:"cmd"`
	URL        string            `json:"url"`
	MaxTimeout int               `json:"maxTimeout"`
	Proxy      map[string]string `json:"proxy,omitempty"`
}

type flareResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Solution struct {
		URL       string `json:"url"`
		UserAgent string `json:"userAgent"`
		Cookies   []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"cookies"`
	} `json:"solution"`
}
