package controls

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/style"
)

type GoToUI struct {
	Surface     *gotoSurface
	Row         *gui.LinearBox
	Input       *gui.TextInput
	Status      *gui.Label
	CloseButton *gui.Button
}

func NewGoToUI(refresh, submit, close func()) *GoToUI {
	b := &GoToUI{}
	b.Surface = &gotoSurface{ui: b}
	l := layout.NewLinearLayout(layout.DirectionVertical)
	l.Padding = geometry.UniformInsets(12)
	l.Spacing = 4
	l.CrossAlign = layout.CrossStretch
	b.Surface.SetLayoutManager(l)
	b.Surface.SetVisible(false)
	b.Row = gui.NewLinearBox(layout.DirectionHorizontal)
	b.Row.SetSpacing(4)
	b.Input = gui.NewTextInput()
	b.Input.SetStyleName("sourceview.find.input")
	b.Input.SetPadding(geometry.UniformInsets(4))
	b.Input.SetMainWeight(1)
	b.Input.ConnectText(func(string) { refresh() })
	b.Input.ConnectSubmit(submit)
	b.CloseButton = gui.NewButton()
	b.CloseButton.SetStyleName("sourceview.find.action")
	b.CloseButton.SetPadding(geometry.UniformInsets(3))
	icon := gui.NewIcon(findIcons["close"])
	icon.SetStyleName("sourceview.find.icon")
	b.CloseButton.SetChild(icon)
	b.CloseButton.ConnectClicked(close)
	b.Row.AddChild(b.Input)
	b.Row.AddChild(b.CloseButton)
	b.Status = gui.NewLabel("")
	b.Status.SetStyleName("sourceview.find.status")
	b.Surface.WidgetBase.AddChild(b.Surface, b.Row)
	b.Surface.WidgetBase.AddChild(b.Surface, b.Status)

	return b
}

type gotoSurface struct {
	gui.WidgetBase
	inset float32
	ui    *GoToUI
}

func (s *gotoSurface) Measure(c layout.Constraint) layout.Measurement {
	if !s.Visible() {
		return layout.Measurement{}
	}
	s.inset = max(1, BoxOf(gui.ResolveWidgetStyle(s, "sourceview.find", "", style.Normal)).Outset()+1)
	s.LayoutManager().(*layout.LinearLayout).Padding = geometry.UniformInsets(s.inset + 6)
	width := min(float32(360), c.Max.Width)
	c.Min.Width = width
	c.Max.Width = width
	return s.WidgetBase.Measure(c)
}
func (s *gotoSurface) Paint(p gui.Painter) {
	BoxOf(gui.ResolveWidgetStyle(s, "sourceview.find", "", style.Normal)).Paint(p, geometry.Rect(0, 0, s.Rect().Width, s.Rect().Height).Inset(s.inset))
	b := s.ui
	state := style.Normal
	if b.Input.Focused() {
		state = style.Focused
	}
	r := b.Input.Rect().Translate(geometry.Point{X: b.Row.Rect().X, Y: b.Row.Rect().Y})
	BoxOf(gui.ResolveWidgetStyle(b.Input, "sourceview.find.input", "frame", state)).Paint(p, r)
}
