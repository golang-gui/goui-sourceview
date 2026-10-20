package sourceview

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
)

// The panel owns finite popup geometry and padding. ScrollView owns scrolling;
// completionList owns only the candidate layouts intersecting that viewport.
type completionPanel struct {
	gui.WidgetBase
	list   *completionList
	scroll *gui.ScrollView
}

func newCompletionPanel(list *completionList) *completionPanel {
	p := &completionPanel{list: list, scroll: gui.NewScrollView()}
	p.scroll.SetDirection(gui.ScrollVertical)
	p.scroll.SetMainWeight(0)
	p.scroll.SetChild(list)
	p.WidgetBase.AddChild(p, p.scroll)
	return p
}

func (p *completionPanel) Measure(c layout.Constraint) layout.Measurement {
	l := p.list
	l.measureItems()
	size := c.Clamp(geometry.Size{Width: l.width, Height: 2*completionInset + float32(min(completionRows, len(l.session.items)))*l.rowHeight})
	p.scroll.SetLineHeight(l.rowHeight)
	p.scroll.Measure(layout.Tight(size.Inset(completionInset)))
	return layout.Measured(size)
}

func (p *completionPanel) Arrange(r geometry.Rectangle) {
	p.WidgetBase.Arrange(r)
	p.scroll.Arrange(geometry.Rect(0, 0, r.Width, r.Height).Inset(completionInset))
}

func (p *completionPanel) Paint(draw gui.Painter) {
	p.list.box.paint(draw, geometry.Rect(0, 0, p.Rect().Width, p.Rect().Height))
}

func (p *completionPanel) Snapshot() gui.WidgetInfo {
	info := p.WidgetBase.Snapshot()
	info.Role = "completion-panel"
	return info
}
