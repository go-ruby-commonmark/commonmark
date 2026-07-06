<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-commonmark/brand/main/social/go-ruby-commonmark-commonmark.png" alt="go-ruby-commonmark/commonmark" width="720"></p>

# commonmark — go-ruby-commonmark

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-commonmark.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) CommonMark renderer** — the deterministic,
interpreter-independent core that backs Ruby's
[commonmarker](https://github.com/gjtorikian/commonmarker) gem. It parses
CommonMark [v0.31.2](https://spec.commonmark.org/0.31.2/) source into a node tree
and renders it to an HTML fragment, and it can additionally enable a subset of the
GitHub Flavored Markdown extensions behind `Options` — **without any Ruby
runtime, and without a cgo binding to libcmark**.

It is the Markdown backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a sibling
of [go-ruby-kramdown](https://github.com/go-ruby-kramdown/kramdown) and
[go-ruby-liquid](https://github.com/go-ruby-liquid/liquid).

> **Conformance is honest, not aspirational.** The parser is validated against the
> upstream `spec.txt` conformance suite: **593 / 652 examples pass** today. The
> remaining failures are concentrated in a handful of hard emphasis-nesting edge
> cases, lazy list-item continuation, and the spec's raw-HTML-block corner cases
> (many of which the reference renderer only produces in "unsafe" mode). Every
> spec example is embedded and run on every CI lane; the exact pass rate is
> asserted in the test suite, so it can only go up.

## Features

- **Full block grammar** — ATX and Setext headings, block quotes, bullet and
  ordered lists (tight/loose), indented and fenced code blocks, thematic breaks,
  HTML blocks, and link reference definitions.
- **Full inline grammar** — emphasis / strong with the CommonMark flanking and
  delimiter-stack algorithm, code spans, links and images (inline, reference,
  collapsed, and shortcut), autolinks, raw inline HTML, backslash escapes, hard
  and soft line breaks, and the complete HTML5 named + numeric entity table.
- **Safe by default** — raw HTML and dangerous URL schemes (`javascript:`,
  `vbscript:`, `file:`, and non-image `data:`) are filtered unless `Unsafe` is
  set. URL destinations are normalised and percent-encoded like the reference
  renderer.
- **GFM extensions behind `Options`** — pipe **tables** (with per-column
  alignment), **strikethrough** (`~~…~~`), extended **autolinks** (bare
  `http(s)://` and `www.` links with GFM trailing-punctuation trimming), and
  **task list** checkboxes (`- [ ]` / `- [x]`). All are off by default, so the
  zero value is strict CommonMark.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x).

## Install

```sh
go get github.com/go-ruby-commonmark/commonmark
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-commonmark/commonmark"
)

func main() {
	// Strict CommonMark (the safe default).
	fmt.Print(commonmark.ToHTML("# Hello\n\nsome *emphasis*.\n", nil))
	// <h1>Hello</h1>
	// <p>some <em>emphasis</em>.</p>

	// Enable GFM tables, strikethrough, autolinks and task lists.
	opts := &commonmark.Options{
		Tables:        true,
		Strikethrough: true,
		Autolink:      true,
		TaskList:      true,
	}
	fmt.Print(commonmark.ToHTML("- [x] ~~done~~ see https://example.com\n", opts))
	// <ul>
	// <li><input type="checkbox" checked="" disabled="" /> <del>done</del> see <a href="https://example.com">https://example.com</a></li>
	// </ul>
}
```

## API

```go
// ToHTML converts CommonMark source to an HTML fragment. A nil opts selects the
// strict CommonMark default.
func ToHTML(src string, opts *Options) string

// Parse parses CommonMark source into a Document node tree for callers that need
// structured access. A nil opts selects the strict CommonMark default.
func Parse(src string, opts *Options) *Node

type Options struct {
	Tables        bool // GFM pipe tables
	Strikethrough bool // GFM ~~text~~
	Autolink      bool // GFM extended bare-URL / www. autolinks
	TaskList      bool // GFM - [ ] / - [x] checkboxes
	GitHubPreLang bool // emit fenced-code language on <pre> (GitHub style)
	Unsafe        bool // pass raw HTML and unsafe URL schemes through
	HardBreaks    bool // render every soft line break as <br />
}
```

## Tests & coverage

The suite embeds the upstream CommonMark `spec.txt` and runs all 652 examples on
every lane (reporting the honest pass rate), alongside targeted block/inline/GFM
and error-path tests that hold statement coverage at **100%** — so the qemu
cross-arch and Windows lanes pass the coverage gate. The host lane keeps cgo
enabled so `-race` runs; the six architecture lanes build and test with
`CGO_ENABLED=0` to prove the pure-Go build.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-commonmark/commonmark authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
