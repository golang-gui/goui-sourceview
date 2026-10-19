package sourceview

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/mlang"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/style"
)

// GoToBar is an optional floating line/column input. Mount it in an Overlay in
// the editor's Root. It borrows its target and does not own document state.
type GoToBar struct {
	gui.WidgetBase
	target        *SourceView
	surface       *gotoSurface
	row           *gui.LinearBox
	input         *gui.TextInput
	status        *gui.Label
	close         *gui.Button
	keys          *gui.KeyEventController
	lifetime      signal.Handles
	attached      bool
	language      signal.Handle
	mounted, open bool
	epoch         uint64
	openSignal    signal.Signal1[bool]
	errorSignal   signal.Signal1[error]
}

func NewGoToBar() *GoToBar {
	b := &GoToBar{}
	b.SetLayoutManager(layout.NewFillLayout())
	b.surface = &gotoSurface{bar: b}
	l := layout.NewLinearLayout(layout.DirectionVertical)
	l.Padding = 12
	l.Spacing = 4
	l.CrossAlign = layout.CrossStretch
	b.surface.SetLayoutManager(l)
	b.surface.SetVisible(false)
	b.WidgetBase.AddChild(b, b.surface)
	b.row = gui.NewLinearBox(layout.DirectionHorizontal)
	b.row.SetSpacing(4)
	b.input = gui.NewTextInput()
	b.input.SetStyleName("sourceview.find.input")
	b.input.SetPadding(4)
	b.input.SetMainWeight(1)
	b.input.ConnectText(func(string) { b.refresh() })
	b.input.ConnectSubmit(b.submit)
	b.close = gui.NewButton()
	b.close.SetStyleName("sourceview.find.action")
	b.close.SetPadding(3)
	icon := gui.NewIcon(findIcons["close"])
	icon.SetStyleName("sourceview.find.icon")
	b.close.SetChild(icon)
	b.close.ConnectClicked(b.Close)
	b.row.AddChild(b.input)
	b.row.AddChild(b.close)
	b.status = gui.NewLabel("")
	b.status.SetStyleName("sourceview.find.status")
	b.surface.WidgetBase.AddChild(b.surface, b.row)
	b.surface.WidgetBase.AddChild(b.surface, b.status)
	b.keys = gui.NewKeyEventController()
	b.keys.SetPhase(gui.PhaseCapture)
	b.keys.ConnectKeyDown(b.key)
	keys := gui.NewKeyEventController()
	keys.SetPhase(gui.PhaseCapture)
	keys.ConnectKeyDown(b.key)
	b.AddEventController(keys)
	b.ConnectMount(func() { b.mounted = true; b.connect(); b.language = mlang.ConnectChanged(b.refresh); b.refresh() })
	b.ConnectUnmount(func() {
		b.mounted = false
		b.open = false
		b.surface.SetVisible(false)
		b.epoch++
		b.disconnect()
		if b.language != nil {
			b.language.Disconnect()
			b.language = nil
		}
	})
	b.refresh()
	return b
}
func (b *GoToBar) Target() *SourceView { return b.target }
func (b *GoToBar) SetTarget(v *SourceView) {
	if b.Destroyed() || v == b.target {
		return
	}
	expected := b.epoch
	if b.open {
		expected++
	}
	b.Close()
	if b.Destroyed() || b.epoch != expected {
		return
	}
	b.disconnect()
	b.target = v
	b.epoch++
	if b.mounted {
		b.connect()
	}
}
func (b *GoToBar) Query() string { return b.input.Text() }
func (b *GoToBar) SetQuery(text string) {
	if !b.Destroyed() {
		b.input.SetText(text)
	}
}
func (b *GoToBar) Opened() bool { return b.open }
func (b *GoToBar) ConnectError(fn func(error)) signal.Handle {
	return b.errorSignal.Connect(func(err error) {
		if !b.Destroyed() {
			fn(err)
		}
	})
}
func (b *GoToBar) ConnectOpen(fn func(bool)) signal.Handle {
	return b.openSignal.Connect(func(open bool) {
		if !b.Destroyed() && b.open == open {
			fn(open)
		}
	})
}
func (b *GoToBar) liveTarget() *SourceView {
	v := b.target
	if b.Destroyed() || !b.mounted || v == nil || v.Destroyed() || !v.mounted || v.Root() != b.Root() || b.Root() == nil {
		return nil
	}
	return v
}
func (b *GoToBar) Open() error {
	v := b.liveTarget()
	if v == nil {
		return ErrNotMounted
	}
	host, ok := b.Root().(gui.EventTarget)
	if !ok {
		return ErrNotMounted
	}
	state, epoch := v.operationState(), b.epoch
	v.closeCompletion()
	v.cancelPreedit()
	if !state.valid(v) || b.epoch != epoch || b.liveTarget() != v {
		return ErrOperationChanged
	}
	m := v.document.model
	at := v.Selection().Caret
	line := m.LineAt(at)
	r, _ := m.LineRange(line)
	prefix, _ := m.Slice(textedit.Range{Start: r.Start, End: at})
	b.input.SetText(fmt.Sprintf("%d:%d", line+1, utf8.RuneCountInString(prefix)+1))
	changed := !b.open
	b.open = true
	b.surface.SetVisible(true)
	b.refresh()
	host.SetFocusedWidget(b.input)
	if b.Destroyed() || b.epoch != epoch || !b.open {
		return nil
	}
	b.input.SetSelection(textedit.Selection{Caret: len(b.input.Text())})
	if changed {
		b.openSignal.Emit(true)
	}
	return nil
}
func (b *GoToBar) Close() {
	if b.Destroyed() || !b.open {
		return
	}
	focus := b.ContainsFocus()
	b.open = false
	b.epoch++
	b.surface.SetVisible(false)
	if focus {
		if v := b.liveTarget(); v != nil && visible(v) {
			if host, ok := b.Root().(gui.EventTarget); ok {
				host.SetFocusedWidget(v)
			}
		}
	}
	if !b.Destroyed() && !b.open {
		b.openSignal.Emit(false)
	}
}
func (b *GoToBar) connect() {
	if b.target == nil || b.target.Destroyed() {
		return
	}
	b.lifetime = signal.Handles{b.target.ConnectMount(b.attach), b.target.ConnectUnmount(func() { b.detach(); b.Close() }), b.target.searchState.Connect(b.refresh)}
	b.attach()
}
func (b *GoToBar) attach() {
	if b.attached || b.liveTarget() == nil {
		return
	}
	b.target.AddEventController(b.keys)
	b.attached = true
}
func (b *GoToBar) detach() {
	if b.target != nil {
		b.target.RemoveEventController(b.keys)
	}
	b.attached = false
}
func (b *GoToBar) disconnect() { b.detach(); b.lifetime.Disconnect(); b.lifetime = nil }
func (b *GoToBar) key(ctx gui.EventContext, e events.KeyEvent) {
	if b.liveTarget() == nil || !visible(b) || !gui.IsEnabled(b) {
		return
	}
	control, _ := gui.ModControl.Resolve()
	if e.Key == events.KeyG && e.Modifiers == control {
		if err := b.Open(); err != nil {
			b.errorSignal.Emit(err)
		}
		e.PreventDefault()
		ctx.StopPropagation()
		return
	}
	if !b.open || !b.ContainsFocus() {
		return
	}
	if e.Key == events.KeyEscape && e.Modifiers == 0 {
		b.Close()
		e.PreventDefault()
		ctx.StopPropagation()
	}
}
func (b *GoToBar) refresh() {
	if b.Destroyed() {
		return
	}
	b.input.SetName(resource.GoTo())
	b.input.SetPlaceholder(resource.GoToPlaceholder())
	b.close.SetName(resource.Close())
	message := resource.GoToPlaceholder()
	name := "sourceview.find.status"
	if v := b.liveTarget(); v != nil && b.input.Text() != "" {
		if _, err := gotoOffset(v.document.model, b.input.Text()); err != nil {
			message = resource.InvalidPosition()
			name = "sourceview.find.error"
		}
	}
	b.status.SetText(message)
	b.status.SetStyleName(name)
}
func (b *GoToBar) submit() {
	v := b.liveTarget()
	if v == nil {
		return
	}
	at, err := gotoOffset(v.document.model, b.input.Text())
	if err != nil {
		b.refresh()
		return
	}
	d, epoch := v.document, b.epoch
	v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
	if !b.Destroyed() && b.epoch == epoch && b.liveTarget() == v && v.document == d {
		b.Close()
	}
}
func gotoOffset(m *textedit.Model, query string) (int, error) {
	parts := strings.Split(strings.TrimSpace(query), ":")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, fmt.Errorf("sourceview: invalid line/column")
	}
	line, err := strconv.Atoi(parts[0])
	if err != nil || line < 1 || line > m.LineCount() {
		return 0, fmt.Errorf("sourceview: line out of range")
	}
	column := 1
	if len(parts) == 2 {
		column, err = strconv.Atoi(parts[1])
		if err != nil || column < 1 {
			return 0, fmt.Errorf("sourceview: column out of range")
		}
	}
	r, _ := m.LineRange(line - 1)
	text, _ := m.Slice(r)
	at := 0
	for n := 1; n < column; n++ {
		if at == len(text) {
			return 0, fmt.Errorf("sourceview: column out of range")
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		at += size
	}
	return r.Start + at, nil
}
func (b *GoToBar) Snapshot() gui.WidgetInfo {
	info := b.WidgetBase.Snapshot()
	info.Role = "sourceview-goto-bar"
	info.SetAttribute("goui.sourceview.goto", map[string]any{"open": b.open})
	return info
}

type gotoSurface struct {
	gui.WidgetBase
	inset float32
	bar   *GoToBar
}

func (s *gotoSurface) Measure(c layout.Constraint) layout.Measurement {
	if !s.Visible() {
		return layout.Measurement{}
	}
	s.inset = max(1, boxOf(gui.ResolveWidgetStyle(s, "sourceview.find", "", style.Normal)).outset()+1)
	s.LayoutManager().(*layout.LinearLayout).Padding = s.inset + 6
	width := min(float32(360), c.Max.Width)
	c.Min.Width = width
	c.Max.Width = width
	return s.WidgetBase.Measure(c)
}
func (s *gotoSurface) Paint(p gui.Painter) {
	boxOf(gui.ResolveWidgetStyle(s, "sourceview.find", "", style.Normal)).paint(p, geometry.Rect(0, 0, s.Rect().Width, s.Rect().Height).Inset(s.inset))
	b := s.bar
	state := style.Normal
	if b.input.Focused() {
		state = style.Focused
	}
	r := b.input.Rect().Translate(geometry.Point{X: b.row.Rect().X, Y: b.row.Rect().Y})
	boxOf(gui.ResolveWidgetStyle(b.input, "sourceview.find.input", "frame", state)).paint(p, r)
}
