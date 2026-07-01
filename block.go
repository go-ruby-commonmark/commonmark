// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import (
	"bytes"
	"strconv"
	"strings"
)

const codeIndent = 4

// parser holds the mutable state of a single parse.
type parser struct {
	opts   *Options
	doc    *Node
	tip    *Node // deepest open block
	oldtip *Node

	lineNumber int
	// current line (with the trailing newline stripped).
	line []byte

	offset               int // byte offset into line
	column               int // visual column (tabs expand to 4)
	nextNonspace         int
	nextNonspaceColumn   int
	indent               int
	indented             bool
	blank                bool
	partiallyConsumedTab bool

	allClosed            bool
	lastMatchedContainer *Node

	refmap map[string]*linkRef

	// inline parser (created lazily).
	inline *inlineParser
}

// linkRef is a resolved link reference definition.
type linkRef struct {
	destination []byte
	title       []byte
}

func newParser(opts *Options) *parser {
	doc := newNode(Document)
	p := &parser{
		opts:      opts,
		doc:       doc,
		tip:       doc,
		refmap:    map[string]*linkRef{},
		allClosed: true,
	}
	p.lastMatchedContainer = doc
	return p
}

// parse runs the two phases: block structure then inline content.
func (p *parser) parse(src string) *Node {
	// Preprocess: normalise line endings and strip a UTF-8 BOM/insecure NUL.
	s := preprocess(src)
	lines := splitLines(s)
	for _, ln := range lines {
		p.incorporateLine(ln)
	}
	for p.tip != nil {
		p.finalize(p.tip, len(lines))
	}
	p.processInlines(p.doc)
	return p.doc
}

// preprocess replaces NUL with U+FFFD (security) and normalises CRLF/CR to LF.
func preprocess(src string) string {
	if strings.IndexByte(src, 0) >= 0 {
		src = strings.ReplaceAll(src, "\x00", "�")
	}
	if strings.IndexByte(src, '\r') < 0 {
		return src
	}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	return src
}

// splitLines splits s into lines without their terminating newline. A trailing
// newline does NOT produce a final empty line; a source with no trailing newline
// still yields its last line.
func splitLines(s string) [][]byte {
	if s == "" {
		return [][]byte{[]byte("")}
	}
	var lines [][]byte
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, []byte(s[start:i]))
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, []byte(s[start:]))
	}
	return lines
}

// blockStartResult enumerates the outcome of a block-start matcher.
type blockStartResult int

const (
	noMatch blockStartResult = iota
	matchedContainer
	matchedLeaf
)

// incorporateLine processes one line of input, advancing the block structure.
func (p *parser) incorporateLine(line []byte) {
	p.line = line
	p.oldtip = p.tip
	p.offset = 0
	p.column = 0
	p.blank = false
	p.partiallyConsumedTab = false
	p.lineNumber++

	allMatched := true
	container := p.doc

	// Phase 1: descend through open containers, matching continuation lines.
	lastChild := container.LastChild
	for lastChild != nil && lastChild.open {
		container = lastChild
		p.findNextNonspace()

		switch p.continueBlock(container) {
		case 0: // matched
		case 1: // not matched, but keep going (shouldn't happen)
			allMatched = false
		case 2: // finished processing (e.g. code block consumed line)
			return
		}
		if !allMatched {
			container = container.Parent
			break
		}
		lastChild = container.LastChild
	}

	p.allClosed = container == p.oldtip
	p.lastMatchedContainer = container

	matchedLeafType := container.Type
	startsAllowed := matchedLeafType != CodeBlock && matchedLeafType != HTMLBlock

	// Phase 2: look for new block starts.
	for startsAllowed {
		p.findNextNonspace()

		if !p.indented && !isMaybeSpecial(p.peek()) {
			p.advanceNextNonspace()
			break
		}

		res, newContainer, isLeaf := p.tryBlockStarts(container)
		if res == noMatch {
			p.advanceNextNonspace()
			break
		}
		container = newContainer
		if isLeaf {
			startsAllowed = false
		}
	}

	// Phase 3: append remaining text to the current block.
	p.findNextNonspace()
	p.blank = p.nextNonspace >= len(p.line)

	if !p.allClosed && !p.blank && p.tip.Type == Paragraph {
		// Lazy continuation of a paragraph.
		p.addLine()
		return
	}

	p.closeUnmatchedBlocks()

	switch container.Type {
	case CodeBlock:
		p.addLine()
	case HTMLBlock:
		p.addLine()
		p.checkHTMLBlockEnd(container)
	default:
		if p.blank {
			container.lastLineBlank = true
		} else if acceptsLines(container.Type) {
			p.addLine()
		} else if container.Type != ThematicBreak && container.Type != Heading {
			// Create a paragraph to hold the text.
			container = p.addChild(Paragraph, p.offset)
			p.advanceNextNonspace()
			p.addLine()
		}
	}
	p.tip = container
}

// isMaybeSpecial reports whether c could begin a block start construct.
func isMaybeSpecial(c byte) bool {
	switch c {
	case '#', '`', '~', '*', '+', '-', '_', '>', '=', '<',
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 0:
		return true
	}
	return false
}

func (p *parser) peek() byte {
	if p.nextNonspace < len(p.line) {
		return p.line[p.nextNonspace]
	}
	return 0
}

// findNextNonspace scans from offset to the next non-space/tab, tracking columns.
func (p *parser) findNextNonspace() {
	i := p.offset
	cols := p.column
	var c byte
	for i < len(p.line) {
		c = p.line[i]
		if c == ' ' {
			i++
			cols++
		} else if c == '\t' {
			i++
			cols += 4 - (cols % 4)
		} else {
			break
		}
	}
	p.blank = i >= len(p.line) || c == '\n'
	p.nextNonspace = i
	p.nextNonspaceColumn = cols
	p.indent = p.nextNonspaceColumn - p.column
	p.indented = p.indent >= codeIndent
}

// advanceNextNonspace jumps offset/column to the next non-space position.
func (p *parser) advanceNextNonspace() {
	p.offset = p.nextNonspace
	p.column = p.nextNonspaceColumn
	p.partiallyConsumedTab = false
}

// advanceOffset moves forward by count characters (columns if cols is true),
// correctly splitting a tab stop.
func (p *parser) advanceOffset(count int, cols bool) {
	for count > 0 && p.offset < len(p.line) {
		c := p.line[p.offset]
		if c == '\t' {
			charsToTab := 4 - (p.column % 4)
			if cols {
				if charsToTab > count {
					p.partiallyConsumedTab = true
					p.column += count
					count = 0
				} else {
					p.partiallyConsumedTab = false
					p.column += charsToTab
					p.offset++
					count -= charsToTab
				}
			} else {
				p.partiallyConsumedTab = false
				p.column += charsToTab
				p.offset++
				count--
			}
		} else {
			p.partiallyConsumedTab = false
			p.offset++
			p.column++
			count--
		}
	}
}

// continueBlock checks whether an open block continues on the current line.
// Returns 0 = matched, 1 = not matched, 2 = fully handled (stop).
func (p *parser) continueBlock(container *Node) int {
	switch container.Type {
	case BlockQuote:
		if !p.indented && p.peek() == '>' {
			p.advanceNextNonspace()
			p.advanceOffset(1, false)
			if isSpaceOrTab(p.charAt(p.offset)) {
				p.advanceOffset(1, true)
			}
			return 0
		}
		return 1
	case Item:
		return p.continueItem(container)
	case CodeBlock:
		return p.continueCodeBlock(container)
	case HTMLBlock:
		if p.blank && (container.htmlBlockType == 6 || container.htmlBlockType == 7) {
			return 1
		}
		return 0
	case Heading, ThematicBreak:
		return 1
	case Paragraph:
		if p.blank {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func (p *parser) charAt(i int) byte {
	if i < len(p.line) {
		return p.line[i]
	}
	return 0
}

func (p *parser) continueItem(container *Node) int {
	ld := container.list
	if p.blank {
		if container.FirstChild == nil {
			// Blank line after an empty list item ends the item.
			return 1
		}
		p.advanceNextNonspace()
		return 0
	}
	if p.indent >= ld.markerOffset+ld.padding {
		p.advanceOffset(ld.markerOffset+ld.padding, true)
		return 0
	}
	return 1
}

func (p *parser) continueCodeBlock(container *Node) int {
	if container.fenced {
		// Check for a closing fence.
		if p.indent <= 3 && p.charAt(p.nextNonspace) == container.fenceChar {
			rest := p.line[p.nextNonspace:]
			if m := reClosingFence.Find(rest); m != nil {
				// Count the run length.
				n := 0
				for n < len(rest) && rest[n] == container.fenceChar {
					n++
				}
				if n >= container.fenceLength {
					p.finalize(container, p.lineNumber)
					return 2
				}
			}
		}
		// Continuation: consume up to fenceOffset spaces of indentation.
		i := 0
		for i < container.fenceOffset && isSpaceOrTab(p.charAt(p.offset)) {
			p.advanceOffset(1, true)
			i++
		}
		return 0
	}
	// Indented code block.
	if p.indent >= codeIndent {
		p.advanceOffset(codeIndent, true)
		return 0
	}
	if p.blank {
		p.advanceNextNonspace()
		return 0
	}
	return 1
}

// tryBlockStarts attempts each block-start matcher in priority order.
func (p *parser) tryBlockStarts(container *Node) (blockStartResult, *Node, bool) {
	starters := []func(*parser, *Node) (blockStartResult, *Node){
		startBlockQuote,
		startATXHeading,
		startFencedCode,
		startHTMLBlock,
		startSetextHeading,
		startThematicBreak,
		startListItem,
		startIndentedCode,
	}
	for _, fn := range starters {
		res, nc := fn(p, container)
		if res != noMatch {
			return res, nc, res == matchedLeaf
		}
	}
	return noMatch, container, false
}

// addChild closes any blocks that can't contain a new child of the given type,
// then appends a new open node.
func (p *parser) addChild(t NodeType, offset int) *Node {
	for !canContain(p.tip.Type, t) {
		p.finalize(p.tip, p.lineNumber-1)
	}
	child := newNode(t)
	child.content = nil
	child.sourceLine = p.lineNumber
	p.tip.appendChild(child)
	p.tip = child
	return child
}

// canContain reports whether a block of parent type may hold a child type.
func canContain(parent, child NodeType) bool {
	switch parent {
	case Document, BlockQuote, Item:
		return child != Item || parent == List
	case List:
		return child == Item
	default:
		return false
	}
}

// acceptsLines reports whether a leaf block accumulates raw text lines.
func acceptsLines(t NodeType) bool {
	return t == Paragraph || t == Heading || t == CodeBlock || t == HTMLBlock
}

// closeUnmatchedBlocks finalizes any open blocks the current line did not match.
func (p *parser) closeUnmatchedBlocks() {
	if p.allClosed {
		return
	}
	for p.oldtip != p.lastMatchedContainer {
		parent := p.oldtip.Parent
		p.finalize(p.oldtip, p.lineNumber-1)
		p.oldtip = parent
	}
	p.allClosed = true
}

// addLine appends the remaining content of the current line to tip.content.
func (p *parser) addLine() {
	if p.partiallyConsumedTab {
		p.offset++ // skip over the tab
		charsToTab := 4 - (p.column % 4)
		for i := 0; i < charsToTab; i++ {
			p.tip.content = append(p.tip.content, ' ')
		}
	}
	p.tip.content = append(p.tip.content, p.line[p.offset:]...)
	p.tip.content = append(p.tip.content, '\n')
}

// finalize closes a block, running any block-specific post-processing.
func (p *parser) finalize(block *Node, lineNumber int) {
	above := block.Parent
	block.open = false

	switch block.Type {
	case Paragraph:
		p.finalizeParagraph(block)
	case CodeBlock:
		p.finalizeCodeBlock(block)
	case HTMLBlock:
		block.Literal = block.content
		block.content = nil
	case List:
		p.finalizeList(block)
	}
	p.tip = above
}

func (p *parser) finalizeParagraph(block *Node) {
	content := block.content
	// Parse leading link reference definitions.
	for len(content) > 0 && content[0] == '[' {
		consumed := p.parseReferenceDef(content)
		if consumed <= 0 {
			break
		}
		content = content[consumed:]
	}
	block.content = content
	if isBlankAll(content) {
		block.unlink()
		return
	}
	if p.opts.Tables {
		if table := tryParseTable(content); table != nil {
			block.insertAfter(table)
			block.unlink()
		}
	}
}

func isBlankAll(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\n' {
			return false
		}
	}
	return true
}

func (p *parser) finalizeCodeBlock(block *Node) {
	if block.fenced {
		// The opening fence line contributes an empty remainder that addLine
		// records as a leading newline; drop that first line so the body starts
		// at the first genuine content line. The info string was captured at
		// start into block.Info. A fence with no body still carries that leading
		// newline, so IndexByte always finds it.
		content := block.content
		if nl := bytes.IndexByte(content, '\n'); nl >= 0 {
			content = content[nl+1:]
		}
		block.Literal = content
	} else {
		// Indented: strip trailing blank lines.
		content := block.content
		content = stripTrailingBlankLines(content)
		block.Literal = content
	}
	block.content = nil
}

func stripTrailingBlankLines(b []byte) []byte {
	lines := bytes.Split(b, []byte("\n"))
	// Last element is empty due to trailing newline; drop trailing blanks.
	end := len(lines)
	// The final split element after the last \n is "".
	if end > 0 && len(lines[end-1]) == 0 {
		end--
	}
	for end > 0 && isBlank(lines[end-1]) {
		end--
	}
	// Indented code always has at least one non-blank line, so end > 0 here.
	res := bytes.Join(lines[:end], []byte("\n"))
	res = append(res, '\n')
	return res
}

func (p *parser) finalizeList(block *Node) {
	// Determine tightness: a list is loose if any item is followed by a blank
	// line, or any item contains a blank between block children.
	tight := true
	item := block.FirstChild
	for item != nil {
		if endsWithBlankLine(item) && item.Next != nil {
			tight = false
			break
		}
		sub := item.FirstChild
		for sub != nil {
			if endsWithBlankLine(sub) && (item.Next != nil || sub.Next != nil) {
				tight = false
				break
			}
			sub = sub.Next
		}
		if !tight {
			break
		}
		item = item.Next
	}
	block.list.tight = tight
}

// endsWithBlankLine reports whether a block ends with a blank line, walking into
// its last child for list items.
func endsWithBlankLine(block *Node) bool {
	for block != nil {
		if block.lastLineBlank {
			return true
		}
		if block.Type == List || block.Type == Item {
			block = block.LastChild
		} else {
			return false
		}
	}
	return false
}

// processInlines walks the tree parsing inline content of every text container.
func (p *parser) processInlines(node *Node) {
	ip := p.getInlineParser()
	p.walkInlines(node, ip)
}

func (p *parser) walkInlines(node *Node, ip *inlineParser) {
	switch node.Type {
	case Paragraph, Heading:
		content := node.content
		if p.opts.TaskList && node.Type == Paragraph {
			content = stripTaskMarker(node, content)
		}
		ip.parse(node, trimSpaceTabNewline(content))
		node.content = nil
	case TableCell:
		ip.parse(node, trimSpaceTabNewline(node.content))
		node.content = nil
	}
	for c := node.FirstChild; c != nil; c = c.Next {
		p.walkInlines(c, ip)
	}
}

func trimSpaceTabNewline(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n') {
		i++
	}
	j := len(b)
	for j > i && (b[j-1] == ' ' || b[j-1] == '\t' || b[j-1] == '\n') {
		j--
	}
	return b[i:j]
}

// getInlineParser lazily constructs the inline parser bound to this parse.
func (p *parser) getInlineParser() *inlineParser {
	if p.inline == nil {
		p.inline = newInlineParser(p.opts, p.refmap)
	}
	return p.inline
}

// --- block-start matchers -------------------------------------------------

func startBlockQuote(p *parser, container *Node) (blockStartResult, *Node) {
	if !p.indented && p.peek() == '>' {
		p.advanceNextNonspace()
		p.advanceOffset(1, false)
		if isSpaceOrTab(p.charAt(p.offset)) {
			p.advanceOffset(1, true)
		}
		p.closeUnmatchedBlocks()
		p.addChild(BlockQuote, p.nextNonspace)
		return matchedContainer, p.tip
	}
	return noMatch, container
}

func startATXHeading(p *parser, container *Node) (blockStartResult, *Node) {
	if p.indented {
		return noMatch, container
	}
	rest := p.line[p.nextNonspace:]
	m := reATXHeading.Find(rest)
	if m == nil {
		return noMatch, container
	}
	p.advanceNextNonspace()
	p.advanceOffset(len(m), false)
	p.closeUnmatchedBlocks()
	level := 0
	for level < len(m) && m[level] == '#' {
		level++
	}
	node := p.addChild(Heading, p.nextNonspace)
	node.Level = level
	// Strip the remaining line: trailing spaces + optional closing sequence.
	content := p.line[p.offset:]
	content = trimSpaceTab(content)
	content = stripATXClosing(content)
	node.content = append([]byte{}, content...)
	p.advanceOffset(len(p.line)-p.offset, false)
	return matchedLeaf, node
}

// stripATXClosing removes a trailing run of # (optionally space-preceded).
func stripATXClosing(b []byte) []byte {
	b = trimRightSpaceTab(b)
	// Remove trailing #'s if preceded by space or at start.
	j := len(b)
	k := j
	for k > 0 && b[k-1] == '#' {
		k--
	}
	if k < j && (k == 0 || b[k-1] == ' ' || b[k-1] == '\t') {
		return trimRightSpaceTab(b[:k])
	}
	return b
}

func startFencedCode(p *parser, container *Node) (blockStartResult, *Node) {
	if p.indented {
		return noMatch, container
	}
	c := p.peek()
	if c != '`' && c != '~' {
		return noMatch, container
	}
	rest := p.line[p.nextNonspace:]
	// Count fence length.
	n := 0
	for n < len(rest) && rest[n] == c {
		n++
	}
	if n < 3 {
		return noMatch, container
	}
	info := rest[n:]
	// Backtick fences cannot contain a backtick in the info string.
	if c == '`' && bytes.IndexByte(info, '`') >= 0 {
		return noMatch, container
	}
	p.closeUnmatchedBlocks()
	node := p.addChild(CodeBlock, p.nextNonspace)
	node.fenced = true
	node.fenceChar = c
	node.fenceLength = n
	node.fenceOffset = p.indent
	node.Info = append([]byte{}, unescapeString(trimSpaceTab(info))...)
	p.advanceNextNonspace()
	// Consume the whole opening line: the fence run and its info string. The
	// body starts on the next line.
	p.advanceOffset(len(p.line)-p.offset, false)
	return matchedLeaf, node
}

func startHTMLBlock(p *parser, container *Node) (blockStartResult, *Node) {
	if p.indented {
		return noMatch, container
	}
	if p.peek() != '<' {
		return noMatch, container
	}
	rest := p.line[p.nextNonspace:]
	for t := 0; t < 6; t++ {
		if reHTMLBlockOpen[t].Match(rest) {
			// Type 7 (paragraph interruption) is not allowed to interrupt a
			// paragraph; types 1..6 may.
			p.closeUnmatchedBlocks()
			node := p.addChild(HTMLBlock, p.offset)
			node.htmlBlockType = t + 1
			return matchedLeaf, node
		}
	}
	if container.Type != Paragraph && reHTMLBlockOpen7.Match(rest) {
		p.closeUnmatchedBlocks()
		node := p.addChild(HTMLBlock, p.offset)
		node.htmlBlockType = 7
		return matchedLeaf, node
	}
	return noMatch, container
}

func (p *parser) checkHTMLBlockEnd(container *Node) {
	t := container.htmlBlockType
	if t >= 1 && t <= 5 {
		if reHTMLBlockClose[t-1].Match(p.line[p.offset:]) {
			p.finalize(container, p.lineNumber)
			p.tip = container.Parent
		}
	}
}

func startSetextHeading(p *parser, container *Node) (blockStartResult, *Node) {
	if p.indented || container.Type != Paragraph {
		return noMatch, container
	}
	rest := p.line[p.nextNonspace:]
	m := reSetextHeading.Find(rest)
	if m == nil {
		return noMatch, container
	}
	p.closeUnmatchedBlocks()
	// The container is a Paragraph with non-blank content (blank paragraphs are
	// never created), so convert it to a setext heading.
	heading := newNode(Heading)
	if rest[0] == '=' {
		heading.Level = 1
	} else {
		heading.Level = 2
	}
	heading.content = container.content
	container.insertAfter(heading)
	container.unlink()
	p.tip = heading
	p.advanceOffset(len(p.line)-p.offset, false)
	return matchedLeaf, heading
}

func startThematicBreak(p *parser, container *Node) (blockStartResult, *Node) {
	if p.indented {
		return noMatch, container
	}
	rest := p.line[p.nextNonspace:]
	if !reThematicBreak.Match(rest) {
		return noMatch, container
	}
	p.closeUnmatchedBlocks()
	p.addChild(ThematicBreak, p.nextNonspace)
	p.advanceOffset(len(p.line)-p.offset, false)
	return matchedLeaf, p.tip
}

func startIndentedCode(p *parser, container *Node) (blockStartResult, *Node) {
	if !p.indented || p.tip.Type == Paragraph || p.blank {
		return noMatch, container
	}
	p.advanceOffset(codeIndent, true)
	p.closeUnmatchedBlocks()
	node := p.addChild(CodeBlock, p.offset)
	node.fenced = false
	return matchedLeaf, node
}

func startListItem(p *parser, container *Node) (blockStartResult, *Node) {
	if p.indented && container.Type != List {
		return noMatch, container
	}
	ld, matched := p.parseListMarker(container)
	if !matched {
		return noMatch, container
	}
	p.closeUnmatchedBlocks()

	// If the tip is a paragraph and this is an ordered list not starting at 1,
	// or a list marker that would interrupt a paragraph with empty content, the
	// spec forbids interruption.
	if p.tip.Type != List || !listsMatch(p.tip.list, ld) {
		lst := p.addChild(List, p.nextNonspace)
		lst.list = ld
	}
	item := p.addChild(Item, p.nextNonspace)
	item.list = ld
	return matchedContainer, item
}

func listsMatch(a, b *listData) bool {
	return a.ordered == b.ordered && a.bulletChar == b.bulletChar && a.delim == b.delim
}

// parseListMarker recognises a bullet or ordered list marker and computes its
// padding. It returns a fresh listData describing the item and whether a marker
// was found.
func (p *parser) parseListMarker(container *Node) (*listData, bool) {
	rest := p.line[p.nextNonspace:]
	ld := &listData{markerOffset: p.indent}

	var markerLen int
	if m := reBulletMarker.Find(rest); m != nil {
		ld.bulletChar = m[0]
		ld.ordered = false
		markerLen = 1
	} else if m := reOrderedMarker.FindSubmatch(rest); m != nil {
		// Ordered lists interrupting a paragraph must start at 1.
		start, _ := strconv.Atoi(string(m[1]))
		if container.Type == Paragraph && start != 1 {
			return nil, false
		}
		ld.ordered = true
		ld.start = start
		ld.delim = m[2][0]
		markerLen = len(m[1]) + 1
	} else {
		return nil, false
	}

	// A marker must be followed by a space/tab or end-of-line, and if it
	// interrupts a paragraph the content after must be non-empty.
	afterMarker := rest[markerLen:]
	if len(afterMarker) > 0 && !isSpaceOrTab(afterMarker[0]) {
		return nil, false
	}
	if container.Type == Paragraph && isBlank(afterMarker) {
		return nil, false
	}

	// Compute padding: spaces after the marker up to the content (1..5). If the
	// content is blank or indented >= 5, padding is 1.
	// Advance past the marker to find the column of the content.
	p.advanceNextNonspace()
	p.advanceOffset(markerLen, false)
	spacesStartCol := p.column
	spacesStartOffset := p.offset
	for {
		if p.column-spacesStartCol >= 5 {
			break
		}
		if !isSpaceOrTab(p.charAt(p.offset)) {
			break
		}
		p.advanceOffset(1, true)
	}
	blankItem := p.charAt(p.offset) == 0 || p.offset >= len(p.line)
	spacesAfterMarker := p.column - spacesStartCol
	if spacesAfterMarker >= 5 || spacesAfterMarker < 1 || blankItem {
		ld.padding = markerLen + 1
		if spacesAfterMarker > 0 {
			// Reset to just past marker + one space.
			p.column = spacesStartCol
			p.offset = spacesStartOffset
			if isSpaceOrTab(p.charAt(p.offset)) {
				p.advanceOffset(1, true)
			}
		}
	} else {
		ld.padding = markerLen + spacesAfterMarker
	}
	return ld, true
}

// parseReferenceDef parses a leading link reference definition from content and
// returns the number of bytes consumed (0 if none). It records the definition in
// the parser's refmap.
func (p *parser) parseReferenceDef(content []byte) int {
	s := &subject{buf: content}
	// Label.
	label, ok := s.parseLinkLabel()
	if !ok || len(label) == 0 {
		return 0
	}
	if s.pos >= len(s.buf) || s.buf[s.pos] != ':' {
		return 0
	}
	s.pos++
	s.skipWhitespaceIncludingOneNewline()
	dest, ok := parseLinkDestination(s)
	if !ok {
		return 0
	}
	// Title (optional).
	beforeTitle := s.pos
	seenWS := s.skipWhitespaceIncludingOneNewline()
	title, hasTitle := parseLinkTitle(s)
	if !hasTitle {
		s.pos = beforeTitle
		title = nil
	}
	if hasTitle && !seenWS {
		// Title must be separated from destination by whitespace.
		s.pos = beforeTitle
		title = nil
		hasTitle = false
	}
	// The rest of the line must be blank.
	afterTitle := s.pos
	s.skipSpaceTab()
	if s.pos < len(s.buf) && s.buf[s.pos] != '\n' {
		if hasTitle {
			// Title made the line invalid — try again without a title.
			s.pos = beforeTitle
			s.skipSpaceTab()
			if s.pos < len(s.buf) && s.buf[s.pos] != '\n' {
				return 0
			}
		} else {
			return 0
		}
	}
	_ = afterTitle
	// Consume the trailing newline.
	if s.pos < len(s.buf) && s.buf[s.pos] == '\n' {
		s.pos++
	}
	norm := normalizeLabel(label)
	if norm == "" {
		return 0
	}
	if _, exists := p.refmap[norm]; !exists {
		p.refmap[norm] = &linkRef{destination: dest, title: title}
	}
	return s.pos
}
