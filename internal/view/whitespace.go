// Package view contains view-local indexes and native presentation resources.
// It never imports the root editor package or owns document/selection state.
package view

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

type WhitespaceMode uint8

const (
	WhitespaceNone WhitespaceMode = iota
	WhitespaceSelection
	WhitespaceTrailing
	WhitespaceAll
	WhitespaceBoundary
)

type WhitespaceLayout struct {
	Version, Revision         uint64
	Text                      string
	TextStart, Length         int
	Read                      func(int, bool) string
	lines                     []typography.TextLine
	runs                      [][]typography.TextCluster
	run, at                   int
	nodes                     []whitespaceNode
	roots                     []int
	Leading, Trailing         int
	LeadingDone, TrailingDone bool
	End                       geometry.Rectangle
	endStart                  int
	EndReady                  bool
	EndRTL                    bool
}

type whitespaceNode struct {
	bounds                  geometry.Rectangle
	left, right             int
	run, first, end, leaves int
}

const whitespaceClusterBlock = 256

func NewWhitespaceLayout(text string, lines []typography.TextLine, clusters []typography.TextCluster) *WhitespaceLayout {
	w := &WhitespaceLayout{Text: text, Length: len(text), lines: lines, Trailing: len(text), endStart: -1}
	if len(clusters) != 0 {
		w.runs = [][]typography.TextCluster{clusters}
	} else {
		for _, line := range lines {
			if len(line.Clusters) != 0 {
				w.runs = append(w.runs, line.Clusters)
			}
		}
	}
	if text == "" && len(lines) != 0 {
		line := lines[len(lines)-1]
		w.End, w.EndReady = geometry.Rect(line.X, line.Y, 0, line.Height), true
	}
	return w
}

func WhitespaceKind(r rune) int {
	switch r {
	case ' ':
		return 1
	case '\t':
		return 2
	case '\u00a0', '\u202f':
		return 3
	case '\u3000':
		return 4
	}
	return 0
}

func (w *WhitespaceLayout) Pending(boundaries bool) bool {
	return w.run < len(w.runs) || boundaries && (!w.LeadingDone || !w.TrailingDone)
}

func WhitespaceIntersects(a, b geometry.Rectangle) bool {
	return a.Width > 0 && a.Height > 0 && a.X < b.X+b.Width && b.X < a.X+a.Width && a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
}

func whitespaceUnion(a, b geometry.Rectangle) geometry.Rectangle {
	if a.Width <= 0 || a.Height <= 0 {
		return b
	}
	if b.Width <= 0 || b.Height <= 0 {
		return a
	}
	x, y := min(a.X, b.X), min(a.Y, b.Y)
	return geometry.Rect(x, y, max(a.X+a.Width, b.X+b.Width)-x, max(a.Y+a.Height, b.Y+b.Height)-y)
}

func (w *WhitespaceLayout) cell(c typography.TextCluster) (geometry.Rectangle, bool) {
	if c.LineIndex < 0 || c.LineIndex >= len(w.lines) || c.Start < 0 || c.Length <= 0 || c.Start > len(w.Text)-c.Length {
		return geometry.Rectangle{}, false
	}
	line := w.lines[c.LineIndex]
	return geometry.Rect(c.X, line.Y, c.Width, line.Height), true
}

// A pass has both a wall-clock deadline and a finite input budget, even when
// the caller supplies no deadline. Tests may repeatedly advance an index.
func (w *WhitespaceLayout) Advance(deadline time.Time, boundaries bool) {
	for work := 0; work < 8192 && w.Pending(boundaries); work += whitespaceClusterBlock {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return
		}
		if w.run < len(w.runs) {
			run := w.runs[w.run]
			end := min(len(run), w.at+whitespaceClusterBlock)
			node := whitespaceNode{left: -1, right: -1, run: w.run, first: w.at, end: end, leaves: 1}
			for _, c := range run[w.at:end] {
				if rect, ok := w.cell(c); ok {
					node.bounds = whitespaceUnion(node.bounds, rect)
					if c.Start+c.Length == len(w.Text) && c.Start > w.endStart {
						x := rect.X + rect.Width
						if c.Direction == typography.TextRightToLeft {
							x = rect.X
						}
						w.End, w.endStart, w.EndReady = geometry.Rect(x, rect.Y, 0, rect.Height), c.Start, true
						w.EndRTL = c.Direction == typography.TextRightToLeft
					}
				}
			}
			index := len(w.nodes)
			w.nodes = append(w.nodes, node)
			for len(w.roots) > 0 {
				left := w.roots[len(w.roots)-1]
				if w.nodes[left].leaves != w.nodes[index].leaves {
					break
				}
				w.roots = w.roots[:len(w.roots)-1]
				parent := whitespaceNode{bounds: whitespaceUnion(w.nodes[left].bounds, w.nodes[index].bounds), left: left, right: index, leaves: 2 * w.nodes[index].leaves}
				index = len(w.nodes)
				w.nodes = append(w.nodes, parent)
			}
			w.roots = append(w.roots, index)
			w.at = end
			if end == len(run) {
				w.run++
				w.at = 0
			}
		}
		if boundaries {
			w.scanBoundaries()
		}

	}
}

// Querying a ready tree only visits intersecting cluster blocks, including for
// a single unwrapped line and logically disjoint bidirectional text spans.
func (w *WhitespaceLayout) Visit(clip geometry.Rectangle, fn func(typography.TextCluster, geometry.Rectangle)) int {
	visited := 0
	var walk func(int)
	walk = func(index int) {
		node := w.nodes[index]
		if !WhitespaceIntersects(node.bounds, clip) {
			return
		}
		if node.left >= 0 {
			walk(node.left)
			walk(node.right)
			return
		}
		for _, c := range w.runs[node.run][node.first:node.end] {
			visited++
			if rect, ok := w.cell(c); ok && WhitespaceIntersects(rect, clip) {
				fn(c, rect)
			}
		}
	}
	for _, root := range w.roots {
		walk(root)
	}
	return visited
}

// read, when present, supplies a bounded valid-UTF-8 chunk beginning at at
// (forward), or ending at at (backward), in complete logical-line offsets.
// This keeps fragment edges distinct from logical indentation and trailing text.
func (w *WhitespaceLayout) chunk(at int, backward bool) string {
	if w.Read != nil {
		return w.Read(at, backward)
	}
	a, b := at, min(len(w.Text), at+512)
	if backward {
		a, b = max(0, at-512), at
		for a < b && !utf8.RuneStart(w.Text[a]) {
			a++
		}
	} else {
		for b < len(w.Text) && b > a && !utf8.RuneStart(w.Text[b]) {
			b--
		}
	}
	return w.Text[a:b]
}

func (w *WhitespaceLayout) scanBoundaries() {
	if !w.LeadingDone {
		text := w.chunk(w.Leading, false)
		for n := 0; n < 128 && text != ""; n++ {
			r, size := utf8.DecodeRuneInString(text)
			if WhitespaceKind(r) == 0 {
				w.LeadingDone = true
				break
			}
			w.Leading += size
			text = text[size:]
		}
		w.LeadingDone = w.LeadingDone || w.Leading == w.Length
	}
	if !w.TrailingDone {
		text := w.chunk(w.Trailing, true)
		for n := 0; n < 128 && text != ""; n++ {
			r, size := utf8.DecodeLastRuneInString(text)
			if WhitespaceKind(r) == 0 {
				w.TrailingDone = true
				break
			}
			w.Trailing -= size
			text = text[:len(text)-size]
		}
		w.TrailingDone = w.TrailingDone || w.Trailing == 0
	}
}

func (w *WhitespaceLayout) Show(mode WhitespaceMode, start, end, kind int, selected bool) bool {
	switch mode {
	case WhitespaceAll:
		return true
	case WhitespaceSelection:
		return selected
	case WhitespaceTrailing:
		return w.TextStart+start >= w.Trailing
	case WhitespaceBoundary:
		if kind != 1 || w.TextStart+start < w.Leading || w.TextStart+start >= w.Trailing || start > 0 && w.Text[start-1] == ' ' || end < len(w.Text) && w.Text[end] == ' ' {
			return true
		}
		if start == 0 && w.TextStart > 0 {
			text := w.chunk(w.TextStart, true)
			if strings.HasSuffix(text, " ") {
				return true
			}
		}
		if end == len(w.Text) && w.TextStart+end < w.Length {
			return strings.HasPrefix(w.chunk(w.TextStart+end, false), " ")
		}
		return false
	}
	return false
}
