package browser

import (
	"context"
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
	LoginURL       string
	EmailSelector  string
	PassSelector   string
	SubmitSelector string
	WaitAfterLoad  int
	WaitAfterSubmit int
	Headless       bool
	SuccessURL     string
	SuccessText    string
	FailText       string
	CheckSubscribe bool
	SubscribeText  string
}

func FromYAML(cfg *config.Config) LoginConfig {
	b := cfg.Browser
	if b.WaitAfterLoad <= 0 {
		b.WaitAfterLoad = 3
	}
	if b.WaitAfterSubmit <= 0 {
		b.WaitAfterSubmit = 5
	}
	return LoginConfig{
		LoginURL:        b.LoginURL,
		EmailSelector:   b.EmailSelector,
		PassSelector:    b.PassSelector,
		SubmitSelector:  b.SubmitSelector,
		WaitAfterLoad:   b.WaitAfterLoad,
		WaitAfterSubmit: b.WaitAfterSubmit,
		Headless:        b.Headless,
		SuccessURL:      b.SuccessURL,
		SuccessText:     b.SuccessText,
		FailText:        b.FailText,
		CheckSubscribe:  b.CheckSubscribe,
		SubscribeText:   b.SubscribeText,
	}
}

func Check(ctx context.Context, email, password string, cfg LoginConfig, rulesSuccess, rulesFail []config.Rule) Result {
	out := Result{Status: "ERROR", Capture: map[string]string{}}

	if cfg.LoginURL == "" || cfg.EmailSelector == "" || cfg.PassSelector == "" {
		out.Detail = "browser config incomplete: login_url, email_selector, pass_selector required"
		return out
	}

	l := launcher.New().
		Headless(cfg.Headless).
		Set("disable-blink-features", "AutomationControlled")

	controlURL, err := l.Launch()
	if err != nil {
		out.Status = "RETRY"
		out.Detail = "chrome launch failed: " + err.Error() + " (install Google Chrome)"
		return out
	}
	defer l.Cleanup()
	defer l.Kill()

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		out.Status = "RETRY"
		out.Detail = "browser connect: " + err.Error()
		return out
	}
	defer browser.MustClose()

	page, err := browser.Page(proto.TargetCreateTarget{URL: cfg.LoginURL})
	if err != nil {
		out.Detail = err.Error()
		return out
	}
	defer page.Close()

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

	if cfg.FailText != "" && strings.Contains(combined, cfg.FailText) {
		out.Status = "FAIL"
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
