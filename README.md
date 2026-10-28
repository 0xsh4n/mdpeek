# mdpeek

Preview Markdown files in your terminal — on **Windows, Linux and macOS**.
One static binary, **zero dependencies**.

```
mdpeek README.md
```

## Features

- Interactive pager: scroll, page, jump between headings, **search** (`/`, `n`, `N`), **table of contents** (`t`)
- **Live reload** — edit the file in your editor and the preview updates
- Tables (with alignment and wide/CJK characters), nested and task lists, blockquotes, GitHub alerts (`> [!NOTE]`), reference links, front matter
- Syntax highlighting for Go, JS/TS, Python, shell, C-family, Rust, JSON, YAML, TOML, SQL
- Dark/light themes, `NO_COLOR` support, `--ascii` mode for limited terminals
- Pipe-friendly: `cat README.md | mdpeek`, or `mdpeek --plain README.md | less -R`
- **Safe on untrusted files**: control characters and escape sequences in the Markdown are stripped, and bidi overrides are made visible

## Install

Download an archive for your platform from the [Releases](../../releases) page, or:

```sh
go install github.com/0xsh4n/mdpeek@latest
```

## Usage

```
mdpeek [options] <file.md>
cat README.md | mdpeek [options]

  -p, --plain          print the rendered document to stdout (no pager)
  -t, --toc            print the heading outline and exit
      --theme NAME     auto (default), dark, light or none
      --width N        wrap at N columns (default: terminal width, max 100)
      --no-urls        do not show link targets after link text
      --no-watch       do not reload when the file changes
      --ascii          ASCII-only borders and bullets
      --color WHEN     auto (default), always or never
  -v, --version        print version information
```

### Keys

| Key | Action |
|-----|--------|
| `j` `k` `↑` `↓` | scroll a line |
| `space` `b` `PgDn` `PgUp` | page down / up |
| `d` `u` | half page |
| `g` `G` | top / bottom |
| `]` `[` | next / previous heading |
| `t` `tab` | table of contents |
| `/` then `n` `N` | search (smart-case), next / previous match |
| `r` | reload now |
| `?` | help |
| `q` `ctrl-c` | quit |

## Build

```sh
go build -o mdpeek .
go test ./...
```

Cross-compile any target (no C toolchain needed):

```sh
GOOS=windows GOARCH=amd64 go build -o mdpeek.exe .
GOOS=darwin  GOARCH=arm64 go build -o mdpeek-mac .
```

## CI and releases

- `.github/workflows/ci.yml` — vet, tests (race detector on Linux/macOS), gofmt, and a smoke test on **Ubuntu, macOS and Windows**; cross-compiles seven OS/arch targets; fuzzes the renderer on pull requests.
- `.github/workflows/release.yml` — push a tag to publish binaries via [GoReleaser](https://goreleaser.com):

  ```sh
  git tag v1.0.0 && git push origin v1.0.0
  ```

  This produces `tar.gz` (zip on Windows) archives for linux/darwin/windows/freebsd on amd64 and arm64, plus `checksums.txt`. Run the workflow manually for a snapshot build without publishing.

## Design

| Package | Role |
|---------|------|
| `internal/render` | Markdown → wrapped, styled lines (block + inline parsers, tables, highlighter, sanitiser) |
| `internal/pager`  | full-screen viewer: scrolling, search, TOC, live reload |
| `internal/term`   | raw mode, window size and key decoding per OS (Unix termios, Windows console VT) |
| `internal/watch`  | portable polling file watcher |

The standard library is the only dependency, which keeps builds reproducible and cross-compilation trivial.

## License

MIT
