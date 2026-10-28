package render

import (
	"strings"
	"unicode"
)

type span struct {
	text string
	st   Style
	fg   string
}

type tok struct {
	s  string
	st Style
	fg string
	w  int
}

// word is an unbreakable run of styled tokens. br marks a forced line break.
type word struct {
	parts []tok
	w     int
	br    bool
}

func plainOf(spans []span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.text)
	}
	return b.String()
}

func toksWidth(ts []tok) int {
	w := 0
	for _, t := range ts {
		w += t.w
	}
	return w
}

// toWords splits styled spans into words on whitespace. Inline code spans
// are kept intact, so `a b` never breaks across lines unless it must.
func toWords(spans []span) []word {
	var words []word
	var cur []tok
	cw := 0
	flush := func() {
		if len(cur) > 0 {
			words = append(words, word{parts: cur, w: cw})
			cur, cw = nil, 0
		}
	}
	add := func(s string, st Style, fg string) {
		w := Width(s)
		if n := len(cur); n > 0 && cur[n-1].st == st && cur[n-1].fg == fg {
			cur[n-1].s += s
			cur[n-1].w += w
		} else {
			cur = append(cur, tok{s, st, fg, w})
		}
		cw += w
	}
	for _, sp := range spans {
		if sp.st&Code != 0 {
			s := strings.Map(func(r rune) rune {
				if r == '\n' || r == '\t' {
					return ' '
				}
				return r
			}, sp.text)
			if s != "" {
				add(s, sp.st, sp.fg)
			}
			continue
		}
		var b strings.Builder
		flushB := func() {
			if b.Len() > 0 {
				add(b.String(), sp.st, sp.fg)
				b.Reset()
			}
		}
		for _, r := range sp.text {
			switch {
			case r == '\n':
				flushB()
				flush()
				words = append(words, word{br: true})
			case r != 0xA0 && unicode.IsSpace(r):
				flushB()
				flush()
			default:
				b.WriteRune(r)
			}
		}
		flushB()
	}
	flush()
	return words
}

// splitToks hard-wraps a token run into chunks of at most maxw cells.
func splitToks(parts []tok, maxw int) [][]tok {
	var chunks [][]tok
	var cur []tok
	cw := 0
	for _, p := range parts {
		var b strings.Builder
		bw := 0
		flush := func() {
			if b.Len() > 0 {
				cur = append(cur, tok{b.String(), p.st, p.fg, bw})
				b.Reset()
				bw = 0
			}
		}
		for _, r := range p.s {
			rw := RuneWidth(r)
			if cw+rw > maxw && cw > 0 {
				flush()
				chunks = append(chunks, cur)
				cur, cw = nil, 0
			}
			b.WriteRune(r)
			bw += rw
			cw += rw
		}
		flush()
	}
	if len(cur) > 0 || len(chunks) == 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

func splitWord(w word, max int) []word {
	chunks := splitToks(w.parts, max)
	res := make([]word, len(chunks))
	for i, ch := range chunks {
		res[i] = word{parts: ch, w: toksWidth(ch)}
	}
	return res
}

// wrapWords greedily fills lines of at most width cells.
func wrapWords(ws []word, width int) [][]tok {
	if width < 1 {
		width = 1
	}
	var lines [][]tok
	var cur []tok
	curW := 0
	place := func(w word) {
		switch {
		case curW == 0:
			cur = append(cur, w.parts...)
			curW = w.w
		case curW+1+w.w <= width:
			cur = append(cur, tok{s: " ", w: 1})
			cur = append(cur, w.parts...)
			curW += 1 + w.w
		default:
			lines = append(lines, cur)
			cur = append([]tok(nil), w.parts...)
			curW = w.w
		}
	}
	for _, w := range ws {
		if w.br {
			lines = append(lines, cur)
			cur, curW = nil, 0
			continue
		}
		if w.w > width {
			for _, p := range splitWord(w, width) {
				place(p)
			}
			continue
		}
		place(w)
	}
	if len(cur) > 0 || len(lines) == 0 {
		lines = append(lines, cur)
	}
	return lines
}

func (c *ctx) wrap(spans []span, width int) []string {
	lines := wrapWords(toWords(spans), width)
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = c.t.emit(l)
	}
	return out
}
