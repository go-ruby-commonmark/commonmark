// Copyright (c) the go-ruby-commonmark/commonmark authors
//
// SPDX-License-Identifier: BSD-3-Clause

package commonmark

// NodeType enumerates the kinds of nodes in the parse tree. Block-level and
// inline-level nodes share one tree, following the CommonMark reference model.
type NodeType int

const (
	// Document is the root of every parse tree.
	Document NodeType = iota
	// BlockQuote is a `>`-prefixed block quote.
	BlockQuote
	// List is a bullet or ordered list container.
	List
	// Item is a single list item.
	Item
	// CodeBlock is an indented or fenced code block.
	CodeBlock
	// HTMLBlock is a raw block-level HTML region.
	HTMLBlock
	// Paragraph is a paragraph of inline content.
	Paragraph
	// Heading is an ATX or Setext heading.
	Heading
	// ThematicBreak is a horizontal rule (`---`, `***`, `___`).
	ThematicBreak
	// Text is a run of literal text.
	Text
	// Softbreak is an end-of-line that renders as a newline (or space).
	Softbreak
	// Linebreak is a hard line break (`  \n` or `\\\n`).
	Linebreak
	// Code is an inline code span.
	Code
	// HTMLInline is raw inline HTML.
	HTMLInline
	// Emphasis is `*`/`_` emphasis (rendered <em>).
	Emphasis
	// Strong is `**`/`__` strong emphasis (rendered <strong>).
	Strong
	// Link is a hyperlink.
	Link
	// Image is an image reference.
	Image
	// Strikethrough is GFM `~~` strikethrough (extension).
	Strikethrough
	// Table is a GFM table (extension).
	Table
	// TableRow is a row within a GFM table.
	TableRow
	// TableCell is a cell within a GFM table row.
	TableCell
)

// listData holds the bookkeeping for a List / Item pair.
type listData struct {
	bulletChar byte // '-', '+', '*' for bullets; 0 for ordered
	delim      byte // '.' or ')' for ordered lists
	start      int  // ordered list start number
	ordered    bool
	tight      bool
	// marker offset + padding used while matching continuation lines
	markerOffset int
	padding      int
}

// taskState is the GFM task-list checkbox state of a list item.
type taskState int

const (
	taskNone taskState = iota
	taskUnchecked
	taskChecked
)

// cellAlign is the alignment of a GFM table column.
type cellAlign int

const (
	alignNone cellAlign = iota
	alignLeft
	alignCenter
	alignRight
)

// Node is a single node in the CommonMark parse tree. It is exported so callers
// that need structured access (via Parse) can walk it; ToHTML uses it internally.
type Node struct {
	Type NodeType

	// Tree links.
	Parent                *Node
	FirstChild, LastChild *Node
	Prev, Next            *Node

	// Literal payload for leaf nodes: Text/Code/HTMLBlock/HTMLInline/CodeBlock.
	Literal []byte

	// Heading level (1..6) or, during Setext resolution, the marker level.
	Level int

	// CodeBlock: whether it is fenced and its info string.
	fenced      bool
	fenceChar   byte
	fenceLength int
	fenceOffset int
	Info        []byte // fenced code info string (raw)

	// Link / Image destination + title.
	Destination []byte
	Title       []byte

	// List / Item data.
	list *listData

	// Table cell alignment / header flag.
	align  cellAlign
	header bool

	// task is the GFM task-list state of a list Item: taskNone, taskUnchecked
	// or taskChecked. It is only set when Options.TaskList is enabled.
	task taskState

	// Parser bookkeeping (block phase).
	open            bool
	lastLineBlank   bool
	lastLineChecked bool
	sourceLine      int
	// content accumulates raw text for paragraphs / headings before inline parse.
	content []byte
	// string content already-stripped for html blocks etc.
	htmlBlockType int
	// stringContent is used for the raw text of code fences etc.
}

func newNode(t NodeType) *Node {
	return &Node{Type: t, open: true}
}

// appendChild attaches child as the last child of n.
func (n *Node) appendChild(child *Node) {
	child.unlink()
	child.Parent = n
	if n.LastChild != nil {
		n.LastChild.Next = child
		child.Prev = n.LastChild
		n.LastChild = child
	} else {
		n.FirstChild = child
		n.LastChild = child
	}
}

// insertAfter inserts sibling immediately after n.
func (n *Node) insertAfter(sibling *Node) {
	sibling.unlink()
	sibling.Next = n.Next
	if sibling.Next != nil {
		sibling.Next.Prev = sibling
	}
	sibling.Prev = n
	n.Next = sibling
	sibling.Parent = n.Parent
	if sibling.Parent != nil && sibling.Parent.LastChild == n {
		sibling.Parent.LastChild = sibling
	}
}

// insertBefore inserts sibling immediately before n.
func (n *Node) insertBefore(sibling *Node) {
	sibling.unlink()
	sibling.Prev = n.Prev
	if sibling.Prev != nil {
		sibling.Prev.Next = sibling
	}
	sibling.Next = n
	n.Prev = sibling
	sibling.Parent = n.Parent
	if sibling.Parent != nil && sibling.Parent.FirstChild == n {
		sibling.Parent.FirstChild = sibling
	}
}

// unlink detaches n from its parent and siblings.
func (n *Node) unlink() {
	if n.Prev != nil {
		n.Prev.Next = n.Next
	} else if n.Parent != nil {
		n.Parent.FirstChild = n.Next
	}
	if n.Next != nil {
		n.Next.Prev = n.Prev
	} else if n.Parent != nil {
		n.Parent.LastChild = n.Prev
	}
	n.Parent = nil
	n.Next = nil
	n.Prev = nil
}
