package browser

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
)

func fillReactInput(el *rod.Element, value string) error {
	_, err := el.Eval(`(value) => {
		const el = this
		el.focus()
		const proto = window.HTMLInputElement.prototype
		const desc = Object.getOwnPropertyDescriptor(proto, 'value')
		if (desc && desc.set) {
			desc.set.call(el, value)
		} else {
			el.value = value
		}
		el.dispatchEvent(new Event('input', { bubbles: true }))
		el.dispatchEvent(new Event('change', { bubbles: true }))
		el.dispatchEvent(new Event('blur', { bubbles: true }))
	}`, value)
	return err
}

func fieldValue(el *rod.Element) string {
	return el.MustEval(`() => this.value || ""`).String()
}

func humanTypeField(page *rod.Page, el *rod.Element, value string) error {
	el.MustScrollIntoView().MustClick()
	page.Keyboard.Press(input.ControlLeft)
	page.Keyboard.Press(input.KeyA)
	page.Keyboard.Release(input.KeyA)
	page.Keyboard.Release(input.ControlLeft)
	page.Keyboard.Press(input.Backspace)
	page.Keyboard.Release(input.Backspace)

	for _, ch := range value {
		if err := page.InsertText(string(ch)); err != nil {
			return err
		}
		time.Sleep(45 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	return nil
}

func fillInputField(page *rod.Page, el *rod.Element, value string) error {
	if err := fillReactInput(el, value); err != nil {
		return err
	}
	if fieldValue(el) == value {
		return nil
	}
	if err := humanTypeField(page, el, value); err != nil {
		return err
	}
	if fieldValue(el) != value {
		return fmt.Errorf("field value mismatch: got %q", fieldValue(el))
	}
	return nil
}

func tickReactCheckbox(page *rod.Page, selector string) error {
	el, err := page.Timeout(8 * time.Second).Element(selector)
	if err != nil {
		return fmt.Errorf("checkbox not found (%s): %w", selector, err)
	}

	if el.MustEval(`() => this.checked`).Bool() {
		return nil
	}

	// label / wrapper click (MUI, custom UI)
	label, err := page.Element(selector + " + label")
	if err == nil {
		label.MustScrollIntoView().MustClick()
	} else {
		el.MustScrollIntoView().MustClick()
	}

	time.Sleep(400 * time.Millisecond)
	if el.MustEval(`() => this.checked`).Bool() {
		return nil
	}

	_, err = el.Eval(`() => {
		this.checked = true
		this.dispatchEvent(new Event('input', { bubbles: true }))
		this.dispatchEvent(new Event('change', { bubbles: true }))
		this.dispatchEvent(new Event('click', { bubbles: true }))
	}`)
	if err != nil {
		return err
	}
	if !el.MustEval(`() => this.checked`).Bool() {
		return fmt.Errorf("checkbox still unchecked after click (%s)", selector)
	}
	return nil
}

func clickPreAction(page *rod.Page, selector string) error {
	if strings.Contains(selector, "checkbox") || strings.HasSuffix(selector, `[type="checkbox"]`) {
		return tickReactCheckbox(page, selector)
	}

	el, err := page.Timeout(8 * time.Second).Element(selector)
	if err != nil {
		return fmt.Errorf("pre-click not found (%s): %w", selector, err)
	}
	el.MustScrollIntoView().MustClick()
	time.Sleep(300 * time.Millisecond)
	return nil
}
