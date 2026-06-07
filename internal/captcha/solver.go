package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Enabled  bool    `yaml:"enabled"`
	Provider string  `yaml:"provider"`
	APIKey   string  `yaml:"api_key"`
	Type     string  `yaml:"type"`
	SiteKey  string  `yaml:"sitekey"`
	PageURL  string  `yaml:"page_url"`
	Action   string  `yaml:"action"`
	MinScore float64 `yaml:"min_score"`
}

type Solver struct {
	client *http.Client
}

func NewSolver() *Solver {
	return &Solver{client: &http.Client{Timeout: 120 * time.Second}}
}

func (c *Config) apiKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	return os.Getenv("CAPTCHA_API_KEY")
}

func (s *Solver) Solve(ctx context.Context, cfg Config) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}
	key := cfg.apiKey()
	if key == "" {
		return "", fmt.Errorf("captcha enabled but no api_key (set in config or CAPTCHA_API_KEY env)")
	}
	if cfg.SiteKey == "" || cfg.PageURL == "" {
		return "", fmt.Errorf("captcha sitekey and page_url required")
	}

	provider := strings.ToLower(cfg.Provider)
	if provider == "" || provider == "2captcha" {
		return s.solve2Captcha(ctx, key, cfg)
	}
	return "", fmt.Errorf("unsupported captcha provider: %s", cfg.Provider)
}

func (s *Solver) solve2Captcha(ctx context.Context, apiKey string, cfg Config) (string, error) {
	params := url.Values{}
	params.Set("key", apiKey)
	params.Set("json", "1")

	switch strings.ToLower(cfg.Type) {
	case "turnstile":
		params.Set("method", "turnstile")
		params.Set("sitekey", cfg.SiteKey)
		params.Set("pageurl", cfg.PageURL)
	case "recaptcha_v3", "recaptchav3", "v3":
		params.Set("method", "userrecaptcha")
		params.Set("googlekey", cfg.SiteKey)
		params.Set("pageurl", cfg.PageURL)
		params.Set("version", "v3")
		action := cfg.Action
		if action == "" {
			action = "login"
		}
		params.Set("action", action)
		minScore := cfg.MinScore
		if minScore <= 0 {
			minScore = 0.3
		}
		params.Set("min_score", fmt.Sprintf("%.1f", minScore))
	default:
		params.Set("method", "userrecaptcha")
		params.Set("googlekey", cfg.SiteKey)
		params.Set("pageurl", cfg.PageURL)
	}

	submitURL := "https://2captcha.com/in.php?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, submitURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("2captcha submit: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var submitResp struct {
		Status  int    `json:"status"`
		Request string `json:"request"`
	}
	if err := json.Unmarshal(body, &submitResp); err != nil {
		return "", fmt.Errorf("2captcha submit parse: %s", string(body))
	}
	if submitResp.Status != 1 {
		return "", fmt.Errorf("2captcha submit error: %s", submitResp.Request)
	}

	taskID := submitResp.Request
	deadline := time.Now().Add(120 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		resultURL := fmt.Sprintf(
			"https://2captcha.com/res.php?key=%s&action=get&id=%s&json=1",
			url.QueryEscape(apiKey),
			url.QueryEscape(taskID),
		)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
		resp, err := s.client.Do(req)
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result struct {
			Status  int    `json:"status"`
			Request string `json:"request"`
		}
		if json.Unmarshal(raw, &result) != nil {
			continue
		}
		if result.Status == 1 {
			return result.Request, nil
		}
		if result.Request != "CAPCHA_NOT_READY" {
			return "", fmt.Errorf("2captcha solve error: %s", result.Request)
		}
	}
	return "", fmt.Errorf("2captcha timeout waiting for solution")
}
