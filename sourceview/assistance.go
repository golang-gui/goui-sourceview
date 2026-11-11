package sourceview

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
)

// WhitespaceMode controls decorative markers without changing source text.
type WhitespaceMode uint8

const (
	WhitespaceNone      WhitespaceMode = WhitespaceMode(view.WhitespaceNone)      // Hide whitespace markers.
	WhitespaceSelection WhitespaceMode = WhitespaceMode(view.WhitespaceSelection) // Mark whitespace contained in the selection.
	WhitespaceTrailing  WhitespaceMode = WhitespaceMode(view.WhitespaceTrailing)  // Mark whitespace at the end of a logical line.
	WhitespaceAll       WhitespaceMode = WhitespaceMode(view.WhitespaceAll)       // Mark all supported whitespace.
	WhitespaceBoundary  WhitespaceMode = WhitespaceMode(view.WhitespaceBoundary)  // Mark indentation, trailing/repeated spaces, Tabs and special spaces.

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
	n, col := editor.LeadingColumns(before, v.indent.TabWidth)
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
	return true, v.applyPlan(editor.Plan{Before: s, After: TextSelection{Anchor: end, Caret: end}, Edits: []TextEdit{{Range: TextRange{Start: r.Start + at, End: s.Caret}, Text: insert}}})
}

func (v *SourceView) smartEdge(p *paragraph, pos textcore.Position, key events.Key) textcore.Position {
	if p.Paged != nil && v.wrap == WrapNone {
		r := v.displayRange(p.row)
		edge := textcore.Position{}
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
				return textcore.Position{Offset: at}
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
				text, _ := v.document.model.Slice(TextRange{Start: a, End: at})
				n := len(text) - len(strings.TrimRight(text, " \t"))
				at -= n
				if n < len(text) {
					break
				}
			}
			if at-r.Start != pos.Offset {
				return textcore.Position{Offset: at - r.Start, Upstream: true}
			}
		}
		return edge
	}
	g := p.GeometryAt(pos.Offset)
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
	return textcore.Position{Offset: at + p.textStart, Upstream: key == events.KeyEnd}
}

func (v *SourceView) occurrenceWord() string {
	if len(v.additional) > 0 {
		return ""
	}
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
		previous, _ := m.Slice(TextRange{Start: at, End: r.Start})
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
		after, _ := m.Slice(TextRange{Start: r.End, End: at})
		c, _ := utf8.DecodeRuneInString(after)
		if word(c) {
			return ""
		}
	}
	return text
}

func (v *SourceView) paintAssistanceBackground(draw gui.Painter, row int, p *paragraph, origin geometry.Point, word string) {
	ruler := graphics.ColorOf(v.partColor("ruler", view.Tint(v.colors.GutterForeground, 45)))
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
		bytes, columns := editor.LeadingColumns(p.text, v.indent.TabWidth)
		guide := graphics.ColorOf(v.partColor("indent-guide", view.Tint(v.colors.GutterForeground, 65)))
		col, target := 0, v.indent.IndentWidth
		for at := 0; at < bytes && target < columns; at++ {
			next := col + 1
			if p.text[at] == '\t' {
				next = col + v.indent.TabWidth - col%v.indent.TabWidth
			}
			for target <= next && target < columns {
				start := p.Geometry.Caret(textcore.Position{Offset: at})
				end := p.Geometry.Caret(textcore.Position{Offset: at + 1, Upstream: true})
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
		r := TextRange{Start: line.Start + p.textStart + start, End: line.Start + p.textStart + end}
		if r == v.Selection().Range() {
			continue
		}
		v.paintRange(draw, p, line, origin, r, v.partColor("occurrence", view.Tint(v.colors.SearchBackground, 55)))
	}
}

// Whitespace geometry borrows the unchanged layout's measured values. The
// paragraph owns both lifetimes; no extra native layout or per-marker geometry
// query is needed. A small bounding tree indexes groups of measured clusters.
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
		if p == nil || p.whitespace == nil || !p.whitespace.Pending(boundaries) {
			continue
		}
		p.whitespace.Advance(deadline, boundaries)
		v.RequestPaint()
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return
		}
	}
}

func (v *SourceView) paintWhitespace(draw gui.Painter, row int, p *paragraph, origin geometry.Point) {
	if v.whitespace == WhitespaceNone && !v.showLineEndings || v.textRight() <= 0 || v.viewport.Height <= 0 {
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
		for w.Pending(boundaries) {
			w.Advance(time.Time{}, boundaries)
		}
	} else if w.Pending(boundaries) {
		v.RequestPaint()
	}
	line, _ := v.document.model.LineRange(sourceRow)
	selection := v.Selection().Range()
	clip := geometry.Rect(v.gutter-origin.X, -origin.Y, max(0, v.textRight()-v.gutter), v.viewport.Height)
	fallback := view.WhitespaceColor(v.colors.Background)
	normal := v.partColor("whitespace", fallback)
	selectedColor := v.partColor("selected-whitespace", fallback)
	if v.whitespace != WhitespaceNone {
		w.Visit(clip, func(c typography.TextCluster, rect geometry.Rectangle) {
			start, end := c.Start, c.Start+c.Length
			r, size := utf8.DecodeRuneInString(w.Text[start:end])
			kind := view.WhitespaceKind(r)
			if kind == 0 {
				return
			}
			// A backend may merge a run of blank characters into one indivisible
			// cluster. Mark that measured cell once; never invent internal stops.
			for at := start + size; at < end; {
				next, n := utf8.DecodeRuneInString(w.Text[at:end])
				if view.WhitespaceKind(next) != kind {
					return
				}
				at += n
			}
			selected := selection.Start <= line.Start+w.TextStart+start && selection.End >= line.Start+w.TextStart+end
			if !w.Show(view.WhitespaceMode(v.whitespace), start, end, kind, selected) {
				return
			}
			color := normal
			if selected {
				color = selectedColor
			}
			view.PaintWhitespaceCell(draw, rect.Translate(origin), kind, c.Direction == typography.TextRightToLeft, color, v.spaceWidth)
		})
	}
	if v.showLineEndings && sourceRow+1 < v.document.model.LineCount() && w.TextStart+len(w.Text) == w.Length && w.EndReady {
		r := w.End.Translate(origin)
		x, y := r.X+1, r.Y+r.Height*.55
		if w.EndRTL {
			x = r.X - 6
		}
		mark := geometry.Rect(x, y-4, 5, 7)
		if !view.WhitespaceIntersects(mark, clip.Translate(origin)) {
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
	if w := p.whitespace; w != nil && w.Version == version && w.Revision == v.document.model.Revision() && w.TextStart == start && w.Text == p.text && w.Length == length {
		return
	}
	var lines []typography.TextLine
	var clusters []typography.TextCluster
	for at := 0; at < len(p.text); {
		end := p.Geometry.Snap(start+at+1, true) - start
		if end <= at || end > len(p.text) {
			break
		}
		r, _ := utf8.DecodeRuneInString(p.text[at:end])
		if view.WhitespaceKind(r) != 0 {
			a := p.Geometry.Caret(textcore.Position{Offset: start + at})
			b := p.Geometry.Caret(textcore.Position{Offset: start + end, Upstream: true})
			direction := typography.TextLeftToRight
			if a.Y == b.Y && a.X > b.X {
				direction = typography.TextRightToLeft
			}
			for _, rect := range p.Geometry.Selection(TextRange{Start: start + at, End: start + end}) {
				clusters = append(clusters, typography.TextCluster{Start: at, Length: end - at, X: rect.X, Width: rect.Width, LineIndex: len(lines), Direction: direction})
				lines = append(lines, typography.TextLine{Y: rect.Y, Height: rect.Height})
			}
		}
		at = end
	}
	w := view.NewWhitespaceLayout(p.text, lines, clusters)
	w.Version, w.TextStart, w.Length, w.Trailing = version, start, length, length
	w.Revision = v.document.model.Revision()
	// The model and revision belong to this snapshot, including its edge reader.
	// A replacement paragraph always receives a different snapshot/reader.
	model, revision := v.document.model, v.document.model.Revision()
	w.Read = func(at int, backward bool) string {
		if model.Revision() != revision {
			w.LeadingDone, w.TrailingDone = true, true
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
		text, _ := model.Slice(TextRange{Start: source.Start + a, End: source.Start + b})
		return text
	}
	if start+len(p.text) == length {
		w.End = p.Geometry.Caret(textcore.Position{Offset: length, Upstream: true})
		w.EndReady = true
		if length > start {
			at := p.Geometry.Snap(length-1, false)
			before := p.Geometry.Caret(textcore.Position{Offset: at})
			w.EndRTL = before.Y == w.End.Y && before.X > w.End.X
		}
	}
	p.whitespace = w
}
