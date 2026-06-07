package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name           string `yaml:"name"`
	Type           string `yaml:"type"`
	WordlistFormat string `yaml:"wordlist_format"`

	Cloudflare CloudflareConfig `yaml:"cloudflare"`
	Captcha    CaptchaConfig    `yaml:"captcha"`
	Request    RequestConfig    `yaml:"request"`
	FollowUp   FollowUpConfig   `yaml:"follow_up"`
	Proxy      bool             `yaml:"proxy"`
	Timeout    int              `yaml:"timeout"`

	Success []Rule `yaml:"success"`
	Fail    []Rule `yaml:"fail"`

	Capture []CaptureRule `yaml:"capture"`
}

type CaptchaConfig struct {
	Enabled  bool    `yaml:"enabled"`
	Provider string  `yaml:"provider"`
	APIKey   string  `yaml:"api_key"`
	Type     string  `yaml:"type"`
	SiteKey  string  `yaml:"sitekey"`
	PageURL  string  `yaml:"page_url"`
	Action   string  `yaml:"action"`
	MinScore float64 `yaml:"min_score"`
}

type FollowUpConfig struct {
	Enabled      bool              `yaml:"enabled"`
	Method       string            `yaml:"method"`
	URL          string            `yaml:"url"`
	Headers      map[string]string `yaml:"headers"`
	Subscription SubscriptionCheck `yaml:"subscription"`
}

type SubscriptionCheck struct {
	Type   string `yaml:"type"`
	Value  string `yaml:"value"`
	SaveAs string `yaml:"save_as"`
	Active string `yaml:"active_label"`
	None   string `yaml:"none_label"`
}

type CloudflareConfig struct {
	Enabled bool `yaml:"enabled"`
	Mode    string `yaml:"mode"`

	Solver struct {
		Type string `yaml:"type"`
		URL  string `yaml:"url"`
	} `yaml:"solver"`

	CookieCache struct {
		Enabled       bool   `yaml:"enabled"`
		TTL           string `yaml:"ttl"`
		RefreshBefore string `yaml:"refresh_before"`
	} `yaml:"cookie_cache"`
}

type RequestConfig struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
}

type Rule struct {
	Type      string `yaml:"type"`
	Path      string `yaml:"path"`
	Condition string `yaml:"condition"`
	Value     string `yaml:"value"`
	Action    string `yaml:"action"`
	Retry     bool   `yaml:"retry"`
}

type CaptureRule struct {
	Type   string `yaml:"type"`
	Path   string `yaml:"path"`
	SaveAs string `yaml:"save_as"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 15
	}
	if cfg.WordlistFormat == "" {
		cfg.WordlistFormat = "email:pass"
	}
	if cfg.Type == "" {
		cfg.Type = "http"
	}
	if cfg.Cloudflare.Solver.URL == "" {
		cfg.Cloudflare.Solver.URL = "http://localhost:8191"
	}

	return &cfg, nil
}

func ListConfigs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func ConfigPath(dir, name string) string {
	return filepath.Join(dir, name)
}

func ParseWordlistLine(line, format string) (email, password string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}

	sep := ":"
	if format == "user:pass" {
		// same separator
	}

	parts := strings.SplitN(line, sep, 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}
