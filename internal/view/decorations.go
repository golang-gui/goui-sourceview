package view

import (
	"fmt"
	"image/color"
	"slices"

	"github.com/golang-gui/goui-sourceview/internal/resource"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/graphics"
)

func PaintWhitespaceCell(draw gui.Painter, r geometry.Rectangle, kind int, rtl bool, color color.NRGBA, spaceWidth float32) {
	brush := graphics.ColorOf(color)
	unit := min(spaceWidth, r.Width, r.Height)
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

type MarkerKind uint8

const (
	MarkerBreakpoint MarkerKind = iota
	MarkerConditionalBreakpoint
	MarkerLogpoint
	MarkerBookmark
)

type MarkerState uint8

const (
	MarkerEnabled MarkerState = iota
	MarkerDisabled
	MarkerPending
	MarkerRejected
)

// PaintLineMark consumes resolved colors and a visible row; the editor owns
// aggregation, style lookup, hit testing, accessibility and document anchors.
func PaintLineMark(p gui.Painter, r geometry.Rectangle, kind MarkerKind, state MarkerState, count int, foreground, background color.NRGBA) {
	c := graphics.ColorOf(foreground)
	x, y := r.Width*.38, r.Y+r.Height/2
	size := min(5.5, r.Height*.27)
	switch kind {
	case MarkerLogpoint:
		path := graphics.MoveTo(x, y-size).LineTo(x+size, y).LineTo(x, y+size).LineTo(x-size, y).Close()
		if state == MarkerEnabled {
			p.FillPath(path, c)
		} else {
			p.DrawPath(path, 1.3, c)
		}
	case MarkerBookmark:
		path := graphics.MoveTo(x-size*.7, y-size).LineTo(x+size*.7, y-size).LineTo(x+size*.7, y+size).LineTo(x, y+size*.5).LineTo(x-size*.7, y+size).Close()
		if state == MarkerEnabled {
			p.FillPath(path, c)
		} else {
			p.DrawPath(path, 1.3, c)
		}
	default:
		if state == MarkerEnabled {
			p.FillEllipse(geometry.Point{X: x, Y: y}, size, size, c)
		} else {
			p.DrawEllipse(geometry.Point{X: x, Y: y}, size, size, 1.3, c)
		}
		if kind == MarkerConditionalBreakpoint {
			p.FillEllipse(geometry.Point{X: x, Y: y}, 1.5, 1.5, graphics.ColorOf(background))
		}
	}
	if state == MarkerRejected {
		p.DrawLine(geometry.Point{X: x - size, Y: y + size}, geometry.Point{X: x + size, Y: y - size}, 1.3, c)
	}
	if state == MarkerPending {
		p.FillEllipse(geometry.Point{X: x, Y: y}, 1, 1, c)
	}
	if count > 1 {
		p.FillEllipse(geometry.Point{X: x + size + 2, Y: y - size}, 1.5, 1.5, c)
	}
}

// Only a confirmed pair receives matching decoration. A lone candidate may be
// unmatched or simply have its partner beyond the bounded analysis window.
func BracketRectangles(measured *textcore.Geometry, line textcore.Range, origin geometry.Point, brackets []textcore.Range) []geometry.Rectangle {
	if len(brackets) != 2 {
		return nil
	}
	var result []geometry.Rectangle
	for _, bracket := range brackets {
		if bracket.End <= line.Start || bracket.Start >= line.End {
			continue
		}
		local := textcore.Range{Start: max(line.Start, bracket.Start) - line.Start, End: min(line.End, bracket.End) - line.Start}
		for _, rect := range measured.Selection(local) {
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

func MarkerLabel(kind MarkerKind, state MarkerState, label string, line int) string {
	if label == "" {
		switch kind {
		case MarkerConditionalBreakpoint:
			label = resource.ConditionalBreakpoint()
		case MarkerLogpoint:
			label = resource.Logpoint()
		case MarkerBookmark:
			label = resource.Bookmark()
		default:
			label = resource.Breakpoint()
		}
	}
	switch state {
	case MarkerDisabled:
		label += " · " + resource.MarkDisabled()
	case MarkerPending:
		label += " · " + resource.MarkPending()
	case MarkerRejected:
		label += " · " + resource.MarkRejected()
	}
	return fmt.Sprintf("%s (%d)", label, line+1)
}
