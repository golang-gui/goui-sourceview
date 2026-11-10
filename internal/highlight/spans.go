// Package highlight implements SourceView lexical and foreground indexes.
package highlight

import (
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

// Span is a validated foreground range, independent of document and view state.
type Span struct {
	Range textedit.Range
	Kind  language.Kind
}

// SpanBuilder constructs a persistent index from ordered, nonoverlapping spans.
// The caller validates ranges against its text snapshot before appending.
type SpanBuilder struct {
	root  *SpanNode
	stack []*SpanNode
	count int
}

func (b *SpanBuilder) Append(span Span) {
	// SplitMix64 supplies reproducible, well-distributed treap priorities.
	p := uint64(b.count+1) + 0x9e3779b97f4a7c15
	p = (p ^ (p >> 30)) * 0xbf58476d1ce4e5b9
	p = (p ^ (p >> 27)) * 0x94d049bb133111eb
	p ^= p >> 31
	n := &SpanNode{span: span, priority: p}
	for len(b.stack) > 0 && b.stack[len(b.stack)-1].priority < p {
		n.left = b.stack[len(b.stack)-1]
		b.stack = b.stack[:len(b.stack)-1]
	}
	if len(b.stack) == 0 {
		b.root = n
	} else {
		b.stack[len(b.stack)-1].right = n
	}
	b.stack = append(b.stack, n)

	b.count++
}

// Root transfers the completed immutable index; do not append after publication.
func (b *SpanBuilder) Root() *SpanNode { return b.root }

// Persistent treap: publication shares immutable nodes; edits clone only search
// paths, discard intersecting spans, and lazily translate a surviving suffix.
// No edit walks the complete semantic result, and queries never mutate a snapshot.
type SpanNode struct {
	span        Span
	left, right *SpanNode
	priority    uint64
	shift       int
}

func shiftHighlights(n *SpanNode, delta int) *SpanNode {
	if n == nil || delta == 0 {
		return n
	}
	c := *n
	c.span.Range.Start += delta
	c.span.Range.End += delta
	c.shift += delta
	return &c
}
func copyHighlight(n *SpanNode) *SpanNode {
	c := *n
	c.left = shiftHighlights(c.left, c.shift)
	c.right = shiftHighlights(c.right, c.shift)
	c.shift = 0
	return &c
}
func mergeHighlights(a, b *SpanNode) *SpanNode {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.priority > b.priority {
		c := copyHighlight(a)
		c.right = mergeHighlights(c.right, b)
		return c
	}
	c := copyHighlight(b)
	c.left = mergeHighlights(a, c.left)
	return c
}

// Split before spans whose start is >= at, or whose end is > at.
func splitHighlights(n *SpanNode, at int, byEnd bool) (*SpanNode, *SpanNode) {
	if n == nil {
		return nil, nil
	}
	c := copyHighlight(n)
	left := c.span.Range.Start < at
	if byEnd {
		left = c.span.Range.End <= at
	}
	if left {
		a, b := splitHighlights(c.right, at, byEnd)
		c.right = a
		return c, b
	}
	a, b := splitHighlights(c.left, at, byEnd)
	c.left = b
	return a, c
}

// MapSpans returns a new index, preserving nodes outside edited ranges.
func MapSpans(root *SpanNode, edits []textedit.Edit) *SpanNode {
	for _, e := range edits {
		left, rest := splitHighlights(root, e.Range.Start, true)
		_, right := splitHighlights(rest, e.Range.End, false)
		root = mergeHighlights(left, shiftHighlights(right, len(e.Text)-(e.Range.End-e.Range.Start)))
	}
	return root
}

// VisitSpans visits ordered spans clipped to r without mutating shared nodes.
func VisitSpans(root *SpanNode, r textedit.Range, visitSpan func(Span)) {
	if r.End <= r.Start {
		return
	}
	var visit func(*SpanNode, int)
	visit = func(n *SpanNode, delta int) {
		if n == nil {
			return
		}
		span := n.span
		span.Range.Start += delta
		span.Range.End += delta
		if span.Range.Start >= r.End {
			visit(n.left, delta+n.shift)
			return
		}
		if span.Range.End <= r.Start {
			visit(n.right, delta+n.shift)
			return
		}
		visit(n.left, delta+n.shift)
		span.Range.Start = max(span.Range.Start, r.Start)
		span.Range.End = min(span.Range.End, r.End)
		visitSpan(span)
		visit(n.right, delta+n.shift)
	}
	visit(root, 0)
}
