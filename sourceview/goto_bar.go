package sourceview

import (
	"fmt"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/controls"
	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/mlang"
	"github.com/golang-gui/goui/platform/events"
)

// goToBar is the editor-owned line/column input.
type goToBar struct {
	gui.WidgetBase
	pointer *gui.MotionEventController
	target  *SourceView
	*controls.GoToUI
	lifetime      signal.Handles
	language      signal.Handle
	mounted, open bool
	epoch         uint64
	openSignal    signal.Signal1[bool]
	errorSignal   signal.Signal1[error]
}

func newGoToBar() *goToBar {
	b := &goToBar{}
	b.SetLayoutManager(layout.NewFillLayout())
	b.SetVisible(false)
	b.pointer = gui.NewMotionEventController()
	b.AddEventController(b.pointer)
	b.GoToUI = controls.NewGoToUI(b.refresh, b.submit, b.Close)
	b.WidgetBase.AddChild(b, b.Content)
	keys := gui.NewKeyEventController()
	keys.SetPhase(gui.PhaseCapture)
	keys.ConnectKeyDown(b.key)
	b.AddEventController(keys)
	b.ConnectMount(func() { b.mounted = true; b.connect(); b.language = mlang.ConnectChanged(b.refresh); b.refresh() })
	b.ConnectUnmount(func() {
		b.mounted = false
		b.open = false
		b.Content.SetVisible(false)
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
func (b *goToBar) Target() *SourceView { return b.target }
func (b *goToBar) SetTarget(v *SourceView) {
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
func (b *goToBar) Query() string { return b.Input.Text() }
func (b *goToBar) SetQuery(text string) {
	if !b.Destroyed() {
		b.Input.SetText(text)
	}
}
func (b *goToBar) Opened() bool { return b.open }
func (b *goToBar) ConnectError(fn func(error)) signal.Handle {
	return b.errorSignal.Connect(func(err error) {
		if !b.Destroyed() {
			fn(err)
		}
	})
}
func (b *goToBar) ConnectOpen(fn func(bool)) signal.Handle {
	return b.openSignal.Connect(func(open bool) {
		if !b.Destroyed() && b.open == open {
			fn(open)
		}
	})
}
func (b *goToBar) liveTarget() *SourceView {
	v := b.target
	if b.Destroyed() || !b.mounted || v == nil || v.Destroyed() || !v.mounted || v.Root() != b.Root() || b.Root() == nil {
		return nil
	}
	return v
}
func (b *goToBar) Open() error {
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
	prefix, _ := m.Slice(TextRange{Start: r.Start, End: at})
	b.Input.SetText(fmt.Sprintf("%d:%d", line+1, utf8.RuneCountInString(prefix)+1))
	changed := !b.open
	b.open = true
	b.SetVisible(true)
	b.Content.SetVisible(true)
	b.refresh()
	host.SetFocusedWidget(b.Input)
	if b.Destroyed() || b.epoch != epoch || !b.open {
		return nil
	}
	b.Input.SetSelection(gui.TextSelection{Caret: len(b.Input.Text())})
	if changed {
		b.openSignal.Emit(true)
	}
	return nil
}
func (b *goToBar) Close() {
	if b.Destroyed() || !b.open {
		return
	}
	focus := b.ContainsFocus()
	b.open = false
	b.epoch++
	b.Content.SetVisible(false)
	b.SetVisible(false)
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
func (b *goToBar) connect() {
	if b.liveTarget() != nil {
		b.lifetime = signal.Handles{b.target.searchState.Connect(b.refresh)}
	}
}
func (b *goToBar) disconnect() { b.lifetime.Disconnect(); b.lifetime = nil }
func (b *goToBar) key(ctx gui.EventContext, e events.KeyEvent) {
	if b.liveTarget() == nil || !visible(b) || !gui.IsEnabled(b) {
		return
	}
	command, bound := b.liveTarget().keyCommand(e)
	if bound && command == CommandGoTo {
		if err := b.target.ShowGoTo(); err != nil {
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
func (b *goToBar) refresh() {
	if b.Destroyed() {
		return
	}
	b.Input.SetName(resource.GoTo())
	b.Input.SetPlaceholder(resource.GoToPlaceholder())
	closeText := resource.Close()
	b.CloseButton.SetName(closeText)
	b.CloseButton.SetTooltip(closeText)
	message := resource.GoToPlaceholder()
	name := "sourceview.find.status"
	if v := b.liveTarget(); v != nil && b.Input.Text() != "" {
		if _, err := editor.GoToOffset(v.document.model, b.Input.Text()); err != nil {
			message = resource.InvalidPosition()
			name = "sourceview.find.error"
		}
	}
	b.Status.SetText(message)
	b.Status.SetStyleName(name)
}
func (b *goToBar) submit() {
	v := b.liveTarget()
	if v == nil {
		return
	}
	at, err := editor.GoToOffset(v.document.model, b.Input.Text())
	if err != nil {
		b.refresh()
		return
	}
	d, epoch := v.document, b.epoch
	v.SetSelection(TextSelection{Anchor: at, Caret: at})
	if !b.Destroyed() && b.epoch == epoch && b.liveTarget() == v && v.document == d {
		b.Close()
	}
}

func (b *goToBar) Snapshot() gui.WidgetInfo {
	info := b.WidgetBase.Snapshot()
	info.Role = "sourceview-goto-bar"
	info.SetAttribute("goui.sourceview.goto", map[string]any{"open": b.open})
	return info
}
