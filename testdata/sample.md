---
title: mdpeek sample
tags: [demo, markdown]
---

# mdpeek sample document

A paragraph with **bold**, *italic*, ***both***, ~~strikethrough~~, `inline code`, and a
[link to the docs](https://example.com/docs). Soft line breaks are joined,
and a bare URL like https://example.com/path?q=1 becomes a link too.
Hard break here  
and another one\
done.

## Lists

- First item
- Second item with a much longer line of text that has to wrap around the available width of the terminal nicely
  - Nested bullet
  - Another nested bullet
    - Third level
- [x] Done task
- [ ] Open task

1. Install the tool
2. Run it
   ```bash
   mdpeek README.md --theme dark
   ```
3. Enjoy

## Table

| Feature     | Linux | macOS | Windows |  Notes          |
|:------------|:-----:|:-----:|:-------:|----------------:|
| Pager       |  yes  |  yes  |   yes   | raw mode / VT   |
| Live reload |  yes  |  yes  |   yes   | polling         |
| Wide: 日本語 |  ok   |  ok   |   ok    | double width    |

## Code

```go
package main

import "fmt"

// main prints a greeting.
func main() {
	name := "world" /* inline block */
	fmt.Printf("hello, %s %d\n", name, 42)
}
```

    indented code block

### Quotes and alerts

> A blockquote that spans a couple of lines and wraps nicely when the terminal is narrow.
>
> > Nested quote.

> [!WARNING]
> Untrusted Markdown cannot inject terminal escapes: &#27;]0;pwned&#7; stays inert.

#### Fourth level heading

---

Setext Heading
==============

Footnote-ish reference link: [Go][go] and an image ![logo](logo.png).

<!-- this comment is hidden -->

<p align="center"><img src="x.png" alt="Centered badge"></p>

[go]: https://go.dev "The Go Programming Language"
