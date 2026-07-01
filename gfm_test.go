// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import "testing"

// TestGFMAllHelper checks the gfmAll convenience constructor enables every
// extension.
func TestGFMAllHelper(t *testing.T) {
	o := gfmAll()
	if !o.Tables || !o.Strikethrough || !o.Autolink || !o.TaskList {
		t.Fatalf("gfmAll did not enable all extensions: %+v", o)
	}
}

// TestStrikethrough covers the ~~...~~ extension and its off-by-default state.
func TestStrikethrough(t *testing.T) {
	eq(t, "~~gone~~\n", &Options{Strikethrough: true}, "<p><del>gone</del></p>\n")
	// Off by default: tildes are literal.
	eq(t, "~~gone~~\n", nil, "<p>~~gone~~</p>\n")
	// GFM also accepts a single-tilde delimiter run.
	eq(t, "~gone~\n", &Options{Strikethrough: true}, "<p><del>gone</del></p>\n")
}

// TestGFMAutolinks covers extended (bare) autolinks: http, https, www, trailing
// punctuation trimming and balanced-paren handling.
func TestGFMAutolinks(t *testing.T) {
	o := &Options{Autolink: true}
	eq(t, "see http://example.com here\n", o,
		"<p>see <a href=\"http://example.com\">http://example.com</a> here</p>\n")
	eq(t, "see https://example.com/p here\n", o,
		"<p>see <a href=\"https://example.com/p\">https://example.com/p</a> here</p>\n")
	eq(t, "see www.example.com here\n", o,
		"<p>see <a href=\"http://www.example.com\">www.example.com</a> here</p>\n")
	// Trailing punctuation is trimmed from the link and left as text.
	eq(t, "at http://example.com.\n", o,
		"<p>at <a href=\"http://example.com\">http://example.com</a>.</p>\n")
	// Balanced parens: the closing paren belongs to the URL.
	eq(t, "http://example.com/a(b)\n", o,
		"<p><a href=\"http://example.com/a(b)\">http://example.com/a(b)</a></p>\n")
	// Unbalanced trailing paren is trimmed off.
	eq(t, "(http://example.com)\n", o,
		"<p>(<a href=\"http://example.com\">http://example.com</a>)</p>\n")
	// A URL immediately preceded by an alnum or slash is not an autolink.
	eq(t, "xhttp://example.com\n", o, "<p>xhttp://example.com</p>\n")
	// Off by default.
	eq(t, "http://example.com\n", nil,
		"<p>http://example.com</p>\n")
}

// TestGFMTables covers pipe tables: alignment, header/body, ragged rows,
// escaped pipes, and the rejection paths.
func TestGFMTables(t *testing.T) {
	o := &Options{Tables: true}

	got := ToHTML("| a | b |\n| :- | -: |\n| 1 | 2 |\n", o)
	want := "<table>\n<thead>\n<tr>\n<th align=\"left\">a</th>\n<th align=\"right\">b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td align=\"left\">1</td>\n<td align=\"right\">2</td>\n</tr>\n</tbody>\n</table>\n"
	if got != want {
		t.Errorf("aligned table:\n got: %q\nwant: %q", got, want)
	}

	// Centre alignment and a header-only table (no body rows).
	got = ToHTML("| h |\n| :-: |\n", o)
	want = "<table>\n<thead>\n<tr>\n<th align=\"center\">h</th>\n</tr>\n</thead>\n</table>\n"
	if got != want {
		t.Errorf("center header-only:\n got: %q\nwant: %q", got, want)
	}

	// Ragged body row: fewer cells than columns -> empty trailing cells; extra
	// cells are ignored.
	got = ToHTML("| a | b |\n| - | - |\n| 1 |\n| 1 | 2 | 3 |\n", o)
	want = "<table>\n<thead>\n<tr>\n<th>a</th>\n<th>b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>1</td>\n<td></td>\n</tr>\n<tr>\n<td>1</td>\n<td>2</td>\n</tr>\n</tbody>\n</table>\n"
	if got != want {
		t.Errorf("ragged rows:\n got: %q\nwant: %q", got, want)
	}

	// Escaped pipe inside a cell stays literal.
	got = ToHTML("| a |\n| - |\n| x \\| y |\n", o)
	want = "<table>\n<thead>\n<tr>\n<th>a</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>x | y</td>\n</tr>\n</tbody>\n</table>\n"
	if got != want {
		t.Errorf("escaped pipe:\n got: %q\nwant: %q", got, want)
	}

	// Rejections fall back to a paragraph.
	// Fewer than two lines.
	eq(t, "| a |\n", o, "<p>| a |</p>\n")
	// Delimiter row column count differs from header.
	eq(t, "| a | b |\n| - |\n", o, "<p>| a | b |\n| - |</p>\n")
	// Non-dash content in the delimiter row.
	eq(t, "| a |\n| xx |\n", o, "<p>| a |\n| xx |</p>\n")
	// Empty delimiter cell.
	eq(t, "| a | b |\n| - |  |\n", o, "<p>| a | b |\n| - |  |</p>\n")
	// Off by default: a table renders as a plain paragraph.
	eq(t, "| a |\n| - |\n", nil, "<p>| a |\n| - |</p>\n")
}

// TestTableDelimEdge exercises the parseTableDelim column-count and colon-only
// rejection branches directly.
func TestTableDelimEdge(t *testing.T) {
	if _, ok := parseTableDelim([]byte("")); ok {
		t.Error("empty delimiter should not parse")
	}
	if _, ok := parseTableDelim([]byte("| : |")); ok {
		t.Error("colon-only delimiter cell should not parse")
	}
	aligns, ok := parseTableDelim([]byte("| - | :-: |"))
	if !ok || len(aligns) != 2 || aligns[0] != alignNone || aligns[1] != alignCenter {
		t.Errorf("parseTableDelim = %v, %v", aligns, ok)
	}
}

// TestSplitTableRowTrailingPipe covers the trailing-pipe trimming and the
// escaped trailing pipe (which must NOT be trimmed) branches.
func TestSplitTableRowTrailingPipe(t *testing.T) {
	// Escaped trailing pipe is a literal cell content, not a separator.
	cells := splitTableRow([]byte(`a \|`))
	if len(cells) != 1 || string(cells[0]) != "a |" {
		t.Errorf("splitTableRow escaped trailing pipe = %q", cells)
	}
	// A lone backslash before end of line stays literal.
	cells = splitTableRow([]byte(`a\`))
	if len(cells) != 1 || string(cells[0]) != `a\` {
		t.Errorf("splitTableRow trailing backslash = %q", cells)
	}
}

// TestTaskLists covers checked/unchecked/absent markers in tight and loose
// lists and the off-by-default behaviour.
func TestTaskLists(t *testing.T) {
	o := &Options{TaskList: true}
	eq(t, "- [ ] todo\n- [x] done\n- [X] also done\n- plain\n", o,
		"<ul>\n"+
			"<li><input type=\"checkbox\" disabled=\"\" /> todo</li>\n"+
			"<li><input type=\"checkbox\" checked=\"\" disabled=\"\" /> done</li>\n"+
			"<li><input type=\"checkbox\" checked=\"\" disabled=\"\" /> also done</li>\n"+
			"<li>plain</li>\n</ul>\n")

	// Loose list: checkbox lands inside the <p>.
	eq(t, "- [ ] a\n\n- [x] b\n", o,
		"<ul>\n"+
			"<li>\n<p><input type=\"checkbox\" disabled=\"\" /> a</p>\n</li>\n"+
			"<li>\n<p><input type=\"checkbox\" checked=\"\" disabled=\"\" /> b</p>\n</li>\n</ul>\n")

	// Not a task marker: bad char in brackets, or no following whitespace.
	eq(t, "- [y] no\n", o, "<ul>\n<li>[y] no</li>\n</ul>\n")
	eq(t, "- [ ]nospace\n", o, "<ul>\n<li>[ ]nospace</li>\n</ul>\n")
	// Marker only applies to the first paragraph of a list item, not to a bare
	// paragraph outside a list.
	eq(t, "[ ] loose\n", o, "<p>[ ] loose</p>\n")
	// Off by default.
	eq(t, "- [ ] todo\n", nil, "<ul>\n<li>[ ] todo</li>\n</ul>\n")
}

// TestStripTaskMarkerDirect exercises stripTaskMarker's guard clauses that are
// awkward to reach via full documents.
func TestStripTaskMarkerDirect(t *testing.T) {
	// A paragraph with no parent is returned unchanged.
	p := newNode(Paragraph)
	in := []byte("[ ] x")
	if got := stripTaskMarker(p, in); string(got) != "[ ] x" {
		t.Errorf("orphan paragraph changed: %q", got)
	}
	// Second paragraph of an item (not FirstChild) is left alone.
	item := newNode(Item)
	first := newNode(Paragraph)
	second := newNode(Paragraph)
	item.appendChild(first)
	item.appendChild(second)
	if got := stripTaskMarker(second, []byte("[x] y")); string(got) != "[x] y" {
		t.Errorf("non-first paragraph changed: %q", got)
	}
	// Content too short to hold a marker.
	if got := stripTaskMarker(first, []byte("[")); string(got) != "[" {
		t.Errorf("short content changed: %q", got)
	}
}
