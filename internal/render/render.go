// Package render turns Markdown into width-wrapped, ANSI-styled terminal
// lines. It is a self-contained CommonMark/GFM subset renderer with no
// third-party dependencies.
//
// Security note: Markdown from the internet is untrusted input, and terminals
// interpret escape sequences. Sanitize strips every control character
// (including ESC, OSC and C1 codes) and makes bidirectional overrides
// visible, so a hostile document cannot retitle the window, rewrite the
// screen or disguise text.
package render

import (
	"fmt"
	"regexp"
	"strings"
)

// Options configures Render.
type Options struct {
	Width    int    // content width in cells (excluding Margin)
	Margin   int    // blank cells added to the left of every line
	Theme    *Theme // nil renders plain text
	HideURLs bool   // omit "(url)" after link text
}

// Heading is one entry of the document outline.
type Heading struct {
	Level int
	Text  string
	Line  int // index into Result.Lines
}

// Result is a rendered document.
type Result struct {
	Lines    []string
	Headings []Heading
}

// Render converts Markdown source to terminal lines.
func Render(src string, o Options) Result {
	if o.Theme == nil {
		o.Theme = NewTheme("none", false, false)
	}
	o.Width = max(o.Width, 20)
	lines := splitLines(Sanitize(src))
	lines, refs := extractRefs(lines)
	c := &ctx{t: o.Theme, refs: refs, showURLs: !o.HideURLs}
	out := c.blocks(lines, o.Width, 0, false)
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if o.Margin > 0 {
		pad := spaces(o.Margin)
		for i := range out {
			if out[i] != "" {
				out[i] = pad + out[i]
			}
		}
	}
	return Result{Lines: out, Headings: c.headings}
}

// Sanitize removes control characters and neutralises bidi overrides.
func Sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t' || r == '\r':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || r == 0xFEFF:
			// dropped
		case (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || r == 0x200E || r == 0x200F:
			fmt.Fprintf(&b, "<U+%04X>", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = expandTabs(l)
	}
	return lines
}

func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 4 - col%4
			b.WriteString(spaces(n))
			col += n
			continue
		}
		b.WriteRune(r)
		col += RuneWidth(r)
	}
	return b.String()
}

var reRef = regexp.MustCompile(`^ {0,3}\[([^\]^][^\]]*)\]:[ \t]*<?([^\s>]+)>?(?:[ \t]+(?:"[^"]*"|'[^']*'|\([^)]*\)))?[ \t]*$`)

// extractRefs removes [label]: url definitions (outside code fences) and
// returns them keyed by normalised label.
func extractRefs(lines []string) ([]string, map[string]string) {
	refs := map[string]string{}
	out := make([]string, 0, len(lines))
	var f *fence
	for _, l := range lines {
		if f != nil {
			if isCloseFence(l, *f) {
				f = nil
			}
			out = append(out, l)
			continue
		}
		if fn, ok := parseFence(l); ok {
			f = &fn
			out = append(out, l)
			continue
		}
		if m := reRef.FindStringSubmatch(l); m != nil {
			key := normLabel(m[1])
			if _, dup := refs[key]; !dup {
				refs[key] = m[2]
			}
			continue
		}
		out = append(out, l)
	}
	return out, refs
}
