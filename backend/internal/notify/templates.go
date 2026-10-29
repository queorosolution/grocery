package notify

import (
	"fmt"
	"strings"
)

// maxSMSLen keeps every text inside one SMS segment.
const maxSMSLen = 160

// Template names.
const OTP = "otp"

// templates are plain text with {name} slots. Keep each under maxSMSLen with the
// longest values its slots can hold; TestTemplatesFit checks that.
var templates = map[string]string{
	OTP: "{platform}: {code} is your verification code. It works for 10 minutes. Do not share it.",
}

// maxVarLen bounds a slot value so a long name cannot push the text past one segment.
const maxVarLen = 30

// Render fills a template. Values are cleaned (no control characters, no braces,
// at most maxVarLen runes) so user text can never add a link, a new line or a slot.
func Render(name string, vars map[string]string) (string, error) {
	text, ok := templates[name]
	if !ok {
		return "", fmt.Errorf("notify: unknown template %q", name)
	}
	for key, value := range vars {
		text = strings.ReplaceAll(text, "{"+key+"}", clean(value))
	}
	if i := strings.IndexByte(text, '{'); i >= 0 && strings.IndexByte(text[i:], '}') > 0 {
		return "", fmt.Errorf("notify: template %q has a slot without a value", name)
	}
	if len([]rune(text)) > maxSMSLen {
		return "", fmt.Errorf("notify: template %q is longer than %d characters", name, maxSMSLen)
	}
	return text, nil
}

func clean(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == maxVarLen {
			break
		}
		if r < ' ' || r == 0x7f || r == '{' || r == '}' {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}
