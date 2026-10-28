package render

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reATX        = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	reSetext     = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)
	reList       = regexp.MustCompile(`^( {0,3})([-*+]|[0-9]{1,9}[.)])( +|$)(.*)$`)
	reTableDelim = regexp.MustCompile(`^[ \t]*\|?[ \t]*:?-+:?[ \t]*(\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$`)
	reDelimCell  = regexp.MustCompile(`^:?-+:?$`)
	reAlert      = regexp.MustCompile(`(?i)^\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$`)
	reTask       = regexp.MustCompile(`^\[([ xX])\][ \t]+`)
	reHeadHTML   = regexp.MustCompile(`(?is)^\s*<h([1-6])[^>]*>(.*?)</h[1-6]>\s*$`)
	reHTMLBlock  = regexp.MustCompile(`(?i)^ {0,3}(<!--|</?(div|p|details|summary|table|thead|tbody|tfoot|tr|td|th|ul|ol|li|h[1-6]|center|picture|source|section|article|aside|header|footer|nav|figure|figcaption|dl|dt|dd|pre|blockquote|hr|br|img|a|kbd|video|audio|iframe|style|script)(\s|/?>|$))`)
)

var alertColor = map[string]string{
	"NOTE": "38;5;75", "TIP": "38;5;78", "IMPORTANT": "38;5;141", "WARNING": "38;5;214", "CAUTION": "38;5;203",
}

type ctx struct {
	t         *Theme
	refs      map[string]string
	showURLs  bool
	headings  []Heading
	listLevel int
}

type fence struct {
	indent int
	ch     byte
	n      int
	info   string
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

func indentOf(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func parseFence(l string) (fence, bool) {
	ind := indentOf(l)
	if ind > 3 || ind >= len(l) {
		return fence{}, false
	}
	ch := l[ind]
	if ch != '`' && ch != '~' {
		return fence{}, false
	}
	n := runLen(l, ind, ch)
	if n < 3 {
		return fence{}, false
	}
	info := strings.TrimSpace(l[ind+n:])
	if ch == '`' && strings.Contains(info, "`") {
		return fence{}, false
	}
	return fence{ind, ch, n, info}, true
}

func isCloseFence(l string, f fence) bool {
	ind := indentOf(l)
	if ind > 3 || ind >= len(l) || l[ind] != f.ch {
		return false
	}
	n := runLen(l, ind, f.ch)
	return n >= f.n && strings.TrimSpace(l[ind+n:]) == ""
}

func stripIndent(l string, n int) string {
	k := 0
	for k < n && k < len(l) && l[k] == ' ' {
		k++
	}
	return l[k:]
}

func isHR(l string) bool {
	if indentOf(l) > 3 {
		return false
	}
	t := strings.TrimSpace(l)
	if len(t) < 3 {
		return false
	}
	ch := t[0]
	if ch != '-' && ch != '*' && ch != '_' {
		return false
	}
	cnt := 0
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case ch:
			cnt++
		case ' ', '\t':
		default:
			return false
		}
	}
	return cnt >= 3
}

func quotePrefix(l string) int {
	ind := indentOf(l)
	if ind > 3 || ind >= len(l) || l[ind] != '>' {
		return -1
	}
	p := ind + 1
	if p < len(l) && l[p] == ' ' {
		p++
	}
	return p
}

// startsBlock reports whether l begins a construct that can interrupt a
// paragraph.
func startsBlock(l string) bool {
	switch {
	case isBlank(l):
		return true
	case reATX.MatchString(l), isHR(l), quotePrefix(l) >= 0, reHTMLBlock.MatchString(l):
		return true
	}
	if _, ok := parseFence(l); ok {
		return true
	}
	if m := reList.FindStringSubmatch(l); m != nil && m[4] != "" {
		if isDigit(m[2][0]) {
			return strings.TrimRight(m[2], ".)") == "1"
		}
		return true
	}
	return false
}

func splitRow(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	if strings.HasSuffix(l, "|") && !strings.HasSuffix(l, `\|`) {
		l = l[:len(l)-1]
	}
	var cells []string
	var b strings.Builder
	for i := 0; i < len(l); i++ {
		switch {
		case l[i] == '\\' && i+1 < len(l) && l[i+1] == '|':
			b.WriteByte('|')
			i++
		case l[i] == '|':
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(l[i])
		}
	}
	return append(cells, strings.TrimSpace(b.String()))
}

// blocks renders block-level Markdown. tight suppresses blank lines between
// blocks (used inside tight list items).
func (c *ctx) blocks(lines []string, width, depth int, tight bool) []string {
	var out []string
	add := func(block []string) int {
		if len(block) == 0 {
			return len(out)
		}
		if len(out) > 0 && !tight {
			out = append(out, "")
		}
		start := len(out)
		out = append(out, block...)
		return start
	}
	record := func(level int, text string, start int) {
		if depth == 0 {
			c.headings = append(c.headings, Heading{Level: level, Text: text, Line: start})
		}
	}

	i := 0
	if depth == 0 && len(lines) > 0 && strings.TrimRight(lines[0], " ") == "---" {
		for j := 1; j < len(lines) && j < 200; j++ {
			if t := strings.TrimRight(lines[j], " "); t == "---" || t == "..." {
				add(c.codeBlock("yaml", lines[1:j], width))
				i = j + 1
				break
			}
		}
	}

	for i < len(lines) {
		line := lines[i]
		if isBlank(line) {
			i++
			continue
		}

		// Fenced code.
		if f, ok := parseFence(line); ok {
			var code []string
			j := i + 1
			for ; j < len(lines); j++ {
				if isCloseFence(lines[j], f) {
					break
				}
				code = append(code, stripIndent(lines[j], f.indent))
			}
			i = j + 1
			add(c.codeBlock(f.info, code, width))
			continue
		}

		// Indented code.
		if indentOf(line) >= 4 {
			var code []string
			j := i
			for j < len(lines) && (isBlank(lines[j]) || indentOf(lines[j]) >= 4) {
				code = append(code, stripIndent(lines[j], 4))
				j++
			}
			i = j
			add(c.codeBlock("", code, width))
			continue
		}

		// ATX heading.
		if m := reATX.FindStringSubmatch(line); m != nil {
			h, plain := c.heading(len(m[1]), m[2], width)
			record(len(m[1]), plain, add(h))
			i++
			continue
		}

		if isHR(line) {
			g := c.t.glyphs()
			add([]string{c.t.paint(c.t.rule, strings.Repeat(g.hr, width))})
			i++
			continue
		}

		// Blockquote.
		if quotePrefix(line) >= 0 {
			var inner []string
			j := i
			for j < len(lines) {
				l := lines[j]
				if p := quotePrefix(l); p >= 0 {
					inner = append(inner, l[p:])
					j++
					continue
				}
				if !isBlank(l) && len(inner) > 0 && !isBlank(inner[len(inner)-1]) && !startsBlock(l) {
					inner = append(inner, l) // lazy continuation
					j++
					continue
				}
				break
			}
			i = j
			add(c.quote(inner, width, depth))
			continue
		}

		// List.
		if reList.MatchString(line) {
			rendered, next := c.list(lines, i, width, depth)
			i = next
			add(rendered)
			continue
		}

		// Table.
		if i+1 < len(lines) && strings.Contains(line, "|") && strings.Contains(lines[i+1], "|") && reTableDelim.MatchString(lines[i+1]) {
			hdr, delim := splitRow(line), splitRow(lines[i+1])
			if len(hdr) == len(delim) && allDelimCells(delim) {
				rows := [][]string{hdr}
				j := i + 2
				for j < len(lines) && !isBlank(lines[j]) && strings.Contains(lines[j], "|") && !startsBlock(lines[j]) {
					row := splitRow(lines[j])
					for len(row) < len(hdr) {
						row = append(row, "")
					}
					rows = append(rows, row[:len(hdr)])
					j++
				}
				aligns := make([]byte, len(delim))
				for k, d := range delim {
					switch {
					case strings.HasPrefix(d, ":") && strings.HasSuffix(d, ":"):
						aligns[k] = 'c'
					case strings.HasSuffix(d, ":"):
						aligns[k] = 'r'
					default:
						aligns[k] = 'l'
					}
				}
				i = j
				add(c.table(rows, aligns, width))
				continue
			}
		}

		// HTML block.
		if reHTMLBlock.MatchString(line) {
			low := strings.ToLower(strings.TrimSpace(line))
			j := i
			switch {
			case strings.HasPrefix(low, "<!--"):
				for j < len(lines) {
					j++
					if strings.Contains(lines[j-1], "-->") {
						break
					}
				}
				i = j
				continue
			case strings.HasPrefix(low, "<script") || strings.HasPrefix(low, "<style"):
				name := low[1:]
				if k := strings.IndexAny(name, " >/\t"); k >= 0 {
					name = name[:k]
				}
				closer := "</" + name + ">"
				for j < len(lines) {
					j++
					if strings.Contains(strings.ToLower(lines[j-1]), closer) {
						break
					}
				}
				i = j
				continue
			}
			var blk []string
			for j < len(lines) && !isBlank(lines[j]) {
				blk = append(blk, strings.TrimSpace(lines[j]))
				j++
			}
			i = j
			text := strings.Join(blk, " ")
			if m := reHeadHTML.FindStringSubmatch(text); m != nil {
				level := int(m[1][0] - '0')
				h, plain := c.heading(level, m[2], width)
				record(level, plain, add(h))
				continue
			}
			spans := c.inline(text, 0, "")
			if strings.TrimSpace(plainOf(spans)) != "" {
				add(c.wrap(spans, width))
			}
			continue
		}

		// Paragraph (or setext heading).
		var para []string
		j := i
		setext := 0
		for j < len(lines) {
			l := lines[j]
			if isBlank(l) {
				break
			}
			if j > i {
				if m := reSetext.FindStringSubmatch(l); m != nil {
					setext = 1
					if m[1][0] == '-' {
						setext = 2
					}
					j++
					break
				}
				if startsBlock(l) {
					break
				}
			}
			para = append(para, l)
			j++
		}
		i = j
		var sb strings.Builder
		for k, l := range para {
			t := strings.TrimLeft(l, " ")
			hard := strings.HasSuffix(t, "  ") || (strings.HasSuffix(t, `\`) && !strings.HasSuffix(t, `\\`))
			sb.WriteString(strings.TrimRight(t, " "))
			if k < len(para)-1 {
				if hard {
					sb.WriteByte('\n')
				} else {
					sb.WriteByte(' ')
				}
			}
		}
		text := sb.String()
		if setext > 0 {
			h, plain := c.heading(setext, text, width)
			record(setext, plain, add(h))
			continue
		}
		add(c.wrap(c.inline(text, 0, ""), width))
	}
	return out
}

func allDelimCells(cells []string) bool {
	for _, d := range cells {
		if !reDelimCell.MatchString(d) {
			return false
		}
	}
	return true
}

func (c *ctx) heading(level int, text string, width int) ([]string, string) {
	t := c.t
	spans := c.inline(strings.TrimSpace(text), Bold, t.heading[level])
	plain := strings.TrimSpace(plainOf(spans))
	prefix := ""
	if level >= 3 {
		prefix = strings.Repeat("#", level) + " "
	}
	pw := len(prefix)
	lines := c.wrap(spans, max(width-pw, 10))
	out := make([]string, 0, len(lines)+1)
	for i, l := range lines {
		switch {
		case prefix == "":
		case i == 0:
			l = t.paint(t.dim, prefix) + l
		default:
			l = spaces(pw) + l
		}
		out = append(out, l)
	}
	if level <= 2 {
		w := 1
		for _, l := range lines {
			w = max(w, Width(l)+pw)
		}
		g := t.glyphs()
		ch := g.h2
		if level == 1 {
			ch = g.h1
		}
		out = append(out, t.paint(t.heading[level], strings.Repeat(ch, min(w, width))))
	}
	return out, plain
}

func (c *ctx) quote(inner []string, width, depth int) []string {
	t := c.t
	g := t.glyphs()
	barColor := t.quote
	label := ""
	if len(inner) > 0 {
		if m := reAlert.FindStringSubmatch(inner[0]); m != nil {
			kind := strings.ToUpper(m[1])
			inner = inner[1:]
			barColor = alertColor[kind]
			label = kind
		}
	}
	body := c.blocks(inner, max(width-2, 10), depth+1, false)
	bar := t.paint(barColor, g.quote) + " "
	var out []string
	if label != "" {
		out = append(out, bar+t.paint("1;"+barColor, label))
	}
	for _, l := range body {
		out = append(out, bar+l)
	}
	if len(out) == 0 {
		out = append(out, bar)
	}
	return out
}

func (c *ctx) codeBlock(info string, code []string, width int) []string {
	for len(code) > 0 && isBlank(code[len(code)-1]) {
		code = code[:len(code)-1]
	}
	for len(code) > 0 && isBlank(code[0]) {
		code = code[1:]
	}
	if len(code) == 0 {
		return nil
	}
	t := c.t
	if !t.Color {
		var out []string
		for _, l := range code {
			for _, ch := range splitToks([]tok{{s: l, w: Width(l)}}, max(width-4, 8)) {
				out = append(out, "    "+t.emit(ch))
			}
		}
		return out
	}
	inner := max(width-2, 8)
	bg := t.codeBlock
	join := func(fg string) string {
		if fg == "" {
			return bg
		}
		return bg + ";" + fg
	}
	row := func(parts []tok) string {
		line := make([]tok, 0, len(parts)+2)
		line = append(line, tok{s: " ", fg: bg, w: 1})
		line = append(line, parts...)
		if pad := inner - toksWidth(parts); pad > 0 {
			line = append(line, tok{s: spaces(pad), fg: bg, w: pad})
		}
		line = append(line, tok{s: " ", fg: bg, w: 1})
		return t.emit(line)
	}
	var out []string
	if f := strings.Fields(info); len(f) > 0 {
		out = append(out, row([]tok{{s: f[0], fg: join(t.dim), w: Width(f[0])}}))
	} else {
		out = append(out, row(nil))
	}
	def := langFor(info)
	inBlock := false
	for _, l := range code {
		toks := c.hlLine(def, l, &inBlock)
		for k := range toks {
			toks[k].fg = join(toks[k].fg)
		}
		for _, ch := range splitToks(toks, inner) {
			out = append(out, row(ch))
		}
	}
	return append(out, row(nil))
}

func (c *ctx) list(lines []string, i, width, depth int) ([]string, int) {
	t := c.t
	first := reList.FindStringSubmatch(lines[i])
	ordered := isDigit(first[2][0])
	delim := ""
	start := 1
	if ordered {
		delim = first[2][len(first[2])-1:]
		start, _ = strconv.Atoi(first[2][:len(first[2])-1])
	}
	var items [][]string
	loose := false
	j := i
	for j < len(lines) {
		mm := reList.FindStringSubmatch(lines[j])
		if mm == nil || isHR(lines[j]) || isDigit(mm[2][0]) != ordered {
			break
		}
		if ordered && mm[2][len(mm[2])-1:] != delim {
			break
		}
		if !ordered && mm[2] != first[2] {
			break
		}
		sp := len(mm[3])
		rest := mm[4]
		if rest == "" {
			sp = 1
		} else if sp > 4 {
			rest = spaces(sp-1) + rest
			sp = 1
		}
		cc := len(mm[1]) + len(mm[2]) + sp
		item := []string{rest}
		j++
		for j < len(lines) {
			l := lines[j]
			if isBlank(l) {
				k := j + 1
				for k < len(lines) && isBlank(lines[k]) {
					k++
				}
				if k >= len(lines) {
					j = k
					break
				}
				if indentOf(lines[k]) >= cc {
					for ; j < k; j++ {
						item = append(item, "")
					}
					loose = true
					continue
				}
				if m2 := reList.FindStringSubmatch(lines[k]); m2 != nil && isDigit(m2[2][0]) == ordered &&
					((ordered && m2[2][len(m2[2])-1:] == delim) || (!ordered && m2[2] == first[2])) {
					loose = true // blank line before a sibling item
				}
				j = k
				break
			}
			if indentOf(l) >= cc {
				item = append(item, l[cc:])
				j++
				continue
			}
			if !isBlank(item[len(item)-1]) && !startsBlock(l) && !reList.MatchString(l) {
				item = append(item, strings.TrimLeft(l, " "))
				j++
				continue
			}
			break
		}
		items = append(items, item)
	}

	g := t.glyphs()
	level := c.listLevel
	c.listLevel++
	defer func() { c.listLevel-- }()
	numW := len(strconv.Itoa(start+len(items)-1)) + len(delim)

	var out []string
	for idx, item := range items {
		if idx > 0 && loose {
			out = append(out, "")
		}
		var marker string
		switch {
		case ordered:
			n := strconv.Itoa(start + idx)
			marker = t.paint(t.bullet, spaces(numW-len(n)-len(delim))+n+delim)
		default:
			marker = t.paint(t.bullet, g.bullets[min(level, 2)])
		}
		mw := 2
		if ordered {
			mw = numW + 1
		}
		if m := reTask.FindStringSubmatch(item[0]); m != nil {
			item[0] = item[0][len(m[0]):]
			if m[1] == " " {
				marker = t.paint(t.dim, "[ ]")
			} else {
				marker = t.paint("38;5;114", "[x]")
			}
			mw = 4
		}
		body := c.blocks(item, max(width-mw, 10), depth+1, !loose)
		if len(body) == 0 {
			out = append(out, marker)
			continue
		}
		for k, l := range body {
			switch {
			case k == 0:
				out = append(out, marker+" "+l)
			case l == "":
				out = append(out, "")
			default:
				out = append(out, spaces(mw)+l)
			}
		}
	}
	return out, j
}

func (c *ctx) table(rows [][]string, aligns []byte, width int) []string {
	t := c.t
	n := len(rows[0])
	cells := make([][][]word, len(rows))
	nat := make([]int, n)
	for r, row := range rows {
		cells[r] = make([][]word, n)
		for k := 0; k < n; k++ {
			base := Style(0)
			if r == 0 {
				base = Bold
			}
			ws := toWords(c.inline(row[k], base, ""))
			cells[r][k] = ws
			for _, ln := range wrapWords(ws, 1<<20) {
				nat[k] = max(nat[k], toksWidth(ln))
			}
		}
	}
	for k := range nat {
		nat[k] = max(nat[k], 1)
	}
	widths := append([]int(nil), nat...)
	avail := width - (3*n + 1)
	// Shrink the widest column until the table fits, first down to 3 cells
	// per column, then (for very narrow terminals) down to 1.
	for _, floor := range []int{3, 1} {
		for {
			sum := 0
			for _, w := range widths {
				sum += w
			}
			if sum <= avail {
				break
			}
			idx, best := -1, floor
			for k, w := range widths {
				if w > best {
					idx, best = k, w
				}
			}
			if idx < 0 {
				break
			}
			widths[idx]--
		}
	}

	g := t.glyphs()
	border := func(l, m, r string) string {
		var b strings.Builder
		b.WriteString(l)
		for k, w := range widths {
			b.WriteString(strings.Repeat(g.h, w+2))
			if k < n-1 {
				b.WriteString(m)
			}
		}
		b.WriteString(r)
		return t.paint(t.border, b.String())
	}
	vbar := t.paint(t.border, g.v)
	out := []string{border(g.tl, g.tt, g.tr)}
	for r := range rows {
		cellLines := make([][]string, n)
		height := 1
		for k := 0; k < n; k++ {
			lines := wrapWords(cells[r][k], widths[k])
			strs := make([]string, len(lines))
			for li, ln := range lines {
				pad := widths[k] - toksWidth(ln)
				s := t.emit(ln)
				switch aligns[k] {
				case 'r':
					s = spaces(pad) + s
				case 'c':
					s = spaces(pad/2) + s + spaces(pad-pad/2)
				default:
					s += spaces(pad)
				}
				strs[li] = s
			}
			cellLines[k] = strs
			height = max(height, len(strs))
		}
		for li := 0; li < height; li++ {
			var b strings.Builder
			b.WriteString(vbar)
			for k := 0; k < n; k++ {
				b.WriteByte(' ')
				if li < len(cellLines[k]) {
					b.WriteString(cellLines[k][li])
				} else {
					b.WriteString(spaces(widths[k]))
				}
				b.WriteByte(' ')
				b.WriteString(vbar)
			}
			out = append(out, b.String())
		}
		if r == 0 {
			out = append(out, border(g.ml, g.mm, g.mr))
		}
	}
	return append(out, border(g.bl, g.bt, g.br))
}
