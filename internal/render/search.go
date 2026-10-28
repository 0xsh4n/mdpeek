package render

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// LowerRunes lower-cases rs rune by rune, preserving length so indexes stay
// aligned with the original text.
func LowerRunes(rs []rune) []rune {
	out := make([]rune, len(rs))
	for i, r := range rs {
		out[i] = unicode.ToLower(r)
	}
	return out
}

// FindAll returns [start,end) rune ranges of needle inside plain. When fold
// is true, plain is compared case-insensitively and needle must already be
// lower-case.
func FindAll(plain, needle []rune, fold bool) [][2]int {
	if len(needle) == 0 || len(plain) < len(needle) {
		return nil
	}
	hay := plain
	if fold {
		hay = LowerRunes(plain)
	}
	var res [][2]int
	for i := 0; i+len(needle) <= len(hay); {
		ok := true
		for k, r := range needle {
			if hay[i+k] != r {
				ok = false
				break
			}
		}
		if ok {
			res = append(res, [2]int{i, i + len(needle)})
			i += len(needle)
		} else {
			i++
		}
	}
	return res
}

// Highlight wraps every match of needle in line with the on/off sequences,
// leaving the line's own styling intact.
func Highlight(line string, needle []rune, fold bool, on, off string) string {
	ms := FindAll([]rune(Strip(line)), needle, fold)
	if len(ms) == 0 {
		return line
	}
	var b strings.Builder
	vis, mi := 0, 0
	inM := false
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			j := skipEsc(line, i)
			b.WriteString(line[i:j])
			if inM {
				b.WriteString(on) // the line's own resets must not end the match early
			}
			i = j
			continue
		}
		_, n := utf8.DecodeRuneInString(line[i:])
		if !inM && mi < len(ms) && vis == ms[mi][0] {
			b.WriteString(on)
			inM = true
		}
		b.WriteString(line[i : i+n])
		vis++
		i += n
		if inM && vis == ms[mi][1] {
			b.WriteString(off)
			inM = false
			mi++
		}
	}
	if inM {
		b.WriteString(off)
	}
	return b.String()
}
