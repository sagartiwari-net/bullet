package browser

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
)

func waitRecaptchaReady(page *rod.Page, siteKey, action string, waitSec int) error {
	if siteKey == "" {
		time.Sleep(time.Duration(waitSec) * time.Second)
		simulateHuman(page)
		return nil
	}
	if action == "" {
		action = "login"
	}
	if waitSec <= 0 {
		waitSec = 8
	}

	deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
	for time.Now().Before(deadline) {
		res, err := page.Eval(`async (siteKey, action) => {
			const sleep = (ms) => new Promise(r => setTimeout(r, ms));
			for (let i = 0; i < 40; i++) {
				const g = window.grecaptcha;
				if (g && g.execute) {
					await new Promise(r => g.ready(r));
					try {
						const token = await g.execute(siteKey, { action });
						return { ready: true, token: token || '' };
					} catch (e) {
						return { ready: true, token: '', error: String(e) };
					}
				}
				await sleep(250);
			}
			return { ready: false, token: '' };
		}`, siteKey, action)
		if err != nil {
			return err
		}
		if res.Value.Get("ready").Bool() {
			simulateHuman(page)
			time.Sleep(2 * time.Second)
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("grecaptcha not ready after %ds", waitSec)
}
