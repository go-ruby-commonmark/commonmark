// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import (
	"strings"
	"testing"
)

// eq is a small helper: it renders src with opts and compares to want.
func eq(t *testing.T, src string, opts *Options, want string) {
	t.Helper()
	got := ToHTML(src, opts)
	if got != want {
		t.Errorf("ToHTML(%q)\n got: %q\nwant: %q", src, got, want)
	}
}

// TestToHTMLBasics exercises the public entry point on a spread of core
// constructs so the render dispatch and common block/inline paths are covered
// outside the spec harness.
func TestToHTMLBasics(t *testing.T) {
	cases := []struct{ src, want string }{
		{"hello\n", "<p>hello</p>\n"},
		{"# Heading\n", "<h1>Heading</h1>\n"},
		{"###### h6\n", "<h6>h6</h6>\n"},
		{"para\n===\n", "<h1>para</h1>\n"},
		{"para\n---\n", "<h2>para</h2>\n"},
		{"> quote\n", "<blockquote>\n<p>quote</p>\n</blockquote>\n"},
		{"* a\n* b\n", "<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n"},
		{"1. a\n2. b\n", "<ol>\n<li>a</li>\n<li>b</li>\n</ol>\n"},
		{"3. a\n", "<ol start=\"3\">\n<li>a</li>\n</ol>\n"},
		{"---\n", "<hr />\n"},
		{"    code\n", "<pre><code>code\n</code></pre>\n"},
		{"*em* **strong**\n", "<p><em>em</em> <strong>strong</strong></p>\n"},
		{"`code span`\n", "<p><code>code span</code></p>\n"},
		{"[link](/url \"t\")\n", "<p><a href=\"/url\" title=\"t\">link</a></p>\n"},
		{"![alt](/img)\n", "<p><img src=\"/img\" alt=\"alt\" /></p>\n"},
		{"a\\\nb\n", "<p>a<br />\nb</p>\n"},
		{"", ""},
	}
	for _, c := range cases {
		eq(t, c.src, nil, c.want)
	}
}

// TestParseNilAndZeroOptions checks that Parse tolerates a nil *Options and
// returns a Document root.
func TestParseNilAndZeroOptions(t *testing.T) {
	doc := Parse("hi\n", nil)
	if doc == nil || doc.Type != Document {
		t.Fatalf("Parse(nil opts) = %v, want a Document node", doc)
	}
	if doc.FirstChild == nil || doc.FirstChild.Type != Paragraph {
		t.Fatalf("expected a paragraph child, got %+v", doc.FirstChild)
	}
	doc2 := Parse("hi\n", &Options{})
	if doc2 == nil || doc2.Type != Document {
		t.Fatalf("Parse(zero opts) = %v, want a Document node", doc2)
	}
}

// TestRenderHTMLNilOptions checks the renderHTML nil-options guard.
func TestRenderHTMLNilOptions(t *testing.T) {
	doc := Parse("x\n", nil)
	if got := renderHTML(doc, nil); got != "<p>x</p>\n" {
		t.Fatalf("renderHTML nil opts = %q", got)
	}
}

// TestHardBreaks verifies the HardBreaks option turns soft breaks into <br />.
func TestHardBreaks(t *testing.T) {
	eq(t, "a\nb\n", &Options{HardBreaks: true}, "<p>a<br />\nb</p>\n")
	// Without it, a soft break is a newline.
	eq(t, "a\nb\n", nil, "<p>a\nb</p>\n")
}

// TestGitHubPreLang verifies the alternate fenced-code language emission.
func TestGitHubPreLang(t *testing.T) {
	eq(t, "```go\nx\n```\n", &Options{GitHubPreLang: true},
		"<pre lang=\"go\"><code>x\n</code></pre>\n")
	// Default CommonMark places the class on <code>.
	eq(t, "```go\nx\n```\n", nil,
		"<pre><code class=\"language-go\">x\n</code></pre>\n")
	// No info string: plain <pre><code>.
	eq(t, "```\nx\n```\n", &Options{GitHubPreLang: true},
		"<pre><code>x\n</code></pre>\n")
}

// TestUnsafeFiltering covers both the safe (filtered) and unsafe (pass-through)
// rendering paths for raw HTML and dangerous URL schemes.
func TestUnsafeFiltering(t *testing.T) {
	// Raw HTML block.
	eq(t, "<div>hi</div>\n", nil, "<!-- raw HTML omitted -->\n")
	eq(t, "<div>hi</div>\n", &Options{Unsafe: true}, "<div>hi</div>\n")
	// Inline raw HTML.
	eq(t, "a <span>b</span>\n", nil, "<p>a <!-- raw HTML omitted -->b<!-- raw HTML omitted --></p>\n")
	eq(t, "a <span>b</span>\n", &Options{Unsafe: true}, "<p>a <span>b</span></p>\n")
	// Dangerous URL schemes are dropped when not unsafe.
	eq(t, "[x](javascript:alert(1))\n", nil, "<p><a href=\"\">x</a></p>\n")
	eq(t, "[x](javascript:alert(1))\n", &Options{Unsafe: true},
		"<p><a href=\"javascript:alert(1)\">x</a></p>\n")
}

// TestIsUnsafeURL directly exercises the scheme filter, including the data:
// image whitelist branch.
func TestIsUnsafeURL(t *testing.T) {
	cases := []struct {
		url    string
		unsafe bool
	}{
		{"javascript:x", true},
		{"vbscript:x", true},
		{"file:///etc/passwd", true},
		{"data:text/html,x", true},
		{"data:image/png;base64,AAAA", false},
		{"data:image/gif;base64,AAAA", false},
		{"data:image/jpeg;base64,AAAA", false},
		{"data:image/webp;base64,AAAA", false},
		{"http://ok", false},
		{"/relative", false},
		{"  javascript:x", true}, // leading whitespace trimmed
	}
	for _, c := range cases {
		if got := isUnsafeURL([]byte(c.url)); got != c.unsafe {
			t.Errorf("isUnsafeURL(%q) = %v, want %v", c.url, got, c.unsafe)
		}
	}
}

// TestPreprocess covers CRLF / CR normalisation and NUL replacement.
func TestPreprocess(t *testing.T) {
	if got := preprocess("a\r\nb\rc\n"); got != "a\nb\nc\n" {
		t.Errorf("CRLF/CR normalise = %q", got)
	}
	if got := preprocess("a\x00b"); got != "a�b" {
		t.Errorf("NUL replace = %q", got)
	}
	if got := preprocess("plain\n"); got != "plain\n" {
		t.Errorf("passthrough = %q", got)
	}
	// End-to-end: CRLF source renders like LF source.
	eq(t, "a\r\nb\r\n", nil, "<p>a\nb</p>\n")
	// NUL in source becomes U+FFFD in output.
	eq(t, "a\x00b\n", nil, "<p>a�b</p>\n")
}

// TestSplitLines covers the edge cases of the line splitter.
func TestSplitLines(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{""}},
		{"a", []string{"a"}},
		{"a\n", []string{"a"}},
		{"a\nb", []string{"a", "b"}},
		{"a\nb\n", []string{"a", "b"}},
		{"\n", []string{""}},
	}
	for _, c := range cases {
		got := splitLines(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitLines(%q) len = %d, want %d (%q)", c.in, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if string(got[i]) != c.want[i] {
				t.Errorf("splitLines(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// TestURLEncodeAndNormalize covers destination percent-encoding, existing
// escape preservation, ampersand handling and invalid-UTF-8 normalisation.
func TestURLEncodeAndNormalize(t *testing.T) {
	var sb strings.Builder
	urlEncode(&sb, []byte("/a b&c%20dé"))
	// Space -> %20, & -> &amp;, existing %20 preserved, é -> %C3%A9.
	if got := sb.String(); got != "/a%20b&amp;c%20d%C3%A9" {
		t.Errorf("urlEncode = %q", got)
	}
	// Invalid UTF-8 gets replaced by U+FFFD before encoding.
	got := normalizeURI([]byte{'a', 0xff, 'b'})
	if string(got) != "a�b" {
		t.Errorf("normalizeURI invalid = %q", got)
	}
	if string(normalizeURI([]byte("ok"))) != "ok" {
		t.Errorf("normalizeURI valid changed input")
	}
	// End-to-end through a link (angle-bracketed destination allows a space).
	eq(t, "[x](</a b>)\n", nil, "<p><a href=\"/a%20b\">x</a></p>\n")
}

// TestCaseFold covers the special folds and the default lowercase path via
// reference-link label matching.
func TestCaseFold(t *testing.T) {
	if caseFold('µ') != "μ" {
		t.Error("micro sign should fold to mu")
	}
	if caseFold('ẞ') != "ss" {
		t.Error("capital sharp s should fold to ss")
	}
	if caseFold('A') != "a" {
		t.Error("ASCII should lowercase")
	}
	// Reference definitions match case-insensitively with whitespace collapse.
	eq(t, "[FOO   bar]\n\n[foo bar]: /u\n", nil, "<p><a href=\"/u\">FOO   bar</a></p>\n")
}

// TestNormalizeLabelWhitespace covers the internal-whitespace-collapse path in
// normalizeLabel via reference matching.
func TestNormalizeLabelWhitespace(t *testing.T) {
	if got := normalizeLabel([]byte("  Foo\tBar  ")); got != "foo bar" {
		t.Errorf("normalizeLabel = %q", got)
	}
}
