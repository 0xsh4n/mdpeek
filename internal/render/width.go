package render

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// RuneWidth returns the number of terminal cells r occupies (0, 1 or 2).
func RuneWidth(r rune) int {
	switch {
	case r == 0 || r < 0x20 || (r >= 0x7f && r < 0xa0):
		return 0
	case r < 0x300:
		return 1
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	case isWide(r):
		return 2
	}
	return 1
}

func isWide(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE30 && r <= 0xFE6F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F300 && r <= 0x1F64F) ||
		(r >= 0x1F900 && r <= 0x1F9FF) ||
		(r >= 0x20000 && r <= 0x3FFFD)
}

// skipEsc returns the index just past the escape sequence starting at s[i].
func skipEsc(s string, i int) int {
	i++ // ESC
	if i >= len(s) {
		return i
	}
	if s[i] != '[' {
		return i + 1
	}
	i++
	for i < len(s) {
		c := s[i]
		i++
		if c >= 0x40 && c <= 0x7e {
			break
		}
	}
	return i
}

// Width returns the display width of s, ignoring ANSI escape sequences.
func Width(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = skipEsc(s, i)
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		w += RuneWidth(r)
		i += n
	}
	return w
}

// Strip removes ANSI escape sequences from s.
func Strip(s string) string {
	if strings.IndexByte(s, 0x1b) < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = skipEsc(s, i)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}
