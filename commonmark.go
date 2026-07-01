// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package commonmark is a pure-Go (CGO=0) strict CommonMark renderer. It
// implements the CommonMark specification v0.31.2 — the same core that backs the
// Ruby `commonmarker` gem — and can additionally enable a subset of the GFM
// extensions (tables, strikethrough, autolinks, task lists) behind Options.
//
// The primary entry point is ToHTML, which converts a Markdown source string to
// an HTML fragment. Parse exposes the intermediate Document node tree for callers
// that need structured access.
package commonmark

// Options configures the renderer. The zero value (or a nil *Options) selects
// strict CommonMark with all extensions disabled, which is the safe default.
type Options struct {
	// Tables enables GFM pipe tables.
	Tables bool
	// Strikethrough enables GFM `~~text~~` strikethrough.
	Strikethrough bool
	// Autolink enables GFM extended autolinks (bare URLs and www links).
	Autolink bool
	// TaskList enables GFM `- [ ]` / `- [x]` task list items.
	TaskList bool

	// GitHubPreLang, when true, emits fenced code language as a class on the
	// <pre> element the way commonmarker's GitHub renderer does. When false
	// (the CommonMark default) the class is emitted on the <code> element.
	GitHubPreLang bool

	// Unsafe, when true, passes raw HTML and unsafe URL schemes through instead
	// of filtering them. The CommonMark reference renderer used by the spec test
	// suite is unsafe, so the conformance tests set this.
	Unsafe bool

	// HardBreaks, when true, renders every soft line break as a <br />.
	HardBreaks bool
}

// gfmAll returns an Options with every GFM extension enabled.
func gfmAll() *Options {
	return &Options{Tables: true, Strikethrough: true, Autolink: true, TaskList: true}
}

// ToHTML converts CommonMark source to an HTML fragment. A nil opts selects the
// strict CommonMark default.
func ToHTML(src string, opts *Options) string {
	doc := Parse(src, opts)
	return renderHTML(doc, opts)
}

// Parse parses CommonMark source into a Document node tree. A nil opts selects
// the strict CommonMark default. The returned node has Type == Document.
func Parse(src string, opts *Options) *Node {
	if opts == nil {
		opts = &Options{}
	}
	p := newParser(opts)
	return p.parse(src)
}
