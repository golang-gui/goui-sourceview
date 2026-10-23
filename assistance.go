package sourceview

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/graphics"
)

type WhitespaceMode uint8

const (
	WhitespaceNone WhitespaceMode = iota
	WhitespaceSelection
	WhitespaceTrailing
	WhitespaceAll
)

func (v *SourceView) WhitespaceMode() WhitespaceMode { return v.whitespace }

func (v *SourceView) SetWhitespaceMode(mode WhitespaceMode) error {
	if mode > WhitespaceAll {
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
	before, err := v.document.model.Slice(textedit.Range{Start: r.Start, End: s.Caret})
	if err != nil {
		return true, err
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
	index := p.geometry.LineIndex(pos)
	edge := p.geometry.LineEdge(index, key == events.KeyEnd)
	if !v.smartHomeEnd {
		return edge
	}
	line := p.geometry.LineMetrics(index)
	a, b := max(0, line.Start), min(len(p.text), line.Start+line.Length)
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
	if at == pos.Offset {
		return edge
	}
	return textedit.Position{Offset: at, Upstream: key == events.KeyEnd}
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
	for _, col := range v.rulers {
		x := v.gutter + padding - v.offset.X + float32(col)*v.spaceWidth
		draw.DrawLine(geometry.Point{X: x, Y: origin.Y}, geometry.Point{X: x, Y: origin.Y + p.height}, 1, ruler)
	}
	if v.indentGuides {
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
		r := textedit.Range{Start: line.Start + start, End: line.Start + end}
		if r == v.Selection().Range() {
			continue
		}
		v.paintRange(draw, p, line, origin, r, v.partColor("occurrence", tint(v.colors.SearchBackground, 55)))
	}
}

func (v *SourceView) paintWhitespace(draw gui.Painter, row int, p *paragraph, origin geometry.Point) {
	if v.whitespace == WhitespaceNone && !v.showLineEndings {
		return
	}
	c := graphics.ColorOf(v.partColor("whitespace", tint(v.colors.GutterForeground, 125)))
	trailing := len(strings.TrimRight(p.text, " \t"))
	line := v.displayRange(row)
	selection := v.Selection().Range()
	for at, b := range []byte(p.text) {
		if b != ' ' && b != '\t' {
			continue
		}
		show := v.whitespace == WhitespaceAll || v.whitespace == WhitespaceTrailing && at >= trailing || v.whitespace == WhitespaceSelection && v.projection == nil && selection.Start <= line.Start+at && selection.End > line.Start+at
		if !show {
			continue
		}
		for _, r := range p.geometry.Selection(textedit.Range{Start: at, End: at + 1}) {
			x, y := origin.X+r.X, origin.Y+r.Y+r.Height*.55
			if b == ' ' {
				draw.FillRect(geometry.Rect(x+r.Width/2-.75, y-.75, 1.5, 1.5), c)
			} else if r.Width >= 5 {
				left, right := x+2, x+r.Width-2
				draw.DrawLine(geometry.Point{X: left, Y: y}, geometry.Point{X: right, Y: y}, 1, c)
				draw.DrawLine(geometry.Point{X: right - 3, Y: y - 2}, geometry.Point{X: right, Y: y}, 1, c)
				draw.DrawLine(geometry.Point{X: right - 3, Y: y + 2}, geometry.Point{X: right, Y: y}, 1, c)
			}
		}
	}
	if v.showLineEndings && v.projection == nil && row+1 < v.document.model.LineCount() {
		r := p.geometry.Caret(textedit.Position{Offset: len(p.text), Upstream: true})
		x, y := origin.X+r.X+3, origin.Y+r.Y+r.Height*.55
		draw.DrawLine(geometry.Point{X: x + 5, Y: y - 4}, geometry.Point{X: x + 5, Y: y}, 1, c)
		draw.DrawLine(geometry.Point{X: x + 5, Y: y}, geometry.Point{X: x, Y: y}, 1, c)
		draw.DrawLine(geometry.Point{X: x + 2, Y: y - 2}, geometry.Point{X: x, Y: y}, 1, c)
		draw.DrawLine(geometry.Point{X: x + 2, Y: y + 2}, geometry.Point{X: x, Y: y}, 1, c)
	}
}
