// Package pager implements mdpeek's interactive full-screen viewer.
package pager

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/0xsh4n/mdpeek/internal/render"
	"github.com/0xsh4n/mdpeek/internal/term"
	"github.com/0xsh4n/mdpeek/internal/watch"
)

// Config describes what to show and how.
type Config struct {
	Name       string                 // shown in the status bar
	Load       func() (string, error) // returns the Markdown source
	Path       string                 // when set, the file is watched and reloaded on change
	Opts       render.Options         // theme and URL behaviour (Width/Margin are computed)
	FixedWidth int                    // 0 follows the terminal
	MaxWidth   int                    // readability cap used when FixedWidth is 0
	In, Out    *os.File               // terminal input and output
}

const (
	enterScreen = "\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[?1007h\x1b[2J"
	leaveScreen = "\x1b[?1007l\x1b[?7h\x1b[?25h\x1b[?1049l"
)

type mode int

const (
	modeNormal mode = iota
	modeSearch
	modeTOC
	modeHelp
)

type pager struct {
	cfg        Config
	cols, rows int
	src        string
	res        render.Result
	plain      []string
	top        int
	mode       mode

	input    []rune // search being typed
	query    []rune // active search
	needle   []rune // query, lower-cased when fold
	fold     bool
	matches  []int // line indexes
	matchSet map[int]bool
	cur      int // index into matches, -1 if none

	tocSel, tocTop int

	msg      string
	msgUntil time.Time
	dirty    bool
}

// Run shows the document until the user quits.
func Run(cfg Config) error {
	restoreOut, _ := term.EnableVT(cfg.Out)
	defer restoreOut()
	restoreIn, err := term.MakeRaw(cfg.In)
	if err != nil {
		return fmt.Errorf("cannot switch the terminal to raw mode: %w", err)
	}
	defer restoreIn()

	cfg.Out.WriteString(enterScreen)
	defer cfg.Out.WriteString(leaveScreen)

	p := &pager{cfg: cfg, cur: -1, matchSet: map[int]bool{}}
	if err := p.measure(); err != nil {
		return err
	}
	if err := p.reload(true); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var changes <-chan struct{}
	if cfg.Path != "" {
		changes = watch.File(ctx, cfg.Path, 400*time.Millisecond)
	}
	keys := term.Keys(cfg.In)
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	p.draw()
	for {
		select {
		case ev, ok := <-keys:
			if !ok || p.handle(ev) {
				return nil
			}
			p.dirty = true
		case <-changes:
			_ = p.reload(false)
		case <-tick.C:
			if err := p.measure(); err == nil && p.dirty {
				// measure marks dirty itself when the size changed
			}
			if p.msg != "" && time.Now().After(p.msgUntil) {
				p.msg = ""
				p.dirty = true
			}
		case <-sig:
			return nil
		}
		if p.dirty {
			p.draw()
		}
	}
}

// measure reads the terminal size and re-renders if the width changed.
func (p *pager) measure() error {
	cols, rows, err := term.Size(p.cfg.Out)
	if err != nil {
		if p.cols == 0 {
			cols, rows = 80, 24
		} else {
			return nil
		}
	}
	if cols == p.cols && rows == p.rows {
		return nil
	}
	p.cols, p.rows = cols, rows
	if p.src != "" {
		ratio := 0.0
		if n := len(p.res.Lines); n > 0 {
			ratio = float64(p.top) / float64(n)
		}
		p.render()
		p.top = int(ratio * float64(len(p.res.Lines)))
		p.clamp()
		p.search(p.query, false)
	}
	p.dirty = true
	return nil
}

func (p *pager) layout() (width, margin int) {
	margin = 2
	if p.cols < 50 {
		margin = 0
	}
	width = p.cols - 2*margin
	if p.cfg.FixedWidth > 0 {
		width = min(width, p.cfg.FixedWidth)
	} else if p.cfg.MaxWidth > 0 {
		width = min(width, p.cfg.MaxWidth)
	}
	width = max(width, 20)
	margin = max((p.cols-width)/2, 0)
	return
}

func (p *pager) render() {
	o := p.cfg.Opts
	o.Width, o.Margin = p.layout()
	p.res = render.Render(p.src, o)
	p.plain = make([]string, len(p.res.Lines))
	for i, l := range p.res.Lines {
		p.plain[i] = render.Strip(l)
	}
}

func (p *pager) reload(first bool) error {
	src, err := p.cfg.Load()
	if err != nil {
		if first {
			return err
		}
		p.flash("reload failed: " + err.Error())
		return nil
	}
	if !first && src == p.src {
		return nil
	}
	p.src = src
	p.render()
	p.clamp()
	p.search(p.query, false)
	if !first {
		p.flash("reloaded")
	}
	p.dirty = true
	return nil
}

func (p *pager) flash(s string) {
	p.msg = s
	p.msgUntil = time.Now().Add(2500 * time.Millisecond)
	p.dirty = true
}

func (p *pager) body() int { return max(p.rows-1, 1) }

func (p *pager) maxTop() int { return max(len(p.res.Lines)-p.body(), 0) }

func (p *pager) clamp() { p.top = min(max(p.top, 0), p.maxTop()) }

func (p *pager) scroll(n int) { p.top += n; p.clamp() }

// search (re)computes matches for q. When jump is true it moves to the first
// match at or below the current position.
func (p *pager) search(q []rune, jump bool) {
	p.query = q
	p.matches, p.matchSet, p.cur = nil, map[int]bool{}, -1
	if len(q) == 0 {
		return
	}
	p.fold = true
	for _, r := range q {
		if r != []rune(strings.ToLower(string(r)))[0] {
			p.fold = false
			break
		}
	}
	p.needle = q
	if p.fold {
		p.needle = render.LowerRunes(q)
	}
	for i, l := range p.plain {
		if len(render.FindAll([]rune(l), p.needle, p.fold)) > 0 {
			p.matches = append(p.matches, i)
			p.matchSet[i] = true
		}
	}
	if len(p.matches) == 0 {
		return
	}
	p.cur = 0
	for k, line := range p.matches {
		if line >= p.top {
			p.cur = k
			break
		}
	}
	if jump {
		p.goMatch(0)
	}
}

func (p *pager) goMatch(delta int) {
	if len(p.matches) == 0 {
		return
	}
	p.cur = (p.cur + delta + len(p.matches)) % len(p.matches)
	line := p.matches[p.cur]
	if line < p.top || line >= p.top+p.body() {
		p.top = line - p.body()/3
		p.clamp()
	}
}

func (p *pager) jumpHeading(dir int) {
	hs := p.res.Headings
	if dir > 0 {
		for _, h := range hs {
			if h.Line > p.top {
				p.top = h.Line
				p.clamp()
				return
			}
		}
		return
	}
	for i := len(hs) - 1; i >= 0; i-- {
		if hs[i].Line < p.top {
			p.top = hs[i].Line
			p.clamp()
			return
		}
	}
	p.top = 0
}

// handle processes a key and reports whether to quit.
func (p *pager) handle(ev term.Event) bool {
	if ev.Key == term.KeyCtrlC {
		return true
	}
	switch p.mode {
	case modeSearch:
		p.handleSearch(ev)
	case modeTOC:
		p.handleTOC(ev)
	case modeHelp:
		p.mode = modeNormal
	default:
		return p.handleNormal(ev)
	}
	return false
}

func (p *pager) handleNormal(ev term.Event) bool {
	page := max(p.body()-1, 1)
	switch ev.Key {
	case term.KeyDown, term.KeyEnter:
		p.scroll(1)
	case term.KeyUp:
		p.scroll(-1)
	case term.KeyPgDn, term.KeyCtrlF:
		p.scroll(page)
	case term.KeyPgUp, term.KeyCtrlB:
		p.scroll(-page)
	case term.KeyCtrlD:
		p.scroll(page / 2)
	case term.KeyCtrlU:
		p.scroll(-page / 2)
	case term.KeyHome:
		p.top = 0
	case term.KeyEnd:
		p.top = p.maxTop()
	case term.KeyCtrlL:
		p.cfg.Out.WriteString("\x1b[2J")
	case term.KeyTab:
		p.openTOC()
	case term.KeyEsc:
		p.search(nil, false)
	case term.KeyRune:
		switch ev.Rune {
		case 'q':
			return true
		case 'j':
			p.scroll(1)
		case 'k':
			p.scroll(-1)
		case ' ', 'f':
			p.scroll(page)
		case 'b':
			p.scroll(-page)
		case 'd':
			p.scroll(page / 2)
		case 'u':
			p.scroll(-page / 2)
		case 'g':
			p.top = 0
		case 'G':
			p.top = p.maxTop()
		case ']', '}':
			p.jumpHeading(1)
		case '[', '{':
			p.jumpHeading(-1)
		case '/':
			p.mode = modeSearch
			p.input = nil
		case 'n':
			p.goMatch(1)
		case 'N':
			p.goMatch(-1)
		case 't':
			p.openTOC()
		case 'r':
			p.src = "\x00" // force a re-render even if the file is unchanged
			_ = p.reload(false)
		case '?', 'h':
			p.mode = modeHelp
		}
	}
	p.clamp()
	return false
}

func (p *pager) handleSearch(ev term.Event) {
	switch ev.Key {
	case term.KeyEsc:
		p.mode = modeNormal
	case term.KeyEnter:
		p.mode = modeNormal
		p.search(p.input, true)
		if len(p.input) > 0 && len(p.matches) == 0 {
			p.flash("pattern not found: " + string(p.input))
		}
	case term.KeyBackspace:
		if n := len(p.input); n > 0 {
			p.input = p.input[:n-1]
		} else {
			p.mode = modeNormal
		}
	case term.KeyRune:
		p.input = append(p.input, ev.Rune)
	}
}

func (p *pager) openTOC() {
	if len(p.res.Headings) == 0 {
		p.flash("no headings in this document")
		return
	}
	p.mode = modeTOC
	p.tocSel = 0
	for i, h := range p.res.Headings {
		if h.Line <= p.top {
			p.tocSel = i
		}
	}
}

func (p *pager) handleTOC(ev term.Event) {
	n := len(p.res.Headings)
	switch ev.Key {
	case term.KeyEsc, term.KeyTab:
		p.mode = modeNormal
	case term.KeyUp:
		p.tocSel = max(p.tocSel-1, 0)
	case term.KeyDown:
		p.tocSel = min(p.tocSel+1, n-1)
	case term.KeyPgUp:
		p.tocSel = max(p.tocSel-p.body(), 0)
	case term.KeyPgDn:
		p.tocSel = min(p.tocSel+p.body(), n-1)
	case term.KeyHome:
		p.tocSel = 0
	case term.KeyEnd:
		p.tocSel = n - 1
	case term.KeyEnter:
		p.top = p.res.Headings[p.tocSel].Line
		p.clamp()
		p.mode = modeNormal
	case term.KeyRune:
		switch ev.Rune {
		case 'q', 't':
			p.mode = modeNormal
		case 'j':
			p.tocSel = min(p.tocSel+1, n-1)
		case 'k':
			p.tocSel = max(p.tocSel-1, 0)
		case 'g':
			p.tocSel = 0
		case 'G':
			p.tocSel = n - 1
		}
	}
}

func (p *pager) draw() {
	p.dirty = false
	var b strings.Builder
	var lines []string
	hi := -1
	switch p.mode {
	case modeTOC:
		lines, hi = p.tocLines()
	case modeHelp:
		lines = helpLines
	default:
		lines = p.res.Lines
	}
	top := p.top
	if p.mode == modeHelp {
		top = 0
	}
	if p.mode == modeTOC {
		p.tocTop = min(max(p.tocTop, hi-p.body()+1), hi)
		p.tocTop = max(p.tocTop, 0)
		top = p.tocTop
	}
	curLine := -1
	if p.cur >= 0 && p.cur < len(p.matches) {
		curLine = p.matches[p.cur]
	}
	for r := 0; r < p.body(); r++ {
		fmt.Fprintf(&b, "\x1b[%d;1H", r+1)
		if idx := top + r; idx < len(lines) {
			l := lines[idx]
			switch {
			case p.mode == modeTOC && idx == hi:
				l = "\x1b[7m" + clip(render.Strip(l), p.cols) + spaces(p.cols-render.Width(clip(render.Strip(l), p.cols))) + "\x1b[0m"
			case p.mode == modeNormal || p.mode == modeSearch:
				if len(p.query) > 0 && p.matchSet[idx] {
					on := "\x1b[7m"
					if idx == curLine {
						on = "\x1b[1;7m"
					}
					l = render.Highlight(l, p.needle, p.fold, on, "\x1b[27m")
				}
			}
			b.WriteString(l)
		} else if len(p.res.Lines) == 0 && r == 0 && p.mode == modeNormal {
			b.WriteString("\x1b[2m(empty document)")
		}
		b.WriteString("\x1b[0m\x1b[K")
	}
	fmt.Fprintf(&b, "\x1b[%d;1H%s", p.rows, p.status())
	_, _ = p.cfg.Out.WriteString(b.String())
}

func (p *pager) tocLines() ([]string, int) {
	lines := make([]string, len(p.res.Headings))
	for i, h := range p.res.Headings {
		lines[i] = " " + strings.Repeat("  ", h.Level-1) + h.Text
	}
	return lines, p.tocSel
}

func (p *pager) status() string {
	var left, right string
	switch p.mode {
	case modeSearch:
		left = " /" + string(p.input) + "█"
		right = "enter: search  esc: cancel "
	case modeTOC:
		left = " Contents"
		right = "↑/↓ select  enter: jump  esc: close "
	case modeHelp:
		left = " Help"
		right = "press any key to close "
	default:
		left = " " + p.cfg.Name
		switch {
		case p.msg != "":
			left += "  ·  " + p.msg
		case len(p.query) > 0 && len(p.matches) > 0:
			left += fmt.Sprintf("  ·  /%s  %d/%d", string(p.query), p.cur+1, len(p.matches))
		case len(p.query) > 0:
			left += "  ·  /" + string(p.query) + "  no match"
		}
		right = p.position() + "  ? help "
	}
	pad := p.cols - render.Width(left) - render.Width(right)
	if pad < 1 {
		left = clip(left, max(p.cols-render.Width(right)-1, 0))
		pad = max(p.cols-render.Width(left)-render.Width(right), 0)
	}
	return "\x1b[7m" + clip(left+spaces(pad)+right, p.cols) + "\x1b[0m"
}

func (p *pager) position() string {
	n := len(p.res.Lines)
	switch {
	case n <= p.body():
		return "All"
	case p.top == 0:
		return "Top"
	case p.top >= p.maxTop():
		return "Bot"
	}
	return fmt.Sprintf("%d%%", (p.top+p.body())*100/n)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// clip truncates plain text s to w display cells.
func clip(s string, w int) string {
	var b strings.Builder
	cw := 0
	for _, r := range s {
		rw := render.RuneWidth(r)
		if cw+rw > w {
			break
		}
		b.WriteRune(r)
		cw += rw
	}
	return b.String()
}

var helpLines = []string{
	"",
	"  mdpeek — Markdown viewer",
	"",
	"  Scrolling",
	"    j / ↓ / enter     line down        k / ↑            line up",
	"    space / f / PgDn  page down        b / PgUp         page up",
	"    d / u             half page        g / G            top / bottom",
	"",
	"  Navigation",
	"    t / tab           table of contents",
	"    ] / [             next / previous heading",
	"",
	"  Search",
	"    /                 search (smart-case)",
	"    n / N             next / previous match",
	"    esc               clear search",
	"",
	"  Other",
	"    r                 reload now (changes are also picked up live)",
	"    ctrl-l            redraw",
	"    q / ctrl-c        quit",
}
