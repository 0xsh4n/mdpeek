// mdpeek previews Markdown files in the terminal, on Windows, Linux and macOS.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/0xsh4n/mdpeek/internal/pager"
	"github.com/0xsh4n/mdpeek/internal/render"
	"github.com/0xsh4n/mdpeek/internal/term"
)

// Set at release time via -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
	maxInput    = 32 << 20 // refuse to load more than 32 MiB
	maxReadable = 100      // default cap on line width, for readability
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func usage(w io.Writer) {
	fmt.Fprint(w, `mdpeek — preview Markdown in your terminal

Usage:
  mdpeek [options] <file.md>
  cat README.md | mdpeek [options]

Interactive pager (default when stdout is a terminal):
  j/k ↑/↓  scroll    space/b  page    g/G  top/bottom    / n N  search
  t  table of contents    ] [  next/previous heading    ?  help    q  quit
  The file is watched and re-rendered automatically when it changes.

Options:
  -p, --plain          print the rendered document to stdout (no pager)
  -t, --toc            print the heading outline and exit
      --theme NAME     auto (default), dark, light or none
      --width N        wrap at N columns (default: terminal width, max 100)
      --no-urls        do not show link targets after link text
      --no-watch       do not reload when the file changes
      --ascii          ASCII-only borders and bullets
      --color WHEN     auto (default), always or never   (NO_COLOR is honoured)
  -v, --version        print version information
  -h, --help           show this help
`)
}

func run(args []string, stdin, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("mdpeek", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		plain, toc, noWatch, noURLs, ascii, showVersion bool
		theme, colorMode                                string
		width                                           int
	)
	fs.BoolVar(&plain, "plain", false, "")
	fs.BoolVar(&plain, "p", false, "")
	fs.BoolVar(&toc, "toc", false, "")
	fs.BoolVar(&toc, "t", false, "")
	fs.BoolVar(&noWatch, "no-watch", false, "")
	fs.BoolVar(&noURLs, "no-urls", false, "")
	fs.BoolVar(&ascii, "ascii", false, "")
	fs.BoolVar(&showVersion, "version", false, "")
	fs.BoolVar(&showVersion, "v", false, "")
	fs.StringVar(&theme, "theme", "auto", "")
	fs.StringVar(&colorMode, "color", "auto", "")
	fs.IntVar(&width, "width", 0, "")

	// Allow options after the file name, and "--" to end option parsing.
	var files []string
	flagArgs := args
	for i, a := range args {
		if a == "--" {
			flagArgs, files = args[:i], append(files, args[i+1:]...)
			break
		}
	}
	for len(flagArgs) > 0 {
		if err := fs.Parse(flagArgs); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				usage(stdout)
				return 0
			}
			fmt.Fprintf(stderr, "mdpeek: %v\n", err)
			usage(stderr)
			return 2
		}
		flagArgs = fs.Args()
		if len(flagArgs) > 0 {
			files = append(files, flagArgs[0])
			flagArgs = flagArgs[1:]
		}
	}

	if showVersion {
		fmt.Fprintln(stdout, versionString())
		return 0
	}
	if len(files) > 1 {
		fmt.Fprintln(stderr, "mdpeek: only one file can be previewed at a time")
		return 2
	}
	switch theme {
	case "auto", "dark", "light", "none":
	default:
		fmt.Fprintf(stderr, "mdpeek: unknown theme %q (use auto, dark, light or none)\n", theme)
		return 2
	}
	switch colorMode {
	case "auto", "always", "never":
	default:
		fmt.Fprintf(stderr, "mdpeek: unknown --color value %q (use auto, always or never)\n", colorMode)
		return 2
	}

	// Locate the input.
	var path string
	fromStdin := false
	switch {
	case len(files) == 0:
		if term.IsTerminal(stdin) {
			usage(stderr)
			return 2
		}
		fromStdin = true
	case files[0] == "-":
		fromStdin = true
	default:
		path = files[0]
	}

	name := "stdin"
	var load func() (string, error)
	if fromStdin {
		b, err := io.ReadAll(io.LimitReader(stdin, maxInput+1))
		if err != nil {
			fmt.Fprintf(stderr, "mdpeek: reading stdin: %v\n", err)
			return 1
		}
		if len(b) > maxInput {
			fmt.Fprintln(stderr, "mdpeek: input is larger than 32 MiB")
			return 1
		}
		src := string(b)
		load = func() (string, error) { return src, nil }
	} else {
		name = filepath.Base(path)
		load = func() (string, error) { return readFile(path) }
	}
	src, err := load()
	if err != nil {
		fmt.Fprintf(stderr, "mdpeek: %v\n", err)
		return 1
	}

	outTTY := term.IsTerminal(stdout)
	colorOK := os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && theme != "none"
	color := false
	switch colorMode {
	case "always":
		color = theme != "none"
	case "auto":
		color = outTTY && colorOK
	}
	th := render.NewTheme(theme, color, ascii)
	opts := render.Options{Theme: th, HideURLs: noURLs}

	// Outline only.
	if toc {
		opts.Theme = render.NewTheme("none", false, ascii)
		opts.Width = 80
		for _, h := range render.Render(src, opts).Headings {
			fmt.Fprintf(stdout, "%s%s\n", strings.Repeat("  ", h.Level-1), h.Text)
		}
		return 0
	}

	// Non-interactive output.
	if plain || !outTTY {
		w := width
		if w <= 0 {
			w = 80
			if outTTY {
				if cols, _, err := term.Size(stdout); err == nil {
					w = min(cols, maxReadable)
				}
			}
		}
		if color && outTTY {
			restore, _ := term.EnableVT(stdout)
			defer restore()
		}
		opts.Width = w
		res := render.Render(src, opts)
		var buf bytes.Buffer
		for _, l := range res.Lines {
			buf.WriteString(l)
			buf.WriteByte('\n')
		}
		_, _ = stdout.Write(buf.Bytes()) // a closed pipe (| head) is not an error worth reporting
		return 0
	}

	// Interactive pager.
	in := stdin
	if !term.IsTerminal(stdin) {
		tty, err := term.OpenTTYInput()
		if err != nil {
			fmt.Fprintf(stderr, "mdpeek: no terminal available for keyboard input (%v); use --plain\n", err)
			return 1
		}
		defer tty.Close()
		in = tty
	}
	cfg := pager.Config{
		Name:       name,
		Load:       load,
		Opts:       opts,
		FixedWidth: width,
		MaxWidth:   maxReadable,
		In:         in,
		Out:        stdout,
	}
	if !fromStdin && !noWatch {
		cfg.Path = path
	}
	if err := pager.Run(cfg); err != nil {
		fmt.Fprintf(stderr, "mdpeek: %v\n", err)
		return 1
	}
	return 0
}

func readFile(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if fi.Size() > maxInput {
		return "", fmt.Errorf("%s is larger than 32 MiB", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	probe := b
	if len(probe) > 8000 {
		probe = probe[:8000]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		return "", fmt.Errorf("%s looks like a binary file", path)
	}
	return string(b), nil
}

func versionString() string {
	v, c, d := version, commit, date
	if v == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			v = bi.Main.Version // go install github.com/0xsh4n/mdpeek@vX.Y.Z
		}
	}
	return fmt.Sprintf("mdpeek %s (commit %s, built %s, %s, %s/%s)", v, c, d, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
