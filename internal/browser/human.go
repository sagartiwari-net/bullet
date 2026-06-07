package browser

import (
	"math/rand"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func simulateHuman(page *rod.Page) {
	// Mouse moves + scroll — reCAPTCHA v3 score ke liye
	for i := 0; i < 4; i++ {
		x := 200 + rand.Intn(600)
		y := 150 + rand.Intn(400)
		_ = page.Mouse.MoveTo(proto.Point{X: float64(x), Y: float64(y)})
		time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)
	}
	_, _ = page.Eval(`() => window.scrollBy(0, 120)`)
	time.Sleep(400 * time.Millisecond)
	_, _ = page.Eval(`() => window.scrollBy(0, -60)`)
	time.Sleep(300 * time.Millisecond)
}

func dismissCookieBanner(page *rod.Page) {
	selectors := []string{
		`button[id*="accept"]`,
		`button[class*="accept"]`,
		`button:has-text("Accept All")`,
		`button:has-text("Accept")`,
		`[data-testid="accept"]`,
	}
	for _, sel := range selectors {
		el, err := page.Timeout(2 * time.Second).Element(sel)
		if err != nil {
			continue
		}
		el.MustClick()
		time.Sleep(500 * time.Millisecond)
		return
	}
}
