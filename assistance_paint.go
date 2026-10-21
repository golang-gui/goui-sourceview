package sourceview

import (
	"image/color"
	"strings"
	"unicode/utf8"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/graphics"
)

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
