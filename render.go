// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import (
	"bytes"
	"strconv"
	"strings"
)

// htmlRenderer walks a parse tree emitting HTML.
type htmlRenderer struct {
	sb          strings.Builder
	opts        *Options
	lastOut     byte
	disableTags int // inside <a> for image alt text
}

// renderHTML renders a Document node tree to an HTML fragment string.
func renderHTML(doc *Node, opts *Options) string {
	if opts == nil {
		opts = &Options{}
	}
	r := &htmlRenderer{opts: opts, lastOut: '\n'}
	r.render(doc)
	return r.sb.String()
}

// cr emits a newline unless the previous output already ended with one.
func (r *htmlRenderer) cr() {
	if r.lastOut != '\n' {
		r.sb.WriteByte('\n')
		r.lastOut = '\n'
	}
}

func (r *htmlRenderer) out(s string) {
	if r.disableTags > 0 {
		// Strip tags: emit only the text between < and >.
		r.outStripTags(s)
	} else {
		r.sb.WriteString(s)
	}
	if len(s) > 0 {
		r.lastOut = s[len(s)-1]
	}
}

func (r *htmlRenderer) outStripTags(s string) {
	in := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			in = true
		case '>':
			in = false
		default:
			if !in {
				r.sb.WriteByte(s[i])
			}
		}
	}
}

func (r *htmlRenderer) outText(b []byte) {
	var tmp strings.Builder
	escapeHTML(&tmp, b)
	r.out(tmp.String())
}

func (r *htmlRenderer) render(node *Node) {
	switch node.Type {
	case Document:
		r.renderChildren(node)
	case Paragraph:
		r.renderParagraph(node)
	case Heading:
		r.renderHeading(node)
	case ThematicBreak:
		r.cr()
		r.out("<hr />")
		r.cr()
	case BlockQuote:
		r.cr()
		r.out("<blockquote>")
		r.cr()
		r.renderChildren(node)
		r.cr()
		r.out("</blockquote>")
		r.cr()
	case List:
		r.renderList(node)
	case Item:
		r.renderItem(node)
	case CodeBlock:
		r.renderCodeBlock(node)
	case HTMLBlock:
		r.renderHTMLBlock(node)
	case Text:
		r.outText(node.Literal)
	case Softbreak:
		// HardBreaks is applied at parse time (soft breaks become Linebreak
		// nodes), so a Softbreak node always renders as a plain newline.
		r.out("\n")
	case Linebreak:
		r.out("<br />")
		r.cr()
	case Code:
		r.out("<code>")
		r.outText(node.Literal)
		r.out("</code>")
	case HTMLInline:
		r.renderRawHTML(node.Literal)
	case Emphasis:
		r.out("<em>")
		r.renderChildren(node)
		r.out("</em>")
	case Strong:
		r.out("<strong>")
		r.renderChildren(node)
		r.out("</strong>")
	case Strikethrough:
		r.out("<del>")
		r.renderChildren(node)
		r.out("</del>")
	case Link:
		r.renderLink(node)
	case Image:
		r.renderImage(node)
	case Table:
		r.renderTable(node)
	case TableRow:
		r.renderTableRow(node)
	case TableCell:
		r.renderTableCell(node)
	}
}

func (r *htmlRenderer) renderChildren(node *Node) {
	for c := node.FirstChild; c != nil; c = c.Next {
		r.render(c)
	}
}

func (r *htmlRenderer) renderItem(node *Node) {
	r.cr()
	r.out("<li>")
	r.renderChildren(node)
	r.out("</li>")
	r.cr()
}

// taskCheckbox emits the GFM checkbox for the first paragraph of a task item.
func (r *htmlRenderer) taskCheckbox(para *Node) {
	item := para.Parent
	if item == nil || item.Type != Item || item.task == taskNone || item.FirstChild != para {
		return
	}
	if item.task == taskChecked {
		r.out("<input type=\"checkbox\" checked=\"\" disabled=\"\" /> ")
	} else {
		r.out("<input type=\"checkbox\" disabled=\"\" /> ")
	}
}

func (r *htmlRenderer) renderParagraph(node *Node) {
	// Suppress <p> wrappers inside a tight list.
	grandparent := node.Parent
	if grandparent != nil && grandparent.Type == Item {
		list := grandparent.Parent
		if list != nil && list.list != nil && list.list.tight {
			r.taskCheckbox(node)
			r.renderChildren(node)
			return
		}
	}
	r.cr()
	r.out("<p>")
	r.taskCheckbox(node)
	r.renderChildren(node)
	r.out("</p>")
	r.cr()
}

func (r *htmlRenderer) renderHeading(node *Node) {
	tag := "h" + strconv.Itoa(node.Level)
	r.cr()
	r.out("<" + tag + ">")
	r.renderChildren(node)
	r.out("</" + tag + ">")
	r.cr()
}

func (r *htmlRenderer) renderList(node *Node) {
	ld := node.list
	r.cr()
	if ld.ordered {
		if ld.start != 1 {
			r.out("<ol start=\"" + strconv.Itoa(ld.start) + "\">")
		} else {
			r.out("<ol>")
		}
	} else {
		r.out("<ul>")
	}
	r.cr()
	r.renderChildren(node)
	r.cr()
	if ld.ordered {
		r.out("</ol>")
	} else {
		r.out("</ul>")
	}
	r.cr()
}

func (r *htmlRenderer) renderCodeBlock(node *Node) {
	r.cr()
	info := trimSpaceTab(node.Info)
	var lang []byte
	if len(info) > 0 {
		if sp := bytes.IndexAny(info, " \t"); sp >= 0 {
			lang = info[:sp]
		} else {
			lang = info
		}
	}
	if len(lang) > 0 {
		var cls strings.Builder
		cls.WriteString("language-")
		escapeHTML(&cls, lang)
		if r.opts.GitHubPreLang {
			r.out("<pre lang=\"")
			r.outText(lang)
			r.out("\"><code>")
		} else {
			r.out("<pre><code class=\"" + cls.String() + "\">")
		}
	} else {
		r.out("<pre><code>")
	}
	r.outText(node.Literal)
	r.out("</code></pre>")
	r.cr()
}

func (r *htmlRenderer) renderHTMLBlock(node *Node) {
	r.cr()
	if r.opts.Unsafe {
		r.out(string(bytes.TrimRight(node.Literal, "\n")))
	} else {
		r.out("<!-- raw HTML omitted -->")
	}
	r.cr()
}

func (r *htmlRenderer) renderRawHTML(lit []byte) {
	if r.opts.Unsafe {
		r.out(string(lit))
	} else {
		r.out("<!-- raw HTML omitted -->")
	}
}

func (r *htmlRenderer) renderLink(node *Node) {
	if r.disableTags == 0 {
		var sb strings.Builder
		sb.WriteString("<a href=\"")
		r.writeURL(&sb, node.Destination)
		sb.WriteByte('"')
		if len(node.Title) > 0 {
			sb.WriteString(" title=\"")
			escapeHTML(&sb, node.Title)
			sb.WriteByte('"')
		}
		sb.WriteByte('>')
		r.out(sb.String())
	}
	r.renderChildren(node)
	if r.disableTags == 0 {
		r.out("</a>")
	}
}

func (r *htmlRenderer) renderImage(node *Node) {
	if r.disableTags == 0 {
		var sb strings.Builder
		sb.WriteString("<img src=\"")
		r.writeURL(&sb, node.Destination)
		sb.WriteString("\" alt=\"")
		r.out(sb.String())
	}
	r.disableTags++
	r.renderChildren(node)
	r.disableTags--
	if r.disableTags == 0 {
		var sb strings.Builder
		sb.WriteByte('"')
		if len(node.Title) > 0 {
			sb.WriteString(" title=\"")
			escapeHTML(&sb, node.Title)
			sb.WriteByte('"')
		}
		sb.WriteString(" />")
		r.out(sb.String())
	}
}

// writeURL writes a normalised, escaped link destination, applying the unsafe
// scheme filter when Options.Unsafe is false.
func (r *htmlRenderer) writeURL(sb *strings.Builder, dest []byte) {
	d := normalizeURI(dest)
	if !r.opts.Unsafe && isUnsafeURL(d) {
		return
	}
	urlEncode(sb, d)
}

// isUnsafeURL reports whether a destination uses a potentially dangerous scheme
// (javascript:, vbscript:, file:, data: except a safe image whitelist).
func isUnsafeURL(dest []byte) bool {
	s := bytes.TrimLeft(dest, " \t\n")
	lower := bytes.ToLower(s)
	prefixes := [][]byte{
		[]byte("javascript:"),
		[]byte("vbscript:"),
		[]byte("file:"),
	}
	for _, p := range prefixes {
		if bytes.HasPrefix(lower, p) {
			return true
		}
	}
	if bytes.HasPrefix(lower, []byte("data:")) {
		safe := [][]byte{
			[]byte("data:image/png"),
			[]byte("data:image/gif"),
			[]byte("data:image/jpeg"),
			[]byte("data:image/webp"),
		}
		for _, ok := range safe {
			if bytes.HasPrefix(lower, ok) {
				return false
			}
		}
		return true
	}
	return false
}
