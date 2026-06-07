package config

import "testing"

func TestParseWordlistLine_combo(t *testing.T) {
	cases := []struct {
		line     string
		email    string
		password string
		ok       bool
	}{
		{"bladexlive44@gmail.com:Manoj@1664@#\t", "bladexlive44@gmail.com", "Manoj@1664@#", true},
		{"nguyendung.tube@gmail.com:Wevic2022$\t", "nguyendung.tube@gmail.com", "Wevic2022$", true},
		{"info@socialwave.com.au:$Weditors123", "info@socialwave.com.au", "$Weditors123", true},
		{"otto@knoke.me:Otto@2014", "otto@knoke.me", "Otto@2014", true},
		{"user:pass:extra", "user", "pass:extra", true},
		{"# comment", "", "", false},
		{"", "", "", false},
		{"nocolon", "", "", false},
	}

	for _, c := range cases {
		email, pass, ok := ParseWordlistLine(c.line, "combo")
		if ok != c.ok || email != c.email || pass != c.password {
			t.Fatalf("combo %q => got (%q,%q,%v) want (%q,%q,%v)",
				c.line, email, pass, ok, c.email, c.password, c.ok)
		}
	}
}

func TestParseWordlistLine_tab(t *testing.T) {
	email, pass, ok := ParseWordlistLine("user@test.com\tSecret!@#", "tab")
	if !ok || email != "user@test.com" || pass != "Secret!@#" {
		t.Fatalf("tab format failed: %q %q %v", email, pass, ok)
	}
}
