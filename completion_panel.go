package sourceview

import (
	"math"

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
	inset := p.inset()
	size := c.Clamp(geometry.Size{Width: min(520, l.width+2*(inset-completionInset)), Height: 2*inset + float32(min(completionRows, len(l.session.items)))*l.rowHeight})
	p.scroll.SetLineHeight(l.rowHeight)
	p.scroll.Measure(layout.Tight(size.Inset(inset)))
	return layout.Measured(size)
}

func (p *completionPanel) Arrange(r geometry.Rectangle) {
	p.WidgetBase.Arrange(r)
	p.scroll.Arrange(geometry.Rect(0, 0, r.Width, r.Height).Inset(p.inset()))
}

// Reserve a conservative safe interior for the styled Popover's rounded
// corners. The shell, shadow and border are drawn only by Popover itself.
func (p *completionPanel) inset() float32 {
	return max(completionInset, float32(math.Ceil(float64(p.list.box.radius/2)))+p.list.box.width)
}

func (p *completionPanel) Snapshot() gui.WidgetInfo {
	info := p.WidgetBase.Snapshot()
	info.Role = "completion-panel"
	return info
}
