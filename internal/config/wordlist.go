package config

import (
	"strings"
	"unicode"
)

// ParseWordlistLine parses one credential line.
// Formats:
//   - combo / email:pass / user:pass → email:password (first colon split)
//   - tab / email\tpass → email<TAB>password
func ParseWordlistLine(line, format string) (email, password string, ok bool) {
	line = cleanLine(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "combo"
	}

	switch format {
	case "tab", "email\tpass", "email-tab-pass":
		return parseDelimited(line, "\t")
	case "combo", "email:pass", "user:pass", "mail:pass":
		return parseDelimited(line, ":")
	default:
		return parseDelimited(line, ":")
	}
}

func parseDelimited(line, sep string) (email, password string, ok bool) {
	idx := strings.Index(line, sep)
	if idx <= 0 || idx >= len(line)-1 {
		return "", "", false
	}
	email = cleanField(line[:idx])
	password = cleanField(line[idx+len(sep):])
	if email == "" || password == "" {
		return "", "", false
	}
	return email, password, true
}

func cleanLine(line string) string {
	line = strings.TrimPrefix(line, "\ufeff") // UTF-8 BOM
	return strings.TrimSpace(line)
}

func cleanField(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '\t' || r == '\u00a0'
	})
}
