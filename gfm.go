// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import (
	"bytes"
	"strings"
)

// GFM extensions layered on top of the core CommonMark parser. Each is guarded
// by an Options flag so strict CommonMark behaviour is preserved by default.
//
// Included behind Options: Strikethrough (~~text~~), Autolink (bare URL / www),
// Tables (pipe tables). TaskList checkboxes are rendered from list items.
//
// Deferred / not implemented: GFM's HTML tag filter (raw-tag disallowing) beyond
// the CommonMark unsafe filter, and footnotes. These are documented in README.

// appendTextWithAutolinks splits a text run into literal text and GFM extended
// autolinks (bare http(s):// URLs and www. links). It is only called when
// Options.Autolink is enabled.
func (s *subject) appendTextWithAutolinks(text []byte) {
	i := 0
	start := 0
	for i < len(text) {
		if url, urlLen, dest := matchExtAutolink(text, i); urlLen > 0 {
			if i > start {
				s.appendText(text[start:i])
			}
			link := newNode(Link)
			link.Destination = dest
			t := newNode(Text)
			t.Literal = append([]byte{}, url...)
			link.appendChild(t)
			s.block.appendChild(link)
			i += urlLen
			start = i
			continue
		}
		i++
	}
	if start < len(text) {
		s.appendText(text[start:])
	}
}

// matchExtAutolink tries to match a GFM extended autolink starting at or after
// position i. It requires a word boundary before the match. It returns the
// matched display text, its length in text bytes, and the destination.
func matchExtAutolink(text []byte, i int) (disp []byte, length int, dest []byte) {
	// Boundary: start of run or preceded by whitespace/(*_~ punctuation.
	if i > 0 {
		prev := text[i-1]
		if isAlnum(prev) || prev == '/' {
			return nil, 0, nil
		}
	}
	rest := text[i:]
	var scheme int
	switch {
	case bytes.HasPrefix(rest, []byte("https://")):
		scheme = len("https://")
	case bytes.HasPrefix(rest, []byte("http://")):
		scheme = len("http://")
	case reExtWWW.Match(rest):
		scheme = 0 // www. handled via domain match
	default:
		return nil, 0, nil
	}

	var domainEnd int
	if scheme > 0 {
		m := reDomain.Find(rest[scheme:])
		if m == nil {
			return nil, 0, nil
		}
		domainEnd = scheme + len(m)
	} else {
		m := reDomain.Find(rest)
		if m == nil {
			return nil, 0, nil
		}
		domainEnd = len(m)
	}

	// Extend over path characters until whitespace or <.
	end := domainEnd
	for end < len(rest) && !isWhitespaceChar(rest[end]) && rest[end] != '<' {
		end++
	}
	// Trim trailing punctuation per GFM rules.
	end = trimAutolinkTail(rest, end)
	if end <= domainEnd && scheme == 0 {
		// www with no path is fine; ensure a dot exists (reDomain guarantees).
	}
	disp = rest[:end]
	if scheme == 0 {
		dest = append([]byte("http://"), disp...)
	} else {
		dest = append([]byte{}, disp...)
	}
	return disp, end, dest
}

// trimAutolinkTail applies GFM trailing-punctuation trimming to an autolink.
func trimAutolinkTail(rest []byte, end int) int {
	for end > 0 {
		c := rest[end-1]
		if strings.IndexByte("?!.,:*_~'\"", c) >= 0 {
			end--
			continue
		}
		if c == ')' {
			// Balance parens: count ( and ) in [0:end).
			opens := bytes.Count(rest[:end], []byte("("))
			closes := bytes.Count(rest[:end], []byte(")"))
			if closes > opens {
				end--
				continue
			}
		}
		break
	}
	return end
}

// --- GFM task lists -------------------------------------------------------

// stripTaskMarker inspects a paragraph's raw content. If the paragraph is the
// first child of a list item and begins with a GFM task-list marker ("[ ]",
// "[x]" or "[X]") followed by whitespace, it records the checkbox state on the
// item and returns the content with the marker (and its trailing space)
// removed. Otherwise it returns content unchanged.
func stripTaskMarker(para *Node, content []byte) []byte {
	item := para.Parent
	if item == nil || item.Type != Item || item.FirstChild != para {
		return content
	}
	c := content
	// Skip leading spaces/tabs before the marker.
	i := 0
	for i < len(c) && (c[i] == ' ' || c[i] == '\t') {
		i++
	}
	if i+3 > len(c) || c[i] != '[' || c[i+2] != ']' {
		return content
	}
	var state taskState
	switch c[i+1] {
	case ' ':
		state = taskUnchecked
	case 'x', 'X':
		state = taskChecked
	default:
		return content
	}
	// The marker must be followed by whitespace (space, tab or newline).
	after := i + 3
	if after >= len(c) || (c[after] != ' ' && c[after] != '\t' && c[after] != '\n') {
		return content
	}
	item.task = state
	// Remove the marker but keep a single following space so the text stays
	// separated (GFM renders "[x] done" as checkbox + " done").
	rest := c[after:]
	out := make([]byte, 0, len(c))
	out = append(out, c[:i]...)
	out = append(out, rest...)
	return out
}

// --- GFM tables -----------------------------------------------------------

// tryParseTable attempts to interpret a paragraph node's content as a GFM table.
// It is called from the block finaliser when Options.Tables is enabled. It
// returns the Table node (already populated) or nil when the content is not a
// table.
func tryParseTable(content []byte) *Node {
	lines := splitNonEmptyLines(content)
	if len(lines) < 2 {
		return nil
	}
	header := lines[0]
	delim := lines[1]
	aligns, ok := parseTableDelim(delim)
	if !ok {
		return nil
	}
	headerCells := splitTableRow(header)
	if len(headerCells) != len(aligns) {
		return nil
	}

	table := newNode(Table)
	table.align = alignNone

	// Header row.
	head := newNode(TableRow)
	head.header = true
	for i, cell := range headerCells {
		c := newNode(TableCell)
		c.header = true
		c.align = aligns[i]
		c.content = trimSpaceTab(cell)
		head.appendChild(c)
	}
	table.appendChild(head)
	table.list = &listData{start: len(aligns)} // stash column count

	// Body rows.
	for _, line := range lines[2:] {
		cells := splitTableRow(line)
		row := newNode(TableRow)
		for i := 0; i < len(aligns); i++ {
			c := newNode(TableCell)
			c.align = aligns[i]
			if i < len(cells) {
				c.content = trimSpaceTab(cells[i])
			}
			row.appendChild(c)
		}
		table.appendChild(row)
	}
	return table
}

func splitNonEmptyLines(content []byte) [][]byte {
	raw := bytes.Split(content, []byte("\n"))
	var out [][]byte
	for _, l := range raw {
		if len(trimSpaceTab(l)) == 0 {
			continue
		}
		out = append(out, l)
	}
	return out
}

// parseTableDelim parses the delimiter row and returns per-column alignment.
func parseTableDelim(line []byte) ([]cellAlign, bool) {
	// splitTableRow always yields at least one cell.
	cells := splitTableRow(line)
	aligns := make([]cellAlign, 0, len(cells))
	for _, cell := range cells {
		c := trimSpaceTab(cell)
		if len(c) == 0 {
			return nil, false
		}
		left := c[0] == ':'
		right := c[len(c)-1] == ':'
		body := c
		if left {
			body = body[1:]
		}
		if right && len(body) > 0 {
			body = body[:len(body)-1]
		}
		if len(body) == 0 {
			return nil, false
		}
		for _, b := range body {
			if b != '-' {
				return nil, false
			}
		}
		switch {
		case left && right:
			aligns = append(aligns, alignCenter)
		case left:
			aligns = append(aligns, alignLeft)
		case right:
			aligns = append(aligns, alignRight)
		default:
			aligns = append(aligns, alignNone)
		}
	}
	return aligns, true
}

// splitTableRow splits a table row on unescaped pipes, trimming an optional
// leading/trailing pipe.
func splitTableRow(line []byte) [][]byte {
	l := trimSpaceTab(line)
	if len(l) > 0 && l[0] == '|' {
		l = l[1:]
	}
	// Trim a trailing unescaped pipe.
	if n := len(l); n > 0 && l[n-1] == '|' && (n < 2 || l[n-2] != '\\') {
		l = l[:n-1]
	}
	var cells [][]byte
	var cur []byte
	for i := 0; i < len(l); i++ {
		c := l[i]
		if c == '\\' && i+1 < len(l) {
			if l[i+1] == '|' {
				cur = append(cur, '|')
				i++
				continue
			}
			cur = append(cur, c)
			continue
		}
		if c == '|' {
			cells = append(cells, cur)
			cur = nil
			continue
		}
		cur = append(cur, c)
	}
	cells = append(cells, cur)
	return cells
}

// --- table rendering ------------------------------------------------------

func (r *htmlRenderer) renderTable(node *Node) {
	r.cr()
	r.out("<table>")
	r.cr()
	child := node.FirstChild
	if child != nil {
		r.out("<thead>")
		r.cr()
		r.render(child)
		r.out("</thead>")
		r.cr()
		child = child.Next
	}
	if child != nil {
		r.out("<tbody>")
		r.cr()
		for child != nil {
			r.render(child)
			child = child.Next
		}
		r.out("</tbody>")
		r.cr()
	}
	r.out("</table>")
	r.cr()
}

func (r *htmlRenderer) renderTableRow(node *Node) {
	r.out("<tr>")
	r.cr()
	r.renderChildren(node)
	r.out("</tr>")
	r.cr()
}

func (r *htmlRenderer) renderTableCell(node *Node) {
	tag := "td"
	if node.header {
		tag = "th"
	}
	var sb strings.Builder
	sb.WriteByte('<')
	sb.WriteString(tag)
	switch node.align {
	case alignLeft:
		sb.WriteString(" align=\"left\"")
	case alignCenter:
		sb.WriteString(" align=\"center\"")
	case alignRight:
		sb.WriteString(" align=\"right\"")
	}
	sb.WriteByte('>')
	r.out(sb.String())
	r.renderChildren(node)
	r.out("</" + tag + ">")
	r.cr()
}
