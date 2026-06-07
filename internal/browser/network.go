package browser

import (
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type loginAPICapture struct {
	mu       sync.Mutex
	request  string
	response string
}

func watchLoginAPI(page *rod.Page, pathPart string) *loginAPICapture {
	cap := &loginAPICapture{}
	if pathPart == "" {
		pathPart = "/api/login"
	}

	pending := map[proto.NetworkRequestID]string{}

	go page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			if !strings.Contains(e.Request.URL, pathPart) {
				return
			}
			cap.mu.Lock()
			cap.request = e.Request.PostData
			pending[e.RequestID] = e.Request.URL
			cap.mu.Unlock()
		},
		func(e *proto.NetworkResponseReceived) {
			cap.mu.Lock()
			_, ok := pending[e.RequestID]
			cap.mu.Unlock()
			if !ok {
				return
			}
			go func(id proto.NetworkRequestID) {
				time.Sleep(300 * time.Millisecond)
				res, err := proto.NetworkGetResponseBody{RequestID: id}.Call(page)
				if err != nil {
					return
				}
				cap.mu.Lock()
				cap.response = res.Body
				cap.mu.Unlock()
			}(e.RequestID)
		},
	)()

	return cap
}

func (c *loginAPICapture) Wait(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		done := c.response != ""
		c.mu.Unlock()
		if done {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (c *loginAPICapture) Snapshot() (request, response string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.request, c.response
}

func evaluateLoginAPI(body string) (status, detail string, ok bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", "", false
	}
	if strings.Contains(body, `"success":true`) || strings.Contains(body, `"success": true`) {
		return "HIT", "api login success", true
	}
	if strings.Contains(body, "credentialsInvalid") {
		return "FAIL", "api: credentialsInvalid (agar manual login pass ho to reCAPTCHA/bot block ho sakta hai)", true
	}
	if strings.Contains(strings.ToLower(body), "captcha") || strings.Contains(strings.ToLower(body), "recaptcha") {
		return "RETRY", "api: Recaptcha invalid — Google cookies preserve + auto retry (2x)", true
	}
	if strings.Contains(body, `"success":false`) || strings.Contains(body, `"success": false`) {
		return "FAIL", "api login failed: " + truncate(body, 200), true
	}
	return "", "", false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
