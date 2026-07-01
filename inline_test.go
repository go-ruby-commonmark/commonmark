// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import (
	"strings"
	"testing"
)

// TestSoftbreakLeadingSpaces covers the leading-space skip after a soft break
// and the hard-break (two trailing spaces) path.
func TestSoftbreakLeadingSpaces(t *testing.T) {
	// Soft break, next line indented: leading spaces are stripped.
	eq(t, "a\n   b\n", nil, "<p>a\nb</p>\n")
	// Hard break via two trailing spaces, then indented next line.
	eq(t, "a  \n   b\n", nil, "<p>a<br />\nb</p>\n")
	// Hard break via backslash-newline, then indented next line.
	eq(t, "a\\\n   b\n", nil, "<p>a<br />\nb</p>\n")
}

// TestNumericEntities covers decimal, hex, invalid, out-of-range and named
// entity references, plus the U+FFFD replacements.
func TestNumericEntities(t *testing.T) {
	eq(t, "&#65;\n", nil, "<p>A</p>\n")           // decimal
	eq(t, "&#x41;\n", nil, "<p>A</p>\n")          // hex
	eq(t, "&amp;\n", nil, "<p>&amp;</p>\n")       // named
	eq(t, "&#0;\n", nil, "<p>�</p>\n")            // NUL -> replacement
	eq(t, "&#xD800;\n", nil, "<p>�</p>\n")        // surrogate -> replacement
	eq(t, "&#9999999;\n", nil, "<p>�</p>\n")      // out of range (7 digits)
	eq(t, "&nope;\n", nil, "<p>&amp;nope;</p>\n") // unknown named entity
	eq(t, "&#;\n", nil, "<p>&amp;#;</p>\n")       // empty numeric
	eq(t, "&notanentity\n", nil, "<p>&amp;notanentity</p>\n")
}

// TestMatchEntityShort exercises the too-short guard in matchEntity directly.
func TestMatchEntityShort(t *testing.T) {
	if r, n := matchEntity([]byte("&x")); n != 0 || r != "" {
		t.Errorf("matchEntity short = %q,%d", r, n)
	}
	if r, n := matchEntity([]byte("no-amp")); n != 0 || r != "" {
		t.Errorf("matchEntity no-& = %q,%d", r, n)
	}
}

// TestCodepointToString covers the direct conversion helper's replacement and
// valid branches.
func TestCodepointToString(t *testing.T) {
	if codepointToString(0) != "�" {
		t.Error("NUL should map to replacement")
	}
	if codepointToString(0xD800) != "�" {
		t.Error("surrogate should map to replacement")
	}
	if codepointToString(0x110000) != "�" {
		t.Error("out-of-range should map to replacement")
	}
	if codepointToString('A') != "A" {
		t.Error("ASCII should round-trip")
	}
}

// TestReferenceDefEdges exercises link reference definition parsing including
// escaped brackets in the label, the whitespace-with-one-newline skip, and the
// two-newline break.
func TestReferenceDefEdges(t *testing.T) {
	// Escaped ] inside the definition label.
	eq(t, "[a\\]b]: /u\n\n[a\\]b]\n", nil, "<p><a href=\"/u\">a]b</a></p>\n")
	// Definition split across a single newline between label and destination.
	eq(t, "[foo]:\n/u\n\n[foo]\n", nil, "<p><a href=\"/u\">foo</a></p>\n")
	// A blank line (two newlines) between label and url breaks the definition, so
	// it stays a paragraph.
	eq(t, "[foo]:\n\n/u\n", nil, "<p>[foo]:</p>\n<p>/u</p>\n")
	// Nested '[' inside a reference-def label is rejected.
	eq(t, "[a[b]: /u\n", nil, "<p>[a[b]: /u</p>\n")
}

// TestParseLinkLabelDirect exercises the length-limit and nested-bracket guards.
func TestParseLinkLabelDirect(t *testing.T) {
	// Over the 999-char limit -> rejected.
	long := "[" + strings.Repeat("a", 1001) + "]"
	s := &subject{buf: []byte(long)}
	if _, ok := s.parseLinkLabel(); ok {
		t.Error("over-length label should be rejected")
	}
	// Nested '[' -> rejected.
	s = &subject{buf: []byte("[a[b]")}
	if _, ok := s.parseLinkLabel(); ok {
		t.Error("nested bracket label should be rejected")
	}
	// Unterminated -> rejected.
	s = &subject{buf: []byte("[abc")}
	if _, ok := s.parseLinkLabel(); ok {
		t.Error("unterminated label should be rejected")
	}
	// Not a '[' at all.
	s = &subject{buf: []byte("x")}
	if _, ok := s.parseLinkLabel(); ok {
		t.Error("non-bracket should be rejected")
	}
}

// TestFullReferenceLinks exercises full reference links `[text][label]`,
// including escaped and nested brackets in the second label (parseBracketLabel).
func TestFullReferenceLinks(t *testing.T) {
	// Full reference with an escaped ] in the reference label.
	eq(t, "[a\\]b]: /u\n\n[text][a\\]b]\n", nil, "<p><a href=\"/u\">text</a></p>\n")
	// A '[' inside the reference label makes it not a valid label (exercises the
	// nested-bracket rejection in parseBracketLabel).
	eq(t, "[text]: /u\n\n[text][a[b]\n", nil, "<p>[text][a[b]</p>\n")
	// Unterminated reference label after ][ (exercises the fall-through path).
	eq(t, "[text]: /u\n\n[text][unterminated\n", nil, "<p>[text][unterminated</p>\n")
}

// TestParseBracketLabelDirect exercises the over-length guard.
func TestParseBracketLabelDirect(t *testing.T) {
	long := "[" + strings.Repeat("a", 1001) + "]"
	s := &subject{buf: []byte(long)}
	if _, ok := s.parseBracketLabel(); ok {
		t.Error("over-length bracket label should be rejected")
	}
}

// TestBottomDelimiterNil exercises the nil-stack path in processEmphasis via a
// document with no delimiters at all.
func TestBottomDelimiterNil(t *testing.T) {
	s := &subject{}
	if d := s.bottomDelimiter(); d != nil {
		t.Errorf("bottomDelimiter on empty subject = %v, want nil", d)
	}
}

// TestParenTitleRejected covers parseLinkTitle's unescaped '(' rejection inside
// a (...) title and the unclosed-title path.
func TestParenTitleRejected(t *testing.T) {
	// Unescaped '(' inside a (...) title makes the inline link fail; it renders
	// as literal text.
	eq(t, "[x](/u (a(b))\n", nil, "<p>[x](/u (a(b))</p>\n")
	// A properly escaped inner paren is accepted.
	eq(t, "[x](/u (a\\(b))\n", nil, "<p><a href=\"/u\" title=\"a(b\">x</a></p>\n")
}
