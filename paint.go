package sourceview

import (
	"image/color"
	"slices"
	"strconv"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
)

func (v *SourceView) Paint(p gui.Painter) {
	bounds := geometry.Rect(0, 0, v.Rect().Width, v.Rect().Height)
	p.FillRect(bounds, graphics.ColorOf(v.colors.Background))
	if v.context == nil || v.mounted && v.observed != v.document.model.Revision() {
		return
	}
	v.ensureMetrics()
	v.updateGutter()
	first, last := v.visibleRows()
	p.Save()
	p.SetClipRect(geometry.Rect(v.gutter, 0, max(0, bounds.Width-v.gutter), bounds.Height))
	caretRow := v.displayLine(v.displayCaret())
	selection := v.Selection().Range()
	occurrence := v.occurrenceWord()
	var brackets []textedit.Range
	if activeFocus(v) && (v.colors.BracketMatchBackground.A != 0 || v.colors.BracketMatchBorder.A != 0) {
		brackets = v.matchingBrackets()
	}
	for row := first; row <= last; row = v.nextDisplayRow(row) {
		item := v.viewportParagraph(row)
		if item == nil {
			continue
		}
		r := v.displayRange(row)
		origin := v.paragraphOrigin(row, item)
		if activeFocus(v) && row == caretRow {
			lineY := v.geometryOrigin(row, 0, 0).Y
			top, bottom := max(0, lineY), min(bounds.Height, lineY+item.height)
			if bottom > top {
				p.FillRect(geometry.Rect(v.gutter, top, max(0, bounds.Width-v.gutter), bottom-top), graphics.ColorOf(v.colors.CurrentLine))
			}
		}
		v.paintAssistanceBackground(p, row, item, origin, occurrence)
		if v.projection == nil {
			// Put the quiet fill below search/selection, and the outline above
			// those backgrounds. Neither pass changes syntax foregrounds.
			bracketRects := bracketRectangles(item, r, origin, brackets)
			if c := v.colors.BracketMatchBackground; c.A != 0 {
				for _, rect := range bracketRects {
					p.FillRect(rect, graphics.ColorOf(c))
				}
			}
			if v.highlightSearchMatches {
				searchRange := r
				if item.paged != nil {
					visible := item.paged.VisibleRange()
					searchRange = textedit.Range{Start: r.Start + visible.Start, End: r.Start + visible.End}
				}
				for _, match := range v.search.InRange(v.document.model, searchRange, 2000) {
					c := v.colors.SearchBackground
					if v.currentMatch != nil && v.currentMatch.raw == match {
						c = v.colors.CurrentSearchBackground
					}
					v.paintRange(p, item, r, origin, match, c)
				}
			}
			c := v.colors.InactiveSelectionBackground
			if activeFocus(v) {
				c = v.colors.SelectionBackground
			}
			v.paintRange(p, item, r, origin, selection, c)
			if c := v.colors.BracketMatchBorder; c.A != 0 {
				for _, rect := range bracketRects {
					// Strokes are centered on the path. Keep the 1-DIP border
					// inside its character cell, including at viewport edges.
					p.DrawRect(geometry.Rect(rect.X+.5, rect.Y+.5, rect.Width-1, rect.Height-1), 1, graphics.ColorOf(c))
				}
			}
			if selection.Start <= r.End && selection.End > r.End && row < v.displayCount()-1 && (item.paged == nil || item.paged.CachedOffset(r.End-r.Start)) {
				edge := item.geometry.Caret(textedit.Position{Offset: r.End - r.Start, Upstream: true})
				p.FillRect(geometry.Rect(origin.X+edge.X, origin.Y+edge.Y, v.spaceWidth, edge.Height), graphics.ColorOf(c))
			}
		}
		if item.paged != nil {
			item.paged.Paint(p, origin)
		} else {
			p.DrawTextLayout(origin, item.layout)
		}
		v.paintWhitespace(p, row, item, origin)
		v.paintFoldBadge(p, row, item)
		if v.projection != nil {
			start := v.projection.Replacement().Start
			end := start + len(v.projection.Text())
			local := textedit.Range{Start: max(r.Start, start) - r.Start, End: min(r.End, end) - r.Start}
			if local.End > local.Start {
				for _, rect := range v.visibleSelection(item, local, origin) {
					y := origin.Y + rect.Y + rect.Height - 1
					p.DrawLine(geometry.Point{X: origin.X + rect.X, Y: y}, geometry.Point{X: origin.X + rect.X + rect.Width, Y: y}, 1, graphics.ColorOf(v.colors.PreeditUnderline))
				}
			}
		}
	}
	if activeFocus(v) && v.caretOn && gui.IsEnabled(v) {
		if r, ok := v.cachedCaretRect(); ok {
			p.FillRect(r, graphics.ColorOf(v.colors.Caret))
			v.im.SetCaretRect(r)
		}
	}
	p.Restore()
	if v.gutter > 0 {
		p.Save()
		p.SetClipRect(geometry.Rect(0, 0, v.gutter, bounds.Height))
		p.FillRect(geometry.Rect(0, 0, v.gutter, bounds.Height), graphics.ColorOf(v.colors.GutterBackground))
		for row := first; row <= last; row = v.nextDisplayRow(row) {
			item := v.viewportParagraph(row)
			if item == nil {
				continue
			}
			v.paintFoldButton(p, row)
			if !v.showLineNumbers {
				continue
			}
			number := row + 1
			if v.projection != nil {
				number = v.document.model.LineAt(v.projection.ModelOffset(v.displayRange(row).Start, false)) + 1
			}
			numberText := strconv.Itoa(number)
			if item.number == nil || item.numberText != numberText {
				if item.number != nil {
					item.number.Destroy()
					item.number = nil
				}
				native, err := v.context.NewTextLayout(numberText, typography.TextFormat{Font: v.font, TextColor: v.colors.GutterForeground}, 1<<16, 1<<16)
				if err != nil {
					continue
				}
				item.number = native
				item.numberText = numberText
			}
			w, _ := item.number.MeasureSize()
			p.DrawTextLayout(geometry.Point{X: v.gutter - v.foldColumn - padding - w, Y: padding + v.paragraphTop(row) - v.offset.Y}, item.number)
		}
		p.Restore()
		p.DrawLine(geometry.Point{X: v.gutter - .5}, geometry.Point{X: v.gutter - .5, Y: bounds.Height}, 1, graphics.ColorOf(v.colors.Border))
	}
}

// Only a confirmed pair receives matching decoration. A lone candidate may be
// unmatched or simply have its partner beyond the bounded analysis window.
func bracketRectangles(item *paragraph, line textedit.Range, origin geometry.Point, brackets []textedit.Range) []geometry.Rectangle {
	if len(brackets) != 2 {
		return nil
	}
	var result []geometry.Rectangle
	for _, bracket := range brackets {
		if bracket.End <= line.Start || bracket.Start >= line.End {
			continue
		}
		local := textedit.Range{Start: max(line.Start, bracket.Start) - line.Start, End: min(line.End, bracket.End) - line.Start}
		for _, rect := range item.geometry.Selection(local) {
			// Native visual-line geometry keeps wrapped/bidi positions exact.
			// A small vertical inset separates the box from neighboring rows.
			if rect.Width <= 1 || rect.Height <= 3 {
				continue
			}
			rect.Y++
			rect.Height -= 2
			rect = rect.Translate(origin)
			// A shaping cluster may contain both characters. Paint its shared
			// rectangle once so translucent decorations do not accumulate.
			if !slices.Contains(result, rect) {
				result = append(result, rect)
			}
		}
	}
	return result
}

func (v *SourceView) paintRange(p gui.Painter, item *paragraph, line textedit.Range, origin geometry.Point, r textedit.Range, c color.NRGBA) {
	start, end := max(line.Start, r.Start)-line.Start, min(line.End, r.End)-line.Start
	if end <= start {
		return
	}
	for _, rect := range v.visibleSelection(item, textedit.Range{Start: start, End: end}, origin) {
		p.FillRect(rect.Translate(origin), graphics.ColorOf(c))
	}
}

func (v *SourceView) visibleSelection(item *paragraph, r textedit.Range, origin geometry.Point) []geometry.Rectangle {
	if v.viewport.Height <= 0 {
		return nil
	}
	first := item.geometry.LineAtY(-origin.Y)
	end := item.geometry.LineAtY(v.viewport.Height-origin.Y) + 1
	return item.geometry.SelectionLines(r, first, end)
}
