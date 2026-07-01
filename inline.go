// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

// inlineParser parses the inline content of a text block into child nodes.
type inlineParser struct {
	opts   *Options
	refmap map[string]*linkRef
}

func newInlineParser(opts *Options, refmap map[string]*linkRef) *inlineParser {
	return &inlineParser{opts: opts, refmap: refmap}
}

// subject holds the state of an in-progress inline scan over one block's text.
type subject struct {
	buf   []byte
	pos   int
	block *Node // the container whose children we append to

	delimiters *delimiter // top of the delimiter stack
	brackets   *bracket   // top of the bracket stack

	ip *inlineParser
}

// delimiter is a node on the emphasis/link delimiter stack.
type delimiter struct {
	prev, next *delimiter
	node       *Node // the Text node holding the run
	char       byte  // '*', '_', '~'
	numdelims  int
	origdelims int
	canOpen    bool
	canClose   bool
}

// bracket tracks a `[` or `![` on the bracket stack for link/image parsing.
type bracket struct {
	prev         *bracket
	node         *Node // the Text node holding the [ or ![
	pos          int   // position after the bracket in buf
	image        bool
	active       bool
	bracketAfter bool
	prevDelim    *delimiter // delimiter stack top when the bracket was pushed
}

// parse fills block with inline child nodes parsed from content.
func (ip *inlineParser) parse(block *Node, content []byte) {
	s := &subject{buf: content, block: block, ip: ip}
	for s.parseInline() {
	}
	s.processEmphasis(nil)
}

func (s *subject) peek() byte {
	if s.pos < len(s.buf) {
		return s.buf[s.pos]
	}
	return 0
}

// appendText appends a text node with the given literal to the block.
func (s *subject) appendText(lit []byte) *Node {
	n := newNode(Text)
	n.Literal = append([]byte{}, lit...)
	s.block.appendChild(n)
	return n
}

// parseInline dispatches on the character at pos, appending one inline node.
// Returns false at end of input.
func (s *subject) parseInline() bool {
	if s.pos >= len(s.buf) {
		return false
	}
	c := s.buf[s.pos]
	switch c {
	case '\n':
		s.parseNewline()
	case '\\':
		s.parseBackslash()
	case '`':
		s.parseBacktick()
	case '*', '_':
		s.parseDelimRun(c)
	case '~':
		if s.ip.opts.Strikethrough {
			s.parseDelimRun(c)
		} else {
			s.parseString()
		}
	case '[':
		s.parseOpenBracket()
	case '!':
		s.parseBang()
	case ']':
		s.parseCloseBracket()
	case '<':
		s.parseAutolinkOrHTML()
	case '&':
		s.parseEntity()
	default:
		s.parseString()
	}
	return true
}

// parseString consumes a run of ordinary text up to the next special char.
func (s *subject) parseString() {
	start := s.pos
	for s.pos < len(s.buf) {
		if isInlineSpecial(s.buf[s.pos], s.ip.opts) {
			break
		}
		s.pos++
	}
	text := s.buf[start:s.pos]
	if s.ip.opts.Autolink {
		s.appendTextWithAutolinks(text)
	} else {
		s.appendText(text)
	}
}

func isInlineSpecial(c byte, opts *Options) bool {
	switch c {
	case '\n', '\\', '`', '*', '_', '[', ']', '!', '<', '&':
		return true
	case '~':
		return opts.Strikethrough
	}
	return false
}

// parseNewline handles a line ending: hard break (two+ spaces or backslash) or
// soft break.
func (s *subject) parseNewline() {
	s.pos++ // skip \n
	last := s.block.LastChild
	hard := false
	if last != nil && last.Type == Text {
		lit := last.Literal
		if len(lit) >= 2 && lit[len(lit)-1] == ' ' && lit[len(lit)-2] == ' ' {
			hard = true
		}
		// Trim trailing spaces from the preceding text.
		last.Literal = trimRightSpaceTab(lit)
	}
	var br *Node
	if hard || s.ip.opts.HardBreaks {
		br = newNode(Linebreak)
	} else {
		br = newNode(Softbreak)
	}
	s.block.appendChild(br)
	// Skip leading spaces of the next line.
	for s.pos < len(s.buf) && s.buf[s.pos] == ' ' {
		s.pos++
	}
}

// parseBackslash handles an escape sequence or a hard-break backslash-newline.
func (s *subject) parseBackslash() {
	s.pos++ // skip backslash
	if s.pos < len(s.buf) && s.buf[s.pos] == '\n' {
		s.pos++
		br := newNode(Linebreak)
		s.block.appendChild(br)
		for s.pos < len(s.buf) && s.buf[s.pos] == ' ' {
			s.pos++
		}
		return
	}
	if s.pos < len(s.buf) && isASCIIPunct(s.buf[s.pos]) {
		s.appendText(s.buf[s.pos : s.pos+1])
		s.pos++
		return
	}
	s.appendText([]byte{'\\'})
}

// parseBacktick handles an inline code span.
func (s *subject) parseBacktick() {
	start := s.pos
	for s.pos < len(s.buf) && s.buf[s.pos] == '`' {
		s.pos++
	}
	openLen := s.pos - start
	afterOpen := s.pos
	// Find a matching run of exactly openLen backticks.
	for s.pos < len(s.buf) {
		if s.buf[s.pos] == '`' {
			runStart := s.pos
			for s.pos < len(s.buf) && s.buf[s.pos] == '`' {
				s.pos++
			}
			if s.pos-runStart == openLen {
				// Found the closing run.
				code := s.buf[afterOpen:runStart]
				n := newNode(Code)
				n.Literal = normalizeCodeSpan(code)
				s.block.appendChild(n)
				return
			}
		} else {
			s.pos++
		}
	}
	// No closing run: emit the opening backticks as literal text.
	s.pos = afterOpen
	s.appendText(s.buf[start:afterOpen])
}

// normalizeCodeSpan applies the code-span content normalisation: line endings to
// spaces (already done at block level) and single leading+trailing space strip
// when the content is not all spaces.
func normalizeCodeSpan(code []byte) []byte {
	// Convert internal newlines to spaces.
	out := make([]byte, len(code))
	copy(out, code)
	for i := range out {
		if out[i] == '\n' {
			out[i] = ' '
		}
	}
	if len(out) >= 2 && out[0] == ' ' && out[len(out)-1] == ' ' && !allSpaces(out) {
		out = out[1 : len(out)-1]
	}
	return out
}

func allSpaces(b []byte) bool {
	for _, c := range b {
		if c != ' ' {
			return false
		}
	}
	return true
}

// parseEntity handles an entity or numeric character reference.
func (s *subject) parseEntity() {
	if repl, length := matchEntity(s.buf[s.pos:]); length > 0 {
		s.appendText([]byte(repl))
		s.pos += length
		return
	}
	s.appendText([]byte{'&'})
	s.pos++
}

// matchEntity attempts to match an entity/character reference at the start of b.
// It returns the replacement string and the number of source bytes consumed.
func matchEntity(b []byte) (string, int) {
	if len(b) < 3 || b[0] != '&' {
		return "", 0
	}
	if b[1] == '#' {
		// Numeric reference.
		if len(b) >= 4 && (b[2] == 'x' || b[2] == 'X') {
			i := 3
			for i < len(b) && isHexDigit(b[i]) && i-3 < 6 {
				i++
			}
			if i > 3 && i < len(b) && b[i] == ';' {
				cp, _ := strconv.ParseInt(string(b[3:i]), 16, 32)
				return codepointToString(rune(cp)), i + 1
			}
			return "", 0
		}
		i := 2
		for i < len(b) && isDigit(b[i]) && i-2 < 7 {
			i++
		}
		if i > 2 && i < len(b) && b[i] == ';' {
			cp, _ := strconv.ParseInt(string(b[2:i]), 10, 32)
			return codepointToString(rune(cp)), i + 1
		}
		return "", 0
	}
	// Named reference: & NAME ;
	i := 1
	for i < len(b) && isAlnum(b[i]) && i-1 < 32 {
		i++
	}
	if i > 1 && i < len(b) && b[i] == ';' {
		name := string(b[1:i])
		if repl, ok := htmlEntities[name]; ok {
			return repl, i + 1
		}
	}
	return "", 0
}

// codepointToString maps a numeric character reference to its UTF-8 string,
// replacing NUL and invalid code points with U+FFFD per the spec.
func codepointToString(cp rune) string {
	if cp == 0 || cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF) {
		return "�"
	}
	return string(cp)
}

// parseAutolinkOrHTML handles `<` — an autolink, raw HTML tag, or literal.
func (s *subject) parseAutolinkOrHTML() {
	rest := s.buf[s.pos:]
	// Autolink: <scheme:...> or <email>.
	if end := bytes.IndexByte(rest, '>'); end > 0 {
		inner := rest[1:end]
		if reURIAutolink.Match(inner) {
			s.appendAutolink(inner, false)
			s.pos += end + 1
			return
		}
		if reEmailAutolink.Match(inner) {
			s.appendAutolink(inner, true)
			s.pos += end + 1
			return
		}
	}
	// Raw HTML.
	if m := reHTMLTag.Find(rest); m != nil {
		n := newNode(HTMLInline)
		n.Literal = append([]byte{}, m...)
		s.block.appendChild(n)
		s.pos += len(m)
		return
	}
	s.appendText([]byte{'<'})
	s.pos++
}

func (s *subject) appendAutolink(inner []byte, email bool) {
	link := newNode(Link)
	var dest []byte
	if email {
		dest = append([]byte("mailto:"), inner...)
	} else {
		dest = append([]byte{}, inner...)
	}
	link.Destination = dest
	text := newNode(Text)
	text.Literal = append([]byte{}, inner...)
	link.appendChild(text)
	s.block.appendChild(link)
}

// --- delimiter run (emphasis / strikethrough) -----------------------------

func (s *subject) parseDelimRun(c byte) {
	numdelims, canOpen, canClose := s.scanDelims(c)
	start := s.pos
	s.pos += numdelims
	node := s.appendText(s.buf[start:s.pos])

	d := &delimiter{
		node:       node,
		char:       c,
		numdelims:  numdelims,
		origdelims: numdelims,
		canOpen:    canOpen,
		canClose:   canClose,
		prev:       s.delimiters,
	}
	if s.delimiters != nil {
		s.delimiters.next = d
	}
	s.delimiters = d
}

// scanDelims counts a run of the delimiter char at pos and computes whether the
// run can open and/or close emphasis, per the flanking rules.
func (s *subject) scanDelims(c byte) (numdelims int, canOpen, canClose bool) {
	startPos := s.pos
	// Only *, _ and ~ reach this path; count the run length.
	for startPos+numdelims < len(s.buf) && s.buf[startPos+numdelims] == c {
		numdelims++
	}

	beforeChar := prevRune(s.buf, startPos)
	afterChar := nextRune(s.buf, startPos+numdelims)

	beforeIsWhitespace := beforeChar == 0 || isUnicodeWhitespace(beforeChar)
	beforeIsPunct := beforeChar != 0 && isPunct(beforeChar)
	afterIsWhitespace := afterChar == 0 || isUnicodeWhitespace(afterChar)
	afterIsPunct := afterChar != 0 && isPunct(afterChar)

	leftFlanking := !afterIsWhitespace &&
		(!afterIsPunct || beforeIsWhitespace || beforeIsPunct)
	rightFlanking := !beforeIsWhitespace &&
		(!beforeIsPunct || afterIsWhitespace || afterIsPunct)

	if c == '_' {
		canOpen = leftFlanking && (!rightFlanking || beforeIsPunct)
		canClose = rightFlanking && (!leftFlanking || afterIsPunct)
	} else if c == '~' {
		canOpen = leftFlanking
		canClose = rightFlanking
	} else {
		canOpen = leftFlanking
		canClose = rightFlanking
	}
	return numdelims, canOpen, canClose
}

func prevRune(b []byte, pos int) rune {
	if pos == 0 {
		return 0
	}
	r, _ := utf8.DecodeLastRune(b[:pos])
	return r
}

func nextRune(b []byte, pos int) rune {
	if pos >= len(b) {
		return 0
	}
	r, _ := utf8.DecodeRune(b[pos:])
	return r
}

// processEmphasis converts matched delimiter runs into Emphasis/Strong nodes,
// scanning from stackBottom upward.
func (s *subject) processEmphasis(stackBottom *delimiter) {
	var openersBottom [3]map[byte]*delimiter
	for i := range openersBottom {
		openersBottom[i] = map[byte]*delimiter{}
	}
	// Initialise openers-bottom to stackBottom for each (char, len%3) combo.
	setBottom := func(m map[byte]*delimiter, c byte) {
		m[c] = stackBottom
	}
	for i := range openersBottom {
		for _, c := range []byte{'*', '_', '~'} {
			setBottom(openersBottom[i], c)
		}
	}

	// Find first potential closer above stackBottom.
	var closer *delimiter
	if stackBottom == nil {
		closer = s.bottomDelimiter()
	} else {
		closer = stackBottom.next
	}

	for closer != nil {
		if !closer.canClose {
			closer = closer.next
			continue
		}
		// Look back for a matching opener.
		opener := closer.prev
		openerFound := false
		for opener != nil && opener != stackBottom && opener != openersBottom[closer.numdelims%3][closer.char] {
			oddMatch := (closer.canOpen || opener.canClose) &&
				closer.origdelims%3 != 0 &&
				(opener.origdelims+closer.origdelims)%3 == 0
			if opener.char == closer.char && opener.canOpen && !oddMatch {
				openerFound = true
				break
			}
			opener = opener.prev
		}
		oldCloser := closer

		if !openerFound {
			// Record where we can stop looking for openers for this char/len.
			openersBottom[closer.numdelims%3][closer.char] = closer.prev
			if !closer.canOpen {
				s.removeDelimiter(closer)
			}
			closer = oldCloser.next
			continue
		}

		s.combineEmphasis(opener, closer)
		closer = closer.next
	}

	// Remove all delimiters above stackBottom.
	if stackBottom == nil {
		for s.delimiters != nil {
			s.removeDelimiter(s.delimiters)
		}
	} else {
		for s.delimiters != nil && s.delimiters != stackBottom {
			s.removeDelimiter(s.delimiters)
		}
	}
}

func (s *subject) bottomDelimiter() *delimiter {
	d := s.delimiters
	if d == nil {
		return nil
	}
	for d.prev != nil {
		d = d.prev
	}
	return d
}

// combineEmphasis wraps the content between opener and closer in an Emphasis or
// Strong node, removing consumed delimiters.
func (s *subject) combineEmphasis(opener, closer *delimiter) {
	var useDelims int
	if closer.char == '~' {
		// Strikethrough is single-delimiter.
		useDelims = 1
		if closer.numdelims >= 2 && opener.numdelims >= 2 {
			useDelims = 2
		}
	} else if closer.numdelims >= 2 && opener.numdelims >= 2 {
		useDelims = 2
	} else {
		useDelims = 1
	}

	openerNode := opener.node
	closerNode := closer.node

	// Trim the delimiter runs.
	openerNode.Literal = openerNode.Literal[:len(openerNode.Literal)-useDelims]
	closerNode.Literal = closerNode.Literal[useDelims:]
	opener.numdelims -= useDelims
	closer.numdelims -= useDelims

	var emph *Node
	if closer.char == '~' {
		emph = newNode(Strikethrough)
	} else if useDelims == 1 {
		emph = newNode(Emphasis)
	} else {
		emph = newNode(Strong)
	}

	// Move nodes between opener and closer into emph.
	tmp := openerNode.Next
	for tmp != nil && tmp != closerNode {
		next := tmp.Next
		emph.appendChild(tmp)
		tmp = next
	}
	openerNode.insertAfter(emph)

	// Remove delimiters between opener and closer.
	s.removeDelimitersBetween(opener, closer)

	// If either delimiter is exhausted, remove it.
	if opener.numdelims == 0 {
		openerNode.unlink()
		s.removeDelimiter(opener)
	}
	if closer.numdelims == 0 {
		closerNode.unlink()
		s.removeDelimiter(closer)
	}
}

func (s *subject) removeDelimitersBetween(bottom, top *delimiter) {
	if bottom.next != top {
		bottom.next = top
		top.prev = bottom
	}
}

func (s *subject) removeDelimiter(d *delimiter) {
	if d.prev != nil {
		d.prev.next = d.next
	}
	if d.next == nil {
		s.delimiters = d.prev
	} else {
		d.next.prev = d.prev
	}
}

// --- bracket / link+image parsing -----------------------------------------

func (s *subject) parseOpenBracket() {
	startPos := s.pos
	s.pos++
	node := s.appendText([]byte{'['})
	s.addBracket(node, startPos+1, false)
}

func (s *subject) parseBang() {
	startPos := s.pos
	s.pos++
	if s.peek() == '[' {
		s.pos++
		node := s.appendText([]byte("!["))
		s.addBracket(node, startPos+2, true)
	} else {
		s.appendText([]byte{'!'})
	}
}

func (s *subject) addBracket(node *Node, pos int, image bool) {
	if s.brackets != nil {
		s.brackets.bracketAfter = true
	}
	s.brackets = &bracket{
		prev:      s.brackets,
		node:      node,
		pos:       pos,
		image:     image,
		active:    true,
		prevDelim: s.delimiters,
	}
}

func (s *subject) parseCloseBracket() {
	s.pos++ // skip ]
	opener := s.brackets
	if opener == nil {
		s.appendText([]byte{']'})
		return
	}
	if !opener.active {
		s.removeBracket()
		s.appendText([]byte{']'})
		return
	}

	isImage := opener.image
	// Try an inline link/image: [text](dest "title")
	savePos := s.pos
	var dest, title []byte
	matched := false

	if s.peek() == '(' {
		s.pos++
		s.skipInlineWhitespace()
		if d, ok := parseLinkDestination(s); ok {
			beforeTitle := s.pos
			sawWS := s.skipInlineWhitespaceReport()
			t, hasT := parseLinkTitle(s)
			if !hasT || !sawWS {
				s.pos = beforeTitle
				t = nil
			}
			s.skipInlineWhitespace()
			if s.peek() == ')' {
				s.pos++
				dest = d
				title = t
				matched = true
			}
		}
	}

	if !matched {
		s.pos = savePos
		// Reference link.
		var label string
		var ok bool
		label, ok, s.pos = s.parseReferenceLabel(opener.pos)
		if ok {
			if ref, found := s.ip.refmap[label]; found {
				dest = ref.destination
				title = ref.title
				matched = true
			}
		}
	}

	if !matched {
		s.removeBracket()
		s.pos = savePos
		s.appendText([]byte{']'})
		return
	}

	// Build the link/image node.
	var node *Node
	if isImage {
		node = newNode(Image)
	} else {
		node = newNode(Link)
	}
	node.Destination = dest
	node.Title = title

	// Move inline nodes after the opening bracket into the node.
	tmp := opener.node.Next
	for tmp != nil {
		next := tmp.Next
		node.appendChild(tmp)
		tmp = next
	}
	opener.node.insertBefore(node)
	opener.node.unlink()

	// Process emphasis inside the link text.
	s.processEmphasis(opener.prevDelim)
	s.removeBracket()

	// Links cannot contain other links: deactivate earlier openers.
	if !isImage {
		b := s.brackets
		for b != nil {
			if !b.image {
				b.active = false
			}
			b = b.prev
		}
	}
}

func (s *subject) removeBracket() {
	if s.brackets != nil {
		s.brackets = s.brackets.prev
	}
}

// parseReferenceLabel parses the reference portion after `]`, handling full,
// collapsed, and shortcut references. It returns the normalised label, whether a
// usable label was formed, and the new position.
func (s *subject) parseReferenceLabel(textStart int) (string, bool, int) {
	// After the `]`, we may have `[label]`, `[]`, or nothing (shortcut).
	shortcutLabel := normalizeLabel(s.buf[textStart : s.pos-1])
	if s.peek() == '[' {
		saved := s.pos
		lab, ok := s.parseBracketLabel()
		if ok {
			if len(strings.TrimSpace(lab)) == 0 {
				// Collapsed reference [text][].
				return shortcutLabel, shortcutLabel != "", s.pos
			}
			return normalizeLabel([]byte(lab)), true, s.pos
		}
		s.pos = saved
		return "", false, s.pos
	}
	// Shortcut reference [text].
	if shortcutLabel == "" {
		return "", false, s.pos
	}
	return shortcutLabel, true, s.pos
}

// parseBracketLabel parses a `[...]` at pos (the caller guarantees a leading
// '[') and returns its raw inner text. A nested unescaped '[' is invalid.
func (s *subject) parseBracketLabel() (string, bool) {
	start := s.pos + 1
	i := start
	for i < len(s.buf) {
		c := s.buf[i]
		if c == '\\' && i+1 < len(s.buf) {
			i += 2
			continue
		}
		if c == ']' {
			s.pos = i + 1
			if i-start > 999 {
				return "", false
			}
			return string(s.buf[start:i]), true
		}
		if c == '[' {
			return "", false
		}
		i++
	}
	return "", false
}

// --- whitespace helpers on subject ----------------------------------------

func (s *subject) skipInlineWhitespace() {
	for s.pos < len(s.buf) && isWhitespaceChar(s.buf[s.pos]) {
		s.pos++
	}
}

func (s *subject) skipInlineWhitespaceReport() bool {
	start := s.pos
	s.skipInlineWhitespace()
	return s.pos > start
}

func (s *subject) skipSpaceTab() {
	for s.pos < len(s.buf) && isSpaceOrTab(s.buf[s.pos]) {
		s.pos++
	}
}

// skipWhitespaceIncludingOneNewline skips spaces/tabs and at most one newline,
// used by link reference definition parsing. Returns whether any ws was skipped.
func (s *subject) skipWhitespaceIncludingOneNewline() bool {
	start := s.pos
	seenNewline := false
	for s.pos < len(s.buf) {
		c := s.buf[s.pos]
		if c == '\n' {
			if seenNewline {
				break
			}
			seenNewline = true
			s.pos++
		} else if isSpaceOrTab(c) {
			s.pos++
		} else {
			break
		}
	}
	return s.pos > start
}

// parseLinkLabel parses a `[label]` and returns the raw label bytes.
func (s *subject) parseLinkLabel() ([]byte, bool) {
	if s.peek() != '[' {
		return nil, false
	}
	start := s.pos + 1
	i := start
	for i < len(s.buf) {
		c := s.buf[i]
		if c == '\\' && i+1 < len(s.buf) {
			i += 2
			continue
		}
		if c == ']' {
			if i-start > 999 {
				return nil, false
			}
			s.pos = i + 1
			return s.buf[start:i], true
		}
		if c == '[' {
			return nil, false
		}
		i++
	}
	return nil, false
}
