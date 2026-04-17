package phoneutil

import "strings"

// NormalizeDigits strips non-digits for loose matching (E.164 vs local formats).
func NormalizeDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
