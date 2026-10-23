// 共享的绘制记录器；断言形状、颜色与绘制顺序，不模拟后端光栅化。
package sourceview

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
)

type paintShape struct {
	rect  geometry.Rectangle
	width float32
	color graphics.Color
}

type recordingPainter struct {
	gui.Painter
	fills, borders []paintShape
	lines          int
	order          []string
}

func (p *recordingPainter) Save() {}

func (p *recordingPainter) Restore() {}

func (p *recordingPainter) SetClipRect(geometry.Rectangle) {}

func (p *recordingPainter) FillRect(r geometry.Rectangle, b graphics.Brush) {
	p.fills = append(p.fills, paintShape{rect: r, color: b.(graphics.Color)})
	p.order = append(p.order, "fill")
}

func (p *recordingPainter) DrawRect(r geometry.Rectangle, w float32, b graphics.Brush) {
	p.borders = append(p.borders, paintShape{rect: r, width: w, color: b.(graphics.Color)})
	p.order = append(p.order, "border")
}

func (p *recordingPainter) DrawLine(geometry.Point, geometry.Point, float32, graphics.Brush) {
	p.lines++
}

func (p *recordingPainter) DrawTextLayout(geometry.Point, typography.TextLayout) {
	p.order = append(p.order, "text")
}
