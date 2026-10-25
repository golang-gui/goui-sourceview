package sourceview

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
)

// WhitespaceMode controls decorative markers without changing source text.
type WhitespaceMode uint8

const (
	WhitespaceNone      WhitespaceMode = iota // Hide whitespace markers.
	WhitespaceSelection                       // Mark whitespace contained in the selection.
	WhitespaceTrailing                        // Mark whitespace at the end of a logical line.
	WhitespaceAll                             // Mark all supported whitespace.
	WhitespaceBoundary                        // Mark indentation, trailing/repeated spaces, Tabs and special spaces.
)

func (v *SourceView) WhitespaceMode() WhitespaceMode { return v.whitespace }

func (v *SourceView) SetWhitespaceMode(mode WhitespaceMode) error {
	if mode > WhitespaceBoundary {
		return fmt.Errorf("sourceview: invalid whitespace mode %d", mode)
	}
	if !v.Destroyed() && v.whitespace != mode {
		v.whitespace = mode
		v.RequestPaint()
	}
	return nil
}

func (v *SourceView) ShowLineEndings() bool { return v.showLineEndings }

func (v *SourceView) SetShowLineEndings(b bool) {
	if !v.Destroyed() && v.showLineEndings != b {
		v.showLineEndings = b
		v.RequestPaint()
	}
}

func (v *SourceView) ShowIndentGuides() bool { return v.indentGuides }

func (v *SourceView) SetShowIndentGuides(b bool) {
	if !v.Destroyed() && v.indentGuides != b {
		v.indentGuides = b
		v.RequestPaint()
	}
}

func (v *SourceView) SmartHomeEnd() bool { return v.smartHomeEnd }

func (v *SourceView) SetSmartHomeEnd(b bool) {
	if !v.Destroyed() {
		v.smartHomeEnd = b
	}
}

func (v *SourceView) SmartBackspace() bool { return v.smartBackspace }

func (v *SourceView) SetSmartBackspace(b bool) {
	if !v.Destroyed() {
		v.smartBackspace = b
	}
}

func (v *SourceView) HighlightOccurrences() bool { return v.highlightOccurrences }

func (v *SourceView) SetHighlightOccurrences(b bool) {
	if !v.Destroyed() && v.highlightOccurrences != b {
		v.highlightOccurrences = b
		v.RequestPaint()
	}
}

func (v *SourceView) Rulers() []int { return slices.Clone(v.rulers) }

func (v *SourceView) SetRulers(columns []int) error {
	columns = slices.Clone(columns)
	for _, col := range columns {
		if col < 1 || col > 1<<20 {
			return fmt.Errorf("sourceview: invalid ruler column %d", col)
		}
	}
	slices.Sort(columns)
	columns = slices.Compact(columns)
	if !v.Destroyed() && !slices.Equal(columns, v.rulers) {
		v.rulers = columns
		v.RequestPaint()
	}
	return nil
}

func leadingColumns(text string, tabWidth int) (bytes, columns int) {
	for bytes < len(text) {
		switch text[bytes] {
		case ' ':
			columns++
		case '\t':
			columns += tabWidth - columns%tabWidth
		default:
			return
		}
		bytes++
	}
	return
}

func (v *SourceView) deleteIndent() (bool, error) {
	if !v.smartBackspace || v.projection != nil {
		return false, nil
	}
	s := v.Selection()
	if s.Anchor != s.Caret {
		return false, nil
	}
	r, _ := v.document.model.LineRange(v.document.model.LineAt(s.Caret))
	before := editor.LeadingIndent(v.document.model, s.Caret)
	if len(before) != s.Caret-r.Start {
		return false, nil
	}
	n, col := leadingColumns(before, v.indent.TabWidth)
	if n != len(before) || col == 0 {
		return false, nil
	}
	target := (col - 1) / v.indent.IndentWidth * v.indent.IndentWidth
	at, current := 0, 0
	for at < len(before) {
		next := current + 1
		if before[at] == '\t' {
			next = current + v.indent.TabWidth - current%v.indent.TabWidth
		}
		if next > target {
			break
		}
		current = next
		at++
	}
	insert := strings.Repeat(" ", target-current)
	end := r.Start + at + len(insert)
	return true, v.applyPlan(editor.Plan{Before: s, After: textedit.Selection{Anchor: end, Caret: end}, Edits: []textedit.Edit{{Range: textedit.Range{Start: r.Start + at, End: s.Caret}, Text: insert}}})
}

func (v *SourceView) smartEdge(p *paragraph, pos textedit.Position, key events.Key) textedit.Position {
	if p.paged != nil && v.wrap == WrapNone {
		r := v.displayRange(p.row)
		edge := textedit.Position{}
		if key == events.KeyEnd {
			edge.Offset = r.End - r.Start
			edge.Upstream = true
		}
		if !v.smartHomeEnd {
			return edge
		}
		if key == events.KeyHome {
			at := len(editor.LeadingIndent(v.document.model, r.Start+pos.Offset))
			if at != pos.Offset {
				return textedit.Position{Offset: at}
			}
		} else {
			// Only the trailing whitespace is relevant. Stop at the first
			// non-space byte instead of materializing the entire logical line.
			at := r.End
			for at > r.Start {
				a := max(r.Start, at-4096)
				for !v.document.model.IsRuneBoundary(a) {
					a++
				}
				text, _ := v.document.model.Slice(textedit.Range{Start: a, End: at})
				n := len(text) - len(strings.TrimRight(text, " \t"))
				at -= n
				if n < len(text) {
					break
				}
			}
			if at-r.Start != pos.Offset {
				return textedit.Position{Offset: at - r.Start, Upstream: true}
			}
		}
		return edge
	}
	g := p.geometryAt(pos.Offset)
	index := g.LineIndex(pos)
	edge := g.LineEdge(index, key == events.KeyEnd)
	if !v.smartHomeEnd {
		return edge
	}
	line := g.LineMetrics(index)
	a, b := max(0, line.Start-p.textStart), min(len(p.text), line.Start+line.Length-p.textStart)
	if a > b {
		return edge
	}
	text := p.text[a:b]
	trimmed := strings.Trim(text, " \t")
	if trimmed == "" {
		return edge
	}
	at := a + len(text) - len(strings.TrimLeft(text, " \t"))
	if key == events.KeyEnd {
		at = a + len(strings.TrimRight(text, " \t"))
	}
	if at+p.textStart == pos.Offset {
		return edge
	}
	return textedit.Position{Offset: at + p.textStart, Upstream: key == events.KeyEnd}
}

func (v *SourceView) occurrenceWord() string {
	if !v.highlightOccurrences || v.projection != nil {
		return ""
	}
	r := v.Selection().Range()
	if r.End == r.Start || r.End-r.Start > 4096 {
		return ""
	}
	text, err := v.document.model.Slice(r)
	if err != nil || strings.ContainsRune(text, '\n') {
		return ""
	}
	word := v.definitionAt(v.Selection().Caret).IsWord
	for _, c := range text {
		if !word(c) {
			return ""
		}
	}
	m := v.document.model
	if r.Start > 0 {
		at := max(0, r.Start-utf8.UTFMax)
		for !m.IsRuneBoundary(at) {
			at++
		}
		previous, _ := m.Slice(textedit.Range{Start: at, End: r.Start})
		c, _ := utf8.DecodeLastRuneInString(previous)
		if word(c) {
			return ""
		}
	}
	if r.End < m.Len() {
		at := min(m.Len(), r.End+utf8.UTFMax)
		for !m.IsRuneBoundary(at) {
			at--
		}
		after, _ := m.Slice(textedit.Range{Start: r.End, End: at})
		c, _ := utf8.DecodeRuneInString(after)
		if word(c) {
			return ""
		}
	}
	return text
}

func tint(c color.NRGBA, alpha uint8) color.NRGBA { c.A = alpha; return c }

func (v *SourceView) paintAssistanceBackground(draw gui.Painter, row int, p *paragraph, origin geometry.Point, word string) {
	ruler := graphics.ColorOf(v.partColor("ruler", tint(v.colors.GutterForeground, 45)))
	lineY := v.geometryOrigin(row, 0, 0).Y
	top, bottom := max(0, lineY), min(v.viewport.Height, lineY+p.height)
	for _, col := range v.rulers {
		if bottom <= top {
			break
		}
		x := v.gutter + padding + float32(float64(col)*float64(v.spaceWidth)-float64(v.offset.X))
		draw.DrawLine(geometry.Point{X: x, Y: top}, geometry.Point{X: x, Y: bottom}, 1, ruler)
	}
	if v.indentGuides && p.textStart == 0 {
		bytes, columns := leadingColumns(p.text, v.indent.TabWidth)
		guide := graphics.ColorOf(v.partColor("indent-guide", tint(v.colors.GutterForeground, 65)))
		col, target := 0, v.indent.IndentWidth
		for at := 0; at < bytes && target < columns; at++ {
			next := col + 1
			if p.text[at] == '\t' {
				next = col + v.indent.TabWidth - col%v.indent.TabWidth
			}
			for target <= next && target < columns {
				start := p.geometry.Caret(textedit.Position{Offset: at})
				end := p.geometry.Caret(textedit.Position{Offset: at + 1, Upstream: true})
				if start.Y == end.Y {
					x := origin.X + start.X + (end.X-start.X)*float32(target-col)/float32(next-col)
					draw.DrawLine(geometry.Point{X: x, Y: origin.Y + start.Y}, geometry.Point{X: x, Y: origin.Y + start.Y + start.Height}, 1, guide)
				}
				target += v.indent.IndentWidth
			}
			col = next
		}
	}
	if word == "" || v.projection != nil {
		return
	}
	line := v.displayRange(row)
	for at := 0; at <= len(p.text)-len(word); {
		n := strings.Index(p.text[at:], word)
		if n < 0 {
			break
		}
		start := at + n
		end := start + len(word)
		at = end
		isWord := v.definitionAt(v.Selection().Caret).IsWord
		if start > 0 {
			c, _ := utf8.DecodeLastRuneInString(p.text[:start])
			if isWord(c) {
				continue
			}
		}
		if end < len(p.text) {
			c, _ := utf8.DecodeRuneInString(p.text[end:])
			if isWord(c) {
				continue
			}
		}
		r := textedit.Range{Start: line.Start + p.textStart + start, End: line.Start + p.textStart + end}
		if r == v.Selection().Range() {
			continue
		}
		v.paintRange(draw, p, line, origin, r, v.partColor("occurrence", tint(v.colors.SearchBackground, 55)))
	}
}

// Whitespace geometry borrows the unchanged layout's measured values. The
// paragraph owns both lifetimes; no extra native layout or per-marker geometry
// query is needed. A small bounding tree indexes groups of measured clusters.
type whitespaceLayout struct {
	version, revision         uint64
	text                      string
	textStart, length         int
	read                      func(int, bool) string
	lines                     []typography.TextLine
	runs                      [][]typography.TextCluster
	run, at                   int
	nodes                     []whitespaceNode
	roots                     []int
	leading, trailing         int
	leadingDone, trailingDone bool
	end                       geometry.Rectangle
	endStart                  int
	endReady                  bool
	endRTL                    bool
}

type whitespaceNode struct {
	bounds                  geometry.Rectangle
	left, right             int
	run, first, end, leaves int
}

const whitespaceClusterBlock = 256

func newWhitespaceLayout(text string, lines []typography.TextLine, clusters []typography.TextCluster) *whitespaceLayout {
	w := &whitespaceLayout{text: text, length: len(text), lines: lines, trailing: len(text), endStart: -1}
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
		w.end, w.endReady = geometry.Rect(line.X, line.Y, 0, line.Height), true
	}
	return w
}

func whitespaceKind(r rune) int {
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

func (w *whitespaceLayout) pending(boundaries bool) bool {
	return w.run < len(w.runs) || boundaries && (!w.leadingDone || !w.trailingDone)
}

func whitespaceIntersects(a, b geometry.Rectangle) bool {
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

func (w *whitespaceLayout) cell(c typography.TextCluster) (geometry.Rectangle, bool) {
	if c.LineIndex < 0 || c.LineIndex >= len(w.lines) || c.Start < 0 || c.Length <= 0 || c.Start > len(w.text)-c.Length {
		return geometry.Rectangle{}, false
	}
	line := w.lines[c.LineIndex]
	return geometry.Rect(c.X, line.Y, c.Width, line.Height), true
}

// A pass has both a wall-clock deadline and a finite input budget, even when
// the caller supplies no deadline. Tests may repeatedly advance an index.
func (w *whitespaceLayout) advance(deadline time.Time, boundaries bool) {
	for work := 0; work < 8192 && w.pending(boundaries); work += whitespaceClusterBlock {
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
					if c.Start+c.Length == len(w.text) && c.Start > w.endStart {
						x := rect.X + rect.Width
						if c.Direction == typography.TextRightToLeft {
							x = rect.X
						}
						w.end, w.endStart, w.endReady = geometry.Rect(x, rect.Y, 0, rect.Height), c.Start, true
						w.endRTL = c.Direction == typography.TextRightToLeft
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
func (w *whitespaceLayout) visit(clip geometry.Rectangle, fn func(typography.TextCluster, geometry.Rectangle)) int {
	visited := 0
	var walk func(int)
	walk = func(index int) {
		node := w.nodes[index]
		if !whitespaceIntersects(node.bounds, clip) {
			return
		}
		if node.left >= 0 {
			walk(node.left)
			walk(node.right)
			return
		}
		for _, c := range w.runs[node.run][node.first:node.end] {
			visited++
			if rect, ok := w.cell(c); ok && whitespaceIntersects(rect, clip) {
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
func (w *whitespaceLayout) chunk(at int, backward bool) string {
	if w.read != nil {
		return w.read(at, backward)
	}
	a, b := at, min(len(w.text), at+512)
	if backward {
		a, b = max(0, at-512), at
		for a < b && !utf8.RuneStart(w.text[a]) {
			a++
		}
	} else {
		for b < len(w.text) && b > a && !utf8.RuneStart(w.text[b]) {
			b--
		}
	}
	return w.text[a:b]
}

func (w *whitespaceLayout) scanBoundaries() {
	if !w.leadingDone {
		text := w.chunk(w.leading, false)
		for n := 0; n < 128 && text != ""; n++ {
			r, size := utf8.DecodeRuneInString(text)
			if whitespaceKind(r) == 0 {
				w.leadingDone = true
				break
			}
			w.leading += size
			text = text[size:]
		}
		w.leadingDone = w.leadingDone || w.leading == w.length
	}
	if !w.trailingDone {
		text := w.chunk(w.trailing, true)
		for n := 0; n < 128 && text != ""; n++ {
			r, size := utf8.DecodeLastRuneInString(text)
			if whitespaceKind(r) == 0 {
				w.trailingDone = true
				break
			}
			w.trailing -= size
			text = text[:len(text)-size]
		}
		w.trailingDone = w.trailingDone || w.trailing == 0
	}
}

func (w *whitespaceLayout) show(mode WhitespaceMode, start, end, kind int, selected bool) bool {
	switch mode {
	case WhitespaceAll:
		return true
	case WhitespaceSelection:
		return selected
	case WhitespaceTrailing:
		return w.textStart+start >= w.trailing
	case WhitespaceBoundary:
		if kind != 1 || w.textStart+start < w.leading || w.textStart+start >= w.trailing || start > 0 && w.text[start-1] == ' ' || end < len(w.text) && w.text[end] == ' ' {
			return true
		}
		if start == 0 && w.textStart > 0 {
			text := w.chunk(w.textStart, true)
			if strings.HasSuffix(text, " ") {
				return true
			}
		}
		if end == len(w.text) && w.textStart+end < w.length {
			return strings.HasPrefix(w.chunk(w.textStart+end, false), " ")
		}
		return false
	}
	return false
}

func (v *SourceView) whitespaceStep(deadline time.Time) {
	if v.whitespace == WhitespaceNone && !v.showLineEndings || !v.mounted || !visible(v) {
		return
	}
	boundaries := v.whitespace == WhitespaceBoundary || v.whitespace == WhitespaceTrailing
	first, last := v.visibleRows()
	for row := first; row <= last; row = v.nextDisplayRow(row) {
		if _, ok := v.highlightRow(row); !ok {
			continue
		}
		p := v.cachedParagraph(row)
		if p == nil || p.whitespace == nil || !p.whitespace.pending(boundaries) {
			continue
		}
		p.whitespace.advance(deadline, boundaries)
		v.RequestPaint()
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return
		}
	}
}

func (v *SourceView) paintWhitespace(draw gui.Painter, row int, p *paragraph, origin geometry.Point) {
	if v.whitespace == WhitespaceNone && !v.showLineEndings || v.viewport.Width <= 0 || v.viewport.Height <= 0 {
		return
	}
	sourceRow, ok := v.highlightRow(row)
	if !ok {
		return
	} // An IME-covered paragraph has no committed source mapping.
	w := p.whitespace
	if w == nil {
		return
	}
	boundaries := v.whitespace == WhitespaceBoundary || v.whitespace == WhitespaceTrailing
	if !v.frameLayout {
		for w.pending(boundaries) {
			w.advance(time.Time{}, boundaries)
		}
	} else if w.pending(boundaries) {
		v.RequestPaint()
	}
	line, _ := v.document.model.LineRange(sourceRow)
	selection := v.Selection().Range()
	clip := geometry.Rect(v.gutter-origin.X, -origin.Y, max(0, v.viewport.Width-v.gutter), v.viewport.Height)
	fallback := whitespaceColor(v.colors.Background)
	normal := v.partColor("whitespace", fallback)
	selectedColor := v.partColor("selected-whitespace", fallback)
	if v.whitespace != WhitespaceNone {
		w.visit(clip, func(c typography.TextCluster, rect geometry.Rectangle) {
			start, end := c.Start, c.Start+c.Length
			r, size := utf8.DecodeRuneInString(w.text[start:end])
			kind := whitespaceKind(r)
			if kind == 0 {
				return
			}
			// A backend may merge a run of blank characters into one indivisible
			// cluster. Mark that measured cell once; never invent internal stops.
			for at := start + size; at < end; {
				next, n := utf8.DecodeRuneInString(w.text[at:end])
				if whitespaceKind(next) != kind {
					return
				}
				at += n
			}
			selected := selection.Start <= line.Start+w.textStart+start && selection.End >= line.Start+w.textStart+end
			if !w.show(v.whitespace, start, end, kind, selected) {
				return
			}
			color := normal
			if selected {
				color = selectedColor
			}
			v.paintWhitespaceCell(draw, rect.Translate(origin), kind, c.Direction == typography.TextRightToLeft, color)
		})
	}
	if v.showLineEndings && sourceRow+1 < v.document.model.LineCount() && w.textStart+len(w.text) == w.length && w.endReady {
		r := w.end.Translate(origin)
		x, y := r.X+1, r.Y+r.Height*.55
		if w.endRTL {
			x = r.X - 6
		}
		mark := geometry.Rect(x, y-4, 5, 7)
		if !whitespaceIntersects(mark, clip.Translate(origin)) {
			return
		}
		color := normal
		if selection.Start <= line.End && selection.End > line.End {
			color = selectedColor
		}
		brush := graphics.ColorOf(color)
		draw.DrawLine(geometry.Point{X: x + 4, Y: y - 4}, geometry.Point{X: x + 4, Y: y}, 1, brush)
		draw.DrawLine(geometry.Point{X: x + 4, Y: y}, geometry.Point{X: x, Y: y}, 1, brush)
		draw.DrawLine(geometry.Point{X: x + 2, Y: y - 2}, geometry.Point{X: x, Y: y}, 1, brush)
		draw.DrawLine(geometry.Point{X: x + 2, Y: y + 2}, geometry.Point{X: x, Y: y}, 1, brush)
	}
}

func (v *SourceView) paintWhitespaceCell(draw gui.Painter, r geometry.Rectangle, kind int, rtl bool, color color.NRGBA) {
	brush := graphics.ColorOf(color)
	unit := min(v.spaceWidth, r.Width, r.Height)
	if unit <= 0 {
		return
	}
	x, y := r.X+r.Width/2, r.Y+r.Height/2
	stroke := unit / 7
	switch kind {
	case 1:
		// Rectangle edge snapping can flatten a tiny rounded square. An
		// ellipse keeps equal radii under fractional translations and DPI.
		draw.FillEllipse(geometry.Point{X: x, Y: y}, unit/7, unit/7, brush)
	case 2:
		// Draw one short silhouette at the logical leading edge. Separate
		// translucent strokes would darken their shared arrowhead joins.
		point := func(dx, dy float32) geometry.Point {
			px := r.X + unit*.05 + dx
			if rtl {
				px = r.X + r.Width - unit*.05 - dx
			}
			return geometry.Point{X: px, Y: y + dy}
		}
		half, head, end := stroke/2, unit*.25, unit*.85
		points := [...]geometry.Point{
			point(0, -half), point(end-head+half, -half),
			point(end-head-half, -head+half), point(end-head+half, -head-half),
			point(end+half, 0), point(end-head+half, head+half),
			point(end-head-half, head-half), point(end-head+half, half), point(0, half),
		}
		path := graphics.MoveTo(points[0].X, points[0].Y)
		for _, p := range points[1:] {
			path = path.LineTo(p.X, p.Y)
		}
		draw.FillPath(path.Close(), brush)
	case 3:
		draw.DrawEllipse(geometry.Point{X: x, Y: y}, unit/4, unit/4, stroke, brush)
	case 4:
		size := unit * .75
		draw.DrawRect(geometry.Rect(x-size/2, y-size/2, size, size), stroke, brush)
	}
}

// refreshWhitespace adapts an existing partial Geometry snapshot. It uses
// measured atomic stops, never reshapes substrings or guesses character widths.
// The caller supplies the fragment's byte origin and geometry generation.
func (v *SourceView) refreshWhitespace(p *paragraph, start int, version uint64) {
	row, ok := v.highlightRow(p.row)
	if !ok {
		p.whitespace = nil
		return
	}
	source, _ := v.document.model.LineRange(row)
	length := source.End - source.Start
	if w := p.whitespace; w != nil && w.version == version && w.revision == v.document.model.Revision() && w.textStart == start && w.text == p.text && w.length == length {
		return
	}
	var lines []typography.TextLine
	var clusters []typography.TextCluster
	for at := 0; at < len(p.text); {
		end := p.geometry.Snap(start+at+1, true) - start
		if end <= at || end > len(p.text) {
			break
		}
		r, _ := utf8.DecodeRuneInString(p.text[at:end])
		if whitespaceKind(r) != 0 {
			a := p.geometry.Caret(textedit.Position{Offset: start + at})
			b := p.geometry.Caret(textedit.Position{Offset: start + end, Upstream: true})
			direction := typography.TextLeftToRight
			if a.Y == b.Y && a.X > b.X {
				direction = typography.TextRightToLeft
			}
			for _, rect := range p.geometry.Selection(textedit.Range{Start: start + at, End: start + end}) {
				clusters = append(clusters, typography.TextCluster{Start: at, Length: end - at, X: rect.X, Width: rect.Width, LineIndex: len(lines), Direction: direction})
				lines = append(lines, typography.TextLine{Y: rect.Y, Height: rect.Height})
			}
		}
		at = end
	}
	w := newWhitespaceLayout(p.text, lines, clusters)
	w.version, w.textStart, w.length, w.trailing = version, start, length, length
	w.revision = v.document.model.Revision()
	// The model and revision belong to this snapshot, including its edge reader.
	// A replacement paragraph always receives a different snapshot/reader.
	model, revision := v.document.model, v.document.model.Revision()
	w.read = func(at int, backward bool) string {
		if model.Revision() != revision {
			w.leadingDone, w.trailingDone = true, true
			return ""
		}
		a, b := at, min(length, at+512)
		if backward {
			a, b = max(0, at-512), at
			for a < b && !model.IsRuneBoundary(source.Start+a) {
				a++
			}
		} else {
			for b > a && !model.IsRuneBoundary(source.Start+b) {
				b--
			}
		}
		text, _ := model.Slice(textedit.Range{Start: source.Start + a, End: source.Start + b})
		return text
	}
	if start+len(p.text) == length {
		w.end = p.geometry.Caret(textedit.Position{Offset: length, Upstream: true})
		w.endReady = true
		if length > start {
			at := p.geometry.Snap(length-1, false)
			before := p.geometry.Caret(textedit.Position{Offset: at})
			w.endRTL = before.Y == w.end.Y && before.X > w.end.X
		}
	}
	p.whitespace = w
}
