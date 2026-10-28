package render

import (
	"os"
	"strings"
)

// Style is a bit set of inline text attributes.
type Style uint16

const (
	Bold Style = 1 << iota
	Italic
	Strike
	Code
	Link
	Dim
	URL
)

// Theme holds the colour palette and glyph set used when rendering. A Theme
// with Color == false emits no escape sequences at all.
type Theme struct {
	Name  string
	Color bool
	ASCII bool

	heading                                       [7]string
	text, code, codeBlock, link, url, quote, rule string
	bullet, border, dim, kw, str, com, num        string
}

// DetectLight guesses whether the terminal has a light background from the
// COLORFGBG variable (set by rxvt-style terminals and a few others).
func DetectLight() bool {
	v := os.Getenv("COLORFGBG")
	if v == "" {
		return false
	}
	parts := strings.Split(v, ";")
	last := parts[len(parts)-1]
	return last == "7" || last == "15"
}

// NewTheme builds a theme. name is "dark", "light" or "auto".
func NewTheme(name string, color, ascii bool) *Theme {
	if name == "" || name == "auto" {
		name = "dark"
		if DetectLight() {
			name = "light"
		}
	}
	t := &Theme{Name: name, Color: color, ASCII: ascii}
	if name == "light" {
		t.heading = [7]string{"", "1;38;5;125", "1;38;5;25", "1;38;5;28", "1;38;5;94", "1;38;5;60", "1;38;5;240"}
		t.text, t.code, t.codeBlock = "38;5;236", "38;5;88;48;5;254", "48;5;254"
		t.link, t.url, t.quote, t.rule = "38;5;26", "38;5;245", "38;5;243", "38;5;250"
		t.bullet, t.border, t.dim = "38;5;130", "38;5;250", "38;5;243"
		t.kw, t.str, t.com, t.num = "38;5;125", "38;5;28", "3;38;5;245", "38;5;130"
		return t
	}
	t.heading = [7]string{"", "1;38;5;213", "1;38;5;81", "1;38;5;149", "1;38;5;180", "1;38;5;146", "1;38;5;245"}
	t.text, t.code, t.codeBlock = "38;5;252", "38;5;216;48;5;236", "48;5;235"
	t.link, t.url, t.quote, t.rule = "38;5;75", "38;5;244", "38;5;244", "38;5;240"
	t.bullet, t.border, t.dim = "38;5;214", "38;5;240", "38;5;244"
	t.kw, t.str, t.com, t.num = "38;5;204", "38;5;114", "3;38;5;244", "38;5;215"
	return t
}

// sgr returns the escape sequence that selects style st plus the extra
// SGR parameters fg, starting from a reset state.
func (t *Theme) sgr(st Style, fg string) string {
	if !t.Color {
		return ""
	}
	p := make([]string, 0, 8)
	p = append(p, "0")
	if st&Bold != 0 {
		p = append(p, "1")
	}
	if st&Dim != 0 {
		p = append(p, "2")
	}
	if st&Italic != 0 {
		p = append(p, "3")
	}
	if st&Strike != 0 {
		p = append(p, "9")
	}
	if st&Code != 0 {
		p = append(p, t.code)
	}
	if st&Link != 0 {
		p = append(p, "4", t.link)
	}
	if st&URL != 0 {
		p = append(p, t.url)
	}
	if fg != "" {
		p = append(p, fg)
	}
	return "\x1b[" + strings.Join(p, ";") + "m"
}

// paint wraps s in the SGR parameters params.
func (t *Theme) paint(params, s string) string {
	if !t.Color || params == "" || s == "" {
		return s
	}
	return "\x1b[" + params + "m" + s + "\x1b[0m"
}

// emit serialises a run of tokens into one styled string.
func (t *Theme) emit(parts []tok) string {
	var b strings.Builder
	cs, cfg := ^Style(0), "\x00"
	for _, p := range parts {
		if p.st != cs || p.fg != cfg {
			b.WriteString(t.sgr(p.st, p.fg))
			cs, cfg = p.st, p.fg
		}
		b.WriteString(p.s)
	}
	if t.Color && len(parts) > 0 {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

type glyphs struct {
	bullets                            [3]string
	quote, hr, h1, h2, h, v            string
	tl, tt, tr, ml, mm, mr, bl, bt, br string
}

func (t *Theme) glyphs() glyphs {
	if t.ASCII {
		return glyphs{
			bullets: [3]string{"*", "-", "+"}, quote: "|", hr: "-", h1: "=", h2: "-", h: "-", v: "|",
			tl: "+", tt: "+", tr: "+", ml: "+", mm: "+", mr: "+", bl: "+", bt: "+", br: "+",
		}
	}
	return glyphs{
		bullets: [3]string{"•", "◦", "▪"}, quote: "│", hr: "─", h1: "═", h2: "─", h: "─", v: "│",
		tl: "╭", tt: "┬", tr: "╮", ml: "├", mm: "┼", mr: "┤", bl: "╰", bt: "┴", br: "╯",
	}
}
