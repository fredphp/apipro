package wschat

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"unicode"
)

func genMsgId() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "m" + hex.EncodeToString(b)
}

// isSafeRoom allows only alphanumeric room ids.
func isSafeRoom(r string) bool {
	if r == "" || len(r) > 32 {
		return false
	}
	for _, ch := range r {
		if !(unicode.IsLetter(ch) || unicode.IsDigit(ch)) {
			return false
		}
	}
	return true
}

// sanitize escapes HTML special chars and trims/limits length. Prevents XSS in chat.
func sanitize(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = s[:500]
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&#34;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
