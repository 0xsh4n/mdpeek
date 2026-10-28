package render

import (
	"strings"
	"unicode"
)

// A deliberately small, dependency-free syntax highlighter: comments,
// strings, numbers and keywords for the languages people paste into READMEs.

type langDef struct {
	kw         map[string]bool
	line       []string
	blockOpen  string
	blockClose string
	quotes     string
	fold       bool
	keys       bool // colour "key": in JSON-like text
}

func mk(keywords string, line []string, open, closer, quotes string) *langDef {
	d := &langDef{kw: map[string]bool{}, line: line, blockOpen: open, blockClose: closer, quotes: quotes}
	for _, k := range strings.Fields(keywords) {
		d.kw[k] = true
	}
	return d
}

var (
	cLike = "auto break case catch char class const continue default delete do double else enum extern false final finally float for goto if implements import inline instanceof int interface long namespace new nil null nullptr override package private protected public register return short signed sizeof static struct super switch template this throw true try typedef typename union unsigned using var virtual void volatile while"
	defs  = map[string]*langDef{
		"go":   mk("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var true false nil iota", []string{"//"}, "/*", "*/", "\"'`"),
		"js":   mk("async await break case catch class const continue debugger default delete do else export extends finally for from function if import in instanceof interface let new of return static super switch this throw try type typeof var void while with yield true false null undefined", []string{"//"}, "/*", "*/", "\"'`"),
		"py":   mk("and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield True False None self", []string{"#"}, "", "", "\"'"),
		"sh":   mk("if then else elif fi for while until do done case esac in function select time return exit export local readonly set unset echo cd source alias sudo", []string{"#"}, "", "", "\"'`"),
		"c":    mk(cLike, []string{"//"}, "/*", "*/", "\"'"),
		"rust": mk("as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while", []string{"//"}, "/*", "*/", "\"'"),
		"json": mk("true false null", nil, "", "", "\""),
		"yaml": mk("true false null yes no on off", []string{"#"}, "", "", "\"'"),
		"toml": mk("true false", []string{"#"}, "", "", "\"'"),
		"sql":  mk("select from where insert into values update set delete create table alter drop index join left right inner outer on group by order having limit offset and or not null as distinct union all primary key foreign references default like in is between exists case when then else end", []string{"--"}, "/*", "*/", "'\""),
	}
	aliases = map[string]string{
		"golang": "go", "go": "go",
		"js": "js", "javascript": "js", "jsx": "js", "mjs": "js", "ts": "js", "typescript": "js", "tsx": "js",
		"py": "py", "python": "py",
		"sh": "sh", "bash": "sh", "zsh": "sh", "shell": "sh", "console": "sh", "fish": "sh",
		"c": "c", "h": "c", "cpp": "c", "cc": "c", "c++": "c", "java": "c", "cs": "c", "csharp": "c", "kotlin": "c", "swift": "c",
		"rust": "rust", "rs": "rust",
		"json": "json", "jsonc": "json", "yaml": "yaml", "yml": "yaml", "toml": "toml", "sql": "sql",
	}
)

func init() {
	defs["sql"].fold = true
	defs["json"].keys = true
	defs["yaml"].keys = true
}

func langFor(info string) *langDef {
	info = strings.ToLower(strings.TrimSpace(info))
	if i := strings.IndexAny(info, " {,"); i >= 0 {
		info = info[:i]
	}
	return defs[aliases[info]]
}

func hasAt(rs []rune, i int, p string) bool {
	if p == "" {
		return false
	}
	pr := []rune(p)
	if i+len(pr) > len(rs) {
		return false
	}
	for k, r := range pr {
		if rs[i+k] != r {
			return false
		}
	}
	return true
}

func indexFrom(rs []rune, i int, p string) int {
	for k := i; k < len(rs); k++ {
		if hasAt(rs, k, p) {
			return k
		}
	}
	return -1
}

func isIdent(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// hlLine tokenises one source line. inBlock carries /* ... */ state.
func (c *ctx) hlLine(def *langDef, s string, inBlock *bool) []tok {
	t := c.t
	var toks []tok
	add := func(text, fg string) {
		if text == "" {
			return
		}
		w := Width(text)
		if n := len(toks); n > 0 && toks[n-1].fg == fg {
			toks[n-1].s += text
			toks[n-1].w += w
			return
		}
		toks = append(toks, tok{s: text, fg: fg, w: w})
	}
	if def == nil {
		add(s, t.text)
		return toks
	}
	rs := []rune(s)
	i := 0
	for i < len(rs) {
		if *inBlock {
			end := indexFrom(rs, i, def.blockClose)
			if end < 0 {
				add(string(rs[i:]), t.com)
				return toks
			}
			end += len([]rune(def.blockClose))
			add(string(rs[i:end]), t.com)
			i = end
			*inBlock = false
			continue
		}
		r := rs[i]
		matched := false
		for _, lc := range def.line {
			if hasAt(rs, i, lc) {
				add(string(rs[i:]), t.com)
				return toks
			}
		}
		if hasAt(rs, i, def.blockOpen) {
			*inBlock = true
			add(def.blockOpen, t.com)
			i += len([]rune(def.blockOpen))
			continue
		}
		switch {
		case strings.ContainsRune(def.quotes, r):
			j := i + 1
			for j < len(rs) {
				if rs[j] == '\\' {
					j += 2
					continue
				}
				if rs[j] == r {
					j++
					break
				}
				j++
			}
			if j > len(rs) {
				j = len(rs)
			}
			fg := t.str
			if def.keys {
				k := j
				for k < len(rs) && rs[k] == ' ' {
					k++
				}
				if k < len(rs) && rs[k] == ':' {
					fg = t.kw
				}
			}
			add(string(rs[i:j]), fg)
			i = j
			matched = true
		case unicode.IsDigit(r) && (i == 0 || !isIdent(rs[i-1])):
			j := i
			for j < len(rs) && (isIdent(rs[j]) || rs[j] == '.') {
				j++
			}
			add(string(rs[i:j]), t.num)
			i = j
			matched = true
		case isIdent(r):
			j := i
			for j < len(rs) && isIdent(rs[j]) {
				j++
			}
			w := string(rs[i:j])
			key := w
			if def.fold {
				key = strings.ToLower(w)
			}
			if def.kw[key] {
				add(w, t.kw)
			} else if def.keys && j < len(rs) && rs[j] == ':' {
				add(w, t.kw)
			} else {
				add(w, t.text)
			}
			i = j
			matched = true
		}
		if !matched {
			add(string(r), t.text)
			i++
		}
	}
	return toks
}
