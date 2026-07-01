// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import "regexp"

// Regular expressions used across the block and inline parsers. They are
// compiled once at package init. Where the reference implementation uses a
// hand-written scanner these mirror its regexps.

var (
	reThematicBreak = regexp.MustCompile(`^(?:\*[ \t]*){3,}$|^(?:_[ \t]*){3,}$|^(?:-[ \t]*){3,}$`)

	reATXHeading = regexp.MustCompile(`^#{1,6}(?:[ \t]+|$)`)

	reCodeFence    = regexp.MustCompile("^`{3,}(?:[^`\n]*)$|^~{3,}[^\n]*$")
	reClosingFence = regexp.MustCompile("^(?:`{3,}|~{3,})[ \t]*$")

	reSetextHeading = regexp.MustCompile(`^(?:=+|-+)[ \t]*$`)

	// Bullet list marker: -, +, or *.
	reBulletMarker = regexp.MustCompile(`^[*+-]`)
	// Ordered list marker: 1-9 digits followed by . or ).
	reOrderedMarker = regexp.MustCompile(`^(\d{1,9})([.)])`)

	// HTML block start conditions (types 1..7). See CommonMark §4.6.
	reHTMLBlockOpen = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^<(?:script|pre|textarea|style)(?:[ \t>]|$)`),
		regexp.MustCompile(`^<!--`),
		regexp.MustCompile(`^<\?`),
		regexp.MustCompile(`^<![A-Za-z]`),
		regexp.MustCompile(`^<!\[CDATA\[`),
		regexp.MustCompile(`(?i)^</?(?:address|article|aside|base|basefont|blockquote|body|caption|center|col|colgroup|dd|details|dialog|dir|div|dl|dt|fieldset|figcaption|figure|footer|form|frame|frameset|h1|h2|h3|h4|h5|h6|head|header|hr|html|iframe|legend|li|link|main|menu|menuitem|nav|noframes|ol|optgroup|option|p|param|search|section|summary|table|tbody|td|tfoot|th|thead|title|tr|track|ul)(?:[ \t]|$|>|/>)`),
	}
	reHTMLBlockClose = []*regexp.Regexp{
		regexp.MustCompile(`(?i)</(?:script|pre|textarea|style)>`),
		regexp.MustCompile(`-->`),
		regexp.MustCompile(`\?>`),
		regexp.MustCompile(`>`),
		regexp.MustCompile(`\]\]>`),
	}
	// Type 7 opener: a complete open or closing tag on a line by itself.
	reHTMLBlockOpen7 = regexp.MustCompile(`^(?:` + htmlOpenTag + `|` + htmlCloseTag + `)[ \t]*$`)
)

// HTML tag component patterns shared by the block (type-7) and inline scanners.
const (
	htmlTagName     = `[A-Za-z][A-Za-z0-9-]*`
	htmlAttrName    = `[A-Za-z_:][A-Za-z0-9_.:-]*`
	htmlUnquoted    = `[^"'=<>` + "`" + `\x00-\x20]+`
	htmlSingle      = `'[^']*'`
	htmlDouble      = `"[^"]*"`
	htmlAttrValue   = `(?:` + htmlUnquoted + `|` + htmlSingle + `|` + htmlDouble + `)`
	htmlAttrValSpec = `[ \t\r\n]*=[ \t\r\n]*` + htmlAttrValue
	htmlAttr        = `[ \t\r\n]+` + htmlAttrName + `(?:` + htmlAttrValSpec + `)?`
	htmlOpenTag     = `<` + htmlTagName + `(?:` + htmlAttr + `)*[ \t\r\n]*/?>`
	htmlCloseTag    = `</` + htmlTagName + `[ \t\r\n]*>`
	htmlComment     = `<!-->|<!--->|<!--(?:[^-]|-[^-]|--[^>])*-->`
	htmlProcInst    = `<\?[\s\S]*?\?>`
	htmlDecl        = `<![A-Za-z][^>]*>`
	htmlCDATA       = `<!\[CDATA\[[\s\S]*?\]\]>`
)

// reHTMLTag matches any inline raw-HTML construct.
var reHTMLTag = regexp.MustCompile(
	`^(?:` + htmlOpenTag + `|` + htmlCloseTag + `|` + htmlComment + `|` +
		htmlProcInst + `|` + htmlDecl + `|` + htmlCDATA + `)`)

// reEmailAutolink matches the body of an email autolink (without < >).
var reEmailAutolink = regexp.MustCompile(
	`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// reURIAutolink matches the body of a URI autolink (scheme:rest, no spaces/<>).
var reURIAutolink = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]{1,31}:[^\x00-\x20<>]*$`)

// GFM extended autolink helpers (used only when Options.Autolink is set).
var (
	reExtWWW = regexp.MustCompile(`^www\.`)
	reDomain = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9_-]*[a-zA-Z0-9])?\.)+[a-zA-Z0-9](?:[a-zA-Z0-9_-]*[a-zA-Z0-9])?`)
)
