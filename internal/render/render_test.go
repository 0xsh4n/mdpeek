package render

import (
	"strings"
	"testing"
)

func plainRender(src string, width int) Result {
	return Render(src, Options{Width: width})
}

func colorRender(src string, width int) Result {
	return Render(src, Options{Width: width, Theme: NewTheme("dark", true, false)})
}

func joined(r Result) string { return strings.Join(r.Lines, "\n") }

func TestNoLineExceedsWidth(t *testing.T) {
	src := "# Title\n\n" + strings.Repeat("word ", 80) + "\n\n- " + strings.Repeat("item ", 40) +
		"\n\n> " + strings.Repeat("quote ", 40) + "\n\n```\n" + strings.Repeat("x", 300) + "\n```\n\n" +
		"| a | b |\n|---|---|\n| " + strings.Repeat("long ", 30) + " | short |\n"
	for _, th := range []*Theme{nil, NewTheme("dark", true, false)} {
		for _, w := range []int{20, 40, 72} {
			res := Render(src, Options{Width: w, Theme: th})
			for _, l := range res.Lines {
				if got := Width(l); got > w {
					t.Fatalf("width %d: line has width %d: %q", w, got, Strip(l))
				}
			}
		}
	}
}

func TestHeadingsAndOutline(t *testing.T) {
	src := "# One\n\ntext\n\n## Two\n\nSetext\n------\n\n### Three ###\n\n```\n# not a heading\n```\n"
	res := plainRender(src, 60)
	want := []struct {
		level int
		text  string
	}{{1, "One"}, {2, "Two"}, {2, "Setext"}, {3, "Three"}}
	if len(res.Headings) != len(want) {
		t.Fatalf("got %d headings, want %d: %+v", len(res.Headings), len(want), res.Headings)
	}
	for i, w := range want {
		h := res.Headings[i]
		if h.Level != w.level || h.Text != w.text {
			t.Errorf("heading %d = %+v, want %+v", i, h, w)
		}
		if h.Line < 0 || h.Line >= len(res.Lines) || !strings.Contains(res.Lines[h.Line], w.text) {
			t.Errorf("heading %q points at line %d (%q)", w.text, h.Line, res.Lines[min(h.Line, len(res.Lines)-1)])
		}
	}
}

func TestTerminalEscapesAreNeutralised(t *testing.T) {
	src := "evil \x1b]0;pwned\x07 title &#27;[31m red &#x9b;31m end\n\n```\n\x1b[2Jcode\n```\n\n[l](http://x/\x1b[0m)\n"
	out := plainRender(src, 80)
	for _, l := range out.Lines {
		if strings.ContainsAny(l, "\x1b\x07\x9b") {
			t.Fatalf("control character leaked into output: %q", l)
		}
	}
	col := colorRender(src, 80)
	for _, l := range col.Lines {
		// only the renderer's own SGR sequences may appear
		stripped := Strip(l)
		if strings.ContainsAny(stripped, "\x1b\x07\x9b") {
			t.Fatalf("control character leaked into coloured output: %q", stripped)
		}
	}
}

func TestBidiOverridesAreMadeVisible(t *testing.T) {
	got := joined(plainRender("safe ‮gnp.exe‬ text", 80))
	if strings.ContainsRune(got, '‮') || !strings.Contains(got, "<U+202E>") {
		t.Fatalf("bidi override not neutralised: %q", got)
	}
}

func TestInlineStyles(t *testing.T) {
	got := joined(colorRender("**bold** *it* `code` ~~gone~~", 80))
	for _, want := range []string{"\x1b[0;1m", "\x1b[0;3m", "\x1b[0;9m"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(Strip(got), "*") || strings.Contains(Strip(got), "~~") || strings.Contains(Strip(got), "`") {
		t.Errorf("markers leaked: %q", Strip(got))
	}
}

func TestPlainModeKeepsBackticksAndURLs(t *testing.T) {
	got := joined(plainRender("see `x` and [docs](https://e.com/a)", 80))
	if got != "see `x` and docs (https://e.com/a)" {
		t.Fatalf("got %q", got)
	}
	got = joined(Render("[docs](https://e.com/a)", Options{Width: 80, HideURLs: true}))
	if got != "docs" {
		t.Fatalf("HideURLs: got %q", got)
	}
}

func TestReferenceLinksAndImages(t *testing.T) {
	got := joined(plainRender("[Go][go] ![alt text](a.png)\n\n[go]: https://go.dev\n", 80))
	if got != "Go (https://go.dev) [image: alt text]" {
		t.Fatalf("got %q", got)
	}
}

func TestListsNestingAndTasks(t *testing.T) {
	src := "- a\n  - b\n- [x] done\n- [ ] todo\n\n1. one\n2. two\n"
	got := joined(plainRender(src, 40))
	want := "• a\n  ◦ b\n• ... "
	_ = want
	for _, s := range []string{"• a", "  ◦ b", "[x] done", "[ ] todo", "1. one", "2. two"} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in:\n%s", s, got)
		}
	}
	if strings.Contains(got, "• a\n\n") {
		t.Errorf("tight list rendered as loose:\n%s", got)
	}
}

func TestTableAlignmentAndWideRunes(t *testing.T) {
	src := "| 名前 | n |\n|:--|--:|\n| 日本語 | 1 |\n| ab | 22 |\n"
	res := plainRender(src, 60)
	var w int
	for i, l := range res.Lines {
		if i == 0 {
			w = Width(l)
		}
		if Width(l) != w {
			t.Fatalf("ragged table (line %d width %d, want %d):\n%s", i, Width(l), w, joined(res))
		}
	}
}

func TestCodeBlockHighlightingHasColour(t *testing.T) {
	out := joined(colorRender("```go\nfunc main() { return \"x\" } // c\n```\n", 60))
	if !strings.Contains(out, "38;5;204") { // keyword colour
		t.Errorf("no keyword colour in %q", out)
	}
	if !strings.Contains(out, "38;5;114") { // string colour
		t.Errorf("no string colour in %q", out)
	}
}

func TestHTMLHandling(t *testing.T) {
	src := "<!-- hidden -->\n\n<h1 align=\"center\">Logo</h1>\n\n<script>alert(1)</script>\n\n<p><img src=\"a\" alt=\"badge\"></p>\n"
	res := plainRender(src, 60)
	got := joined(res)
	if strings.Contains(got, "hidden") || strings.Contains(got, "alert") {
		t.Errorf("comment/script leaked: %q", got)
	}
	if !strings.Contains(got, "Logo") || !strings.Contains(got, "[image: badge]") {
		t.Errorf("missing html content: %q", got)
	}
	if len(res.Headings) != 1 || res.Headings[0].Text != "Logo" {
		t.Errorf("html heading not in outline: %+v", res.Headings)
	}
}

func TestAlertBlock(t *testing.T) {
	got := joined(plainRender("> [!NOTE]\n> Useful.\n", 60))
	if !strings.Contains(got, "NOTE") || !strings.Contains(got, "Useful.") {
		t.Errorf("got %q", got)
	}
}

func TestEmptyAndWhitespaceInput(t *testing.T) {
	for _, s := range []string{"", "\n\n", "   \n\t\n"} {
		if r := plainRender(s, 40); len(r.Lines) != 0 {
			t.Errorf("%q rendered %q", s, r.Lines)
		}
	}
}

func TestHighlightKeepsText(t *testing.T) {
	line := colorRender("hello **big** world", 80).Lines[0]
	h := Highlight(line, []rune("big wo"), true, "<", ">")
	if Strip(strings.NewReplacer("<", "", ">", "").Replace(h)) != "hello big world" {
		t.Fatalf("text changed: %q", h)
	}
	if !strings.Contains(h, "<") {
		t.Fatalf("no highlight inserted: %q", h)
	}
}

func TestPathologicalInputDoesNotHang(t *testing.T) {
	inputs := []string{
		strings.Repeat("*", 5000),
		strings.Repeat("[", 3000) + strings.Repeat("]", 3000),
		strings.Repeat("`", 4000),
		strings.Repeat("> ", 500) + "x",
		strings.Repeat("- ", 500) + "x",
		strings.Repeat("_a ", 3000),
	}
	for _, in := range inputs {
		_ = colorRender(in, 80)
	}
}

func FuzzRender(f *testing.F) {
	data, _ := readSample()
	f.Add(data)
	f.Add("# h\n\n- a\n  - b\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")
	f.Fuzz(func(t *testing.T, s string) {
		res := colorRender(s, 40)
		for _, l := range res.Lines {
			if strings.ContainsAny(Strip(l), "\x1b\x07") {
				t.Fatalf("control character in output: %q", l)
			}
		}
	})
}
