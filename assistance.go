package sourceview

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
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
	word := v.document.definition.IsWord
	for _, c := range text {
		if !word(c) {
			return ""
		}
	}
	m := v.document.model
	if r.Start > 0 {
		at := v.adjacent(r.Start, false)
		previous, _ := m.Slice(textedit.Range{Start: at, End: r.Start})
		c, _ := utf8.DecodeLastRuneInString(previous)
		if word(c) {
			return ""
		}
	}
	if r.End < m.Len() {
		at := v.adjacent(r.End, true)
		after, _ := m.Slice(textedit.Range{Start: r.End, End: at})
		c, _ := utf8.DecodeRuneInString(after)
		if word(c) {
			return ""
		}
	}
	return text
}
