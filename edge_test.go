// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import "testing"

// TestAutolinkNoDomain covers the reDomain no-match branches for http:// and
// www. prefixes with no following domain.
func TestAutolinkNoDomain(t *testing.T) {
	o := gfmAll()
	eq(t, "http:// nodomain\n", o, "<p>http:// nodomain</p>\n")
	eq(t, "www. nodomain\n", o, "<p>www. nodomain</p>\n")
}

// TestTaskMarkerLeadingSpace covers the leading-space skip before a task marker,
// reached when the item marker's padding leaves a leading space in the content.
func TestTaskMarkerLeadingSpace(t *testing.T) {
	eq(t, "1.  [ ] x\n", gfmAll(),
		"<ol>\n<li><input type=\"checkbox\" disabled=\"\" /> x</li>\n</ol>\n")
}

// TestTableCellBackslash covers the splitTableRow branch where a backslash is
// not followed by a pipe (kept literal).
func TestTableCellBackslash(t *testing.T) {
	got := ToHTML("| a\\b |\n| - |\n| x |\n", gfmAll())
	want := "<table>\n<thead>\n<tr>\n<th>a\\b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>x</td>\n</tr>\n</tbody>\n</table>\n"
	if got != want {
		t.Errorf("table backslash cell:\n got: %q\nwant: %q", got, want)
	}
}

// TestFencedCodeNoNewline covers finalizeCodeBlock when the fence has no body
// newline (EOF right after the opening fence).
func TestFencedCodeNoNewline(t *testing.T) {
	eq(t, "```", nil, "<pre><code></code></pre>\n")
}

// TestIndentedCodeAllBlank covers stripTrailingBlankLines returning nil for an
// all-blank indented code block (which is then dropped).
func TestIndentedCodeAllBlank(t *testing.T) {
	eq(t, "    \n    \n", nil, "")
}

// TestLinkDestUnbalancedParen covers the parseLinkDestination depth!=0 rejection
// (a bare destination whose parentheses are left unbalanced at a break).
func TestLinkDestUnbalancedParen(t *testing.T) {
	eq(t, "[x](a(b c)\n", nil, "<p>[x](a(b c)</p>\n")
}

// TestEmphasisInsideLink exercises processEmphasis with a non-nil stackBottom.
// An unclosed emphasis inside the link text leaves a delimiter above the bottom,
// exercising the "remove delimiters above stackBottom" branch.
func TestEmphasisInsideLink(t *testing.T) {
	eq(t, "[*a*](/u)\n", nil, "<p><a href=\"/u\"><em>a</em></a></p>\n")
	eq(t, "[a *b](/u)\n", nil, "<p><a href=\"/u\">a *b</a></p>\n")
}

// TestNestedListBlankLine exercises endsWithBlankLine walking into a nested
// list's last child and returning at a non-list node, plus the empty-item case
// where the walk reaches a nil last child.
func TestNestedListBlankLine(t *testing.T) {
	eq(t, "- a\n  - b\n\n- c\n", nil,
		"<ul>\n<li>\n<p>a</p>\n<ul>\n<li>b</li>\n</ul>\n</li>\n<li>\n<p>c</p>\n</li>\n</ul>\n")
	// An empty list item (no children) followed by another item: the tightness
	// check walks into the empty item's nil last child.
	eq(t, "-\n\n- x\n", nil,
		"<ul>\n<li></li>\n<li>\n<p>x</p>\n</li>\n</ul>\n")
}

// --- whitebox tests for internal-only branches ----------------------------

// TestSkipWhitespaceOneNewline directly covers the two-newline break in
// skipWhitespaceIncludingOneNewline.
func TestSkipWhitespaceOneNewline(t *testing.T) {
	s := &subject{buf: []byte("  \n \n x")}
	if !s.skipWhitespaceIncludingOneNewline() {
		t.Fatal("expected whitespace to be skipped")
	}
	// It must stop at the second newline.
	if s.pos != 4 { // "  \n " -> positions 0..3, stop before second '\n' at index 4
		t.Fatalf("stopped at pos %d, want 4 (buf=%q)", s.pos, s.buf)
	}
	// No whitespace at all: returns false.
	s2 := &subject{buf: []byte("x")}
	if s2.skipWhitespaceIncludingOneNewline() {
		t.Error("expected no whitespace skipped")
	}
}

// TestParseBracketLabelEscape directly covers the escape (\\) handling in
// parseBracketLabel.
func TestParseBracketLabelEscape(t *testing.T) {
	// Escaped ] does not terminate the label.
	s := &subject{buf: []byte(`[a\]b]`)}
	got, ok := s.parseBracketLabel()
	if !ok || got != `a\]b` {
		t.Fatalf("escaped label = %q,%v", got, ok)
	}
	s = &subject{buf: []byte(`[abc]`)}
	got, ok = s.parseBracketLabel()
	if !ok || got != "abc" {
		t.Fatalf("plain label = %q,%v", got, ok)
	}
	// A nested '[' is rejected.
	s = &subject{buf: []byte(`[a[b]`)}
	if _, ok := s.parseBracketLabel(); ok {
		t.Fatal("nested bracket should be rejected")
	}
	// Unterminated is rejected.
	s = &subject{buf: []byte(`[abc`)}
	if _, ok := s.parseBracketLabel(); ok {
		t.Fatal("unterminated should be rejected")
	}
}

// mkSubject builds a subject over buf with a fresh paragraph block.
func mkSubject(buf string) *subject {
	ip := newInlineParser(&Options{}, map[string]*linkRef{})
	return &subject{buf: []byte(buf), block: newNode(Paragraph), ip: ip}
}

// TestParseNewlineLeadingSpaces covers the leading-space skip loops in
// parseNewline (soft break) and parseBackslash (hard break) at the subject
// level, where continuation-line indentation survives block splitting.
func TestParseNewlineLeadingSpaces(t *testing.T) {
	s := mkSubject("a\n  b")
	for s.parseInline() {
	}
	if s.pos != len(s.buf) {
		t.Errorf("soft-break leading-space skip left pos=%d", s.pos)
	}
	s = mkSubject("a\\\n  b")
	for s.parseInline() {
	}
	if s.pos != len(s.buf) {
		t.Errorf("hard-break leading-space skip left pos=%d", s.pos)
	}
}

// TestSetextFirstLine covers the setext-underline-with-no-paragraph case.
func TestSetextFirstLine(t *testing.T) {
	// A '===' as the very first line has no preceding paragraph; it is a
	// paragraph itself.
	eq(t, "===\n", nil, "<p>===</p>\n")
}

// TestStripTaskMarkerLeadingSpace covers the leading-space skip in
// stripTaskMarker directly (block parsing usually trims this).
func TestStripTaskMarkerLeadingSpace(t *testing.T) {
	item := newNode(Item)
	p := newNode(Paragraph)
	item.appendChild(p)
	got := stripTaskMarker(p, []byte("  [ ] hi"))
	if string(got) != "   hi" {
		t.Errorf("stripTaskMarker leading space = %q", got)
	}
	if item.task != taskUnchecked {
		t.Errorf("task state = %v, want unchecked", item.task)
	}
}

// TestEndsWithBlankLineEmptyItem covers endsWithBlankLine walking into an empty
// Item's nil last child and returning false.
func TestEndsWithBlankLineEmptyItem(t *testing.T) {
	if endsWithBlankLine(newNode(Item)) {
		t.Error("empty item should not end with a blank line")
	}
	// A non-list, non-item block also returns false.
	if endsWithBlankLine(newNode(Paragraph)) {
		t.Error("paragraph without blank line should return false")
	}
}

// TestProcessEmphasisStackBottom covers the "remove delimiters above
// stackBottom" branch of processEmphasis by leaving an unmatched delimiter and
// invoking processEmphasis with a non-nil bottom.
func TestProcessEmphasisStackBottom(t *testing.T) {
	// Two '*' runs produce two delimiters; both can open but neither closes, so
	// after processing with the lower one as stackBottom the upper one remains
	// above it and must be removed by the else-branch loop.
	s := mkSubject("*a *b")
	for s.parseInline() {
	}
	bottom := s.delimiters
	for bottom.prev != nil {
		bottom = bottom.prev
	}
	if bottom == s.delimiters {
		t.Fatal("expected at least two delimiters")
	}
	s.processEmphasis(bottom)
	if s.delimiters != bottom {
		t.Errorf("delimiters above stackBottom were not cleared: top=%p bottom=%p", s.delimiters, bottom)
	}
}
