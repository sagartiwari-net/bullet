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
	Browser    BrowserConfig    `yaml:"browser"`
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
	Mode     string  `yaml:"mode"` // manual (free) | 2captcha (paid)
	Provider string  `yaml:"provider"`
	APIKey   string  `yaml:"api_key"`
	Type     string  `yaml:"type"`
	SiteKey  string  `yaml:"sitekey"`
	PageURL  string  `yaml:"page_url"`
	Action   string  `yaml:"action"`
	MinScore float64 `yaml:"min_score"`
}

type BrowserConfig struct {
	LoginURL         string `yaml:"login_url"`
	EmailSelector    string `yaml:"email_selector"`
	PassSelector     string `yaml:"pass_selector"`
	SubmitSelector   string `yaml:"submit_selector"`
	ChromePath       string `yaml:"chrome_path"`
	WaitAfterLoad    int    `yaml:"wait_after_load"`
	WaitAfterSubmit  int    `yaml:"wait_after_submit"`
	KeepOpenSeconds  int    `yaml:"keep_open_seconds"`
	Headless         bool   `yaml:"headless"`
	Devtools         bool   `yaml:"devtools"`
	SuccessURL       string `yaml:"success_url"`
	SuccessText      string `yaml:"success_text"`
	FailText         string `yaml:"fail_text"`
	CheckSubscribe   bool   `yaml:"check_subscribe"`
	SubscribeText    string `yaml:"subscribe_text"`
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
	meta, err := ListConfigMeta(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(meta))
	for i, m := range meta {
		names[i] = m.File
	}
	return names, nil
}

type ConfigMeta struct {
	File string `json:"file"`
	Type string `json:"type"`
	Name string `json:"name"`
}

func ListConfigMeta(dir string) ([]ConfigMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var out []ConfigMeta
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		cfg, err := Load(path)
		if err != nil {
			out = append(out, ConfigMeta{File: e.Name(), Type: "http", Name: e.Name()})
			continue
		}
		out = append(out, ConfigMeta{File: e.Name(), Type: cfg.Type, Name: cfg.Name})
	}
	return out, nil
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
