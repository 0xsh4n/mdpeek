package render

import (
	"html"
	"regexp"
	"strings"
)

var (
	reAuto  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]{1,31}:[^\s<>]*$`)
	reEmail = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
	reTag   = regexp.MustCompile(`^/?([A-Za-z][A-Za-z0-9-]*)(\s[^>]*)?/?$`)
	reAlt   = regexp.MustCompile(`(?i)\balt\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

func isPunct(b byte) bool  { return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", b) >= 0 }
func isSpaceB(b byte) bool { return b == ' ' || b == '\t' || b == '\n' }
func isAlnumB(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}

func runLen(s string, i int, ch byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == ch {
		n++
	}
	return n
}

// findBackticks returns the index of the next run of exactly n backticks at
// or after from, or -1.
func findBackticks(s string, from, n int) int {
	for k := from; k < len(s); k++ {
		if s[k] != '`' {
			continue
		}
		m := runLen(s, k, '`')
		if m == n {
			return k
		}
		k += m - 1
	}
	return -1
}

func normLabel(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// inline parses inline Markdown into styled spans.
func (c *ctx) inline(s string, base Style, fg string) []span {
	var out []span
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			// Entities can decode to control characters (&#27;), so clean again.
			out = append(out, span{text: Sanitize(html.UnescapeString(buf.String())), st: base, fg: fg})
			buf.Reset()
		}
	}
	i := 0
	for i < len(s) {
		ch := s[i]
		switch ch {
		case '\\':
			if i+1 < len(s) && isPunct(s[i+1]) {
				buf.WriteByte(s[i+1])
				i += 2
				continue
			}
			if i+1 < len(s) && s[i+1] == '\n' {
				flush()
				out = append(out, span{text: "\n", st: base, fg: fg})
				i += 2
				continue
			}
			buf.WriteByte(ch)
			i++
		case '`':
			n := runLen(s, i, '`')
			if j := findBackticks(s, i+n, n); j >= 0 {
				flush()
				content := strings.ReplaceAll(s[i+n:j], "\n", " ")
				if len(content) >= 2 && content[0] == ' ' && content[len(content)-1] == ' ' && strings.TrimSpace(content) != "" {
					content = content[1 : len(content)-1]
				}
				if !c.t.Color {
					content = "`" + content + "`"
				}
				out = append(out, span{text: content, st: base | Code, fg: fg})
				i = j + n
			} else {
				buf.WriteString(s[i : i+n])
				i += n
			}
		case '!':
			if i+1 < len(s) && s[i+1] == '[' {
				if text, _, end, ok := c.parseLink(s, i+1); ok {
					flush()
					out = append(out, span{text: imageLabel(plainOf(c.inline(text, 0, ""))), st: base | Dim, fg: fg})
					i = end
					continue
				}
			}
			buf.WriteByte(ch)
			i++
		case '[':
			if text, dest, end, ok := c.parseLink(s, i); ok {
				flush()
				inner := c.inline(text, base|Link, fg)
				out = append(out, inner...)
				if c.showURLs && dest != "" && !strings.HasPrefix(dest, "#") && dest != plainOf(inner) {
					out = append(out, span{text: " (" + dest + ")", st: base | URL, fg: fg})
				}
				i = end
				continue
			}
			buf.WriteByte(ch)
			i++
		case '<':
			if n, sp, ok := c.parseAngle(s, i, base, fg); ok {
				flush()
				out = append(out, sp...)
				i += n
				continue
			}
			buf.WriteByte(ch)
			i++
		case '*', '_', '~':
			if n, sp, ok := c.parseEmphasis(s, i, base, fg, &buf, flush); ok {
				out = append(out, sp...)
				i += n
				continue
			}
			j := i + runLen(s, i, ch)
			buf.WriteString(s[i:j])
			i = j
		case 'h':
			if (strings.HasPrefix(s[i:], "http://") || strings.HasPrefix(s[i:], "https://")) && (i == 0 || !isAlnumB(s[i-1])) {
				k := i
				for k < len(s) && !isSpaceB(s[k]) && s[k] != '<' {
					k++
				}
				u := trimURL(s[i:k])
				if len(u) > 8 {
					flush()
					out = append(out, span{text: u, st: base | Link, fg: fg})
					i += len(u)
					continue
				}
			}
			buf.WriteByte(ch)
			i++
		default:
			buf.WriteByte(ch)
			i++
		}
	}
	flush()
	return out
}

func imageLabel(alt string) string {
	if alt = strings.TrimSpace(alt); alt != "" {
		return "[image: " + alt + "]"
	}
	return "[image]"
}

func trimURL(u string) string {
	for len(u) > 0 {
		last := u[len(u)-1]
		switch {
		case strings.IndexByte(".,;:!?'\"*_~", last) >= 0:
			u = u[:len(u)-1]
		case last == ')' && strings.Count(u, "(") < strings.Count(u, ")"):
			u = u[:len(u)-1]
		case last == ']' && strings.Count(u, "[") < strings.Count(u, "]"):
			u = u[:len(u)-1]
		default:
			return u
		}
	}
	return u
}

// parseAngle handles <autolinks>, inline HTML tags and HTML comments.
func (c *ctx) parseAngle(s string, i int, base Style, fg string) (int, []span, bool) {
	if strings.HasPrefix(s[i:], "<!--") {
		if k := strings.Index(s[i:], "-->"); k >= 0 {
			return k + 3, nil, true
		}
		return 0, nil, false
	}
	e := strings.IndexByte(s[i:], '>')
	if e < 2 {
		return 0, nil, false
	}
	inner := s[i+1 : i+e]
	switch {
	case reAuto.MatchString(inner) || reEmail.MatchString(inner):
		return e + 1, []span{{text: inner, st: base | Link, fg: fg}}, true
	case reTag.MatchString(inner):
		name := strings.ToLower(reTag.FindStringSubmatch(inner)[1])
		switch name {
		case "br":
			return e + 1, []span{{text: "\n", st: base, fg: fg}}, true
		case "img":
			alt := ""
			if m := reAlt.FindStringSubmatch(inner); m != nil {
				alt = m[1] + m[2]
			}
			return e + 1, []span{{text: imageLabel(alt), st: base | Dim, fg: fg}}, true
		}
		return e + 1, nil, true
	}
	return 0, nil, false
}

// parseEmphasis handles *em*, **strong**, ***both***, _em_, __strong__ and
// ~~strike~~. On success it returns the number of bytes consumed.
func (c *ctx) parseEmphasis(s string, i int, base Style, fg string, buf *strings.Builder, flush func()) (int, []span, bool) {
	ch := s[i]
	n := runLen(s, i, ch)
	if i+n >= len(s) || isSpaceB(s[i+n]) {
		return 0, nil, false
	}
	if ch == '_' && i > 0 && isAlnumB(s[i-1]) {
		return 0, nil, false
	}
	if ch == '~' {
		if n != 2 {
			return 0, nil, false
		}
		j := findClose(s, i+n, ch, 2)
		if j < 0 {
			return 0, nil, false
		}
		flush()
		return j + 2 - i, c.inline(s[i+n:j], base|Strike, fg), true
	}
	for u := min(n, 3); u >= 1; u-- {
		j := findClose(s, i+n, ch, u)
		if j < 0 {
			continue
		}
		buf.WriteString(strings.Repeat(string(ch), n-u))
		flush()
		st := base
		switch u {
		case 1:
			st |= Italic
		case 2:
			st |= Bold
		default:
			st |= Bold | Italic
		}
		return j + u - i, c.inline(s[i+n:j], st, fg), true
	}
	return 0, nil, false
}

// findClose finds a closing delimiter run of exactly u copies of ch at or
// after from, skipping code spans and escapes.
func findClose(s string, from int, ch byte, u int) int {
	for k := from; k < len(s); k++ {
		switch s[k] {
		case '\\':
			k++
		case '`':
			n := runLen(s, k, '`')
			if j := findBackticks(s, k+n, n); j >= 0 {
				k = j + n - 1
			} else {
				k += n - 1
			}
		case ch:
			r := runLen(s, k, ch)
			if r == u && k > from && !isSpaceB(s[k-1]) {
				if ch == '_' && k+u < len(s) && isAlnumB(s[k+u]) {
					k += r - 1
					continue
				}
				return k
			}
			k += r - 1
		}
	}
	return -1
}

// parseLink parses [text](dest "title"), [text][ref], [text][] and [ref].
// s[i] must be '['. It returns the link text, destination and end index.
func (c *ctx) parseLink(s string, i int) (text, dest string, end int, ok bool) {
	j := matchBracket(s, i)
	if j < 0 {
		return
	}
	text = s[i+1 : j]
	rest := j + 1
	switch {
	case rest < len(s) && s[rest] == '(':
		d, e, good := parseDest(s, rest)
		if !good {
			return "", "", 0, false
		}
		return text, d, e, true
	case rest < len(s) && s[rest] == '[':
		if k := strings.IndexByte(s[rest:], ']'); k >= 0 {
			label := s[rest+1 : rest+k]
			if label == "" {
				label = text
			}
			if d, found := c.refs[normLabel(label)]; found {
				return text, d, rest + k + 1, true
			}
		}
		return "", "", 0, false
	}
	if d, found := c.refs[normLabel(text)]; found {
		return text, d, rest, true
	}
	return "", "", 0, false
}

func matchBracket(s string, i int) int {
	depth := 0
	for k := i; k < len(s); k++ {
		switch s[k] {
		case '\\':
			k++
		case '`':
			n := runLen(s, k, '`')
			if j := findBackticks(s, k+n, n); j >= 0 {
				k = j + n - 1
			} else {
				k += n - 1
			}
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return k
			}
		}
	}
	return -1
}

func parseDest(s string, p int) (string, int, bool) {
	k := p + 1
	skip := func() {
		for k < len(s) && isSpaceB(s[k]) {
			k++
		}
	}
	skip()
	var dest string
	if k < len(s) && s[k] == '<' {
		e := strings.IndexByte(s[k:], '>')
		if e < 0 {
			return "", 0, false
		}
		dest = s[k+1 : k+e]
		k += e + 1
	} else {
		depth, start := 0, k
	loop:
		for k < len(s) {
			switch ch := s[k]; {
			case ch == '\\' && k+1 < len(s):
				k += 2
				continue
			case isSpaceB(ch):
				break loop
			case ch == '(':
				depth++
			case ch == ')':
				if depth == 0 {
					break loop
				}
				depth--
			}
			k++
		}
		dest = s[start:k]
	}
	skip()
	if k < len(s) && (s[k] == '"' || s[k] == '\'' || s[k] == '(') {
		closer := s[k]
		if closer == '(' {
			closer = ')'
		}
		e := strings.IndexByte(s[k+1:], closer)
		if e < 0 {
			return "", 0, false
		}
		k += e + 2
		skip()
	}
	if k < len(s) && s[k] == ')' {
		return unescapeMD(dest), k + 1, true
	}
	return "", 0, false
}

func unescapeMD(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && isPunct(s[i+1]) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
