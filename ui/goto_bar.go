package ui

import (
	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	base "github.com/golang-gui/goui/ui"
)

// GoToBar declares an optional floating companion in the target's Root.
func GoToBar(targetID string) *GoToBarView {
	v := &GoToBarView{targetID: targetID}
	v.Self = v
	return v
}

type GoToBarView struct {
	base.ViewBase[GoToBarView]
	targetID string
	query    *string
	open     *bool
	onOpen   func(bool)
	onError  func(error)
}

func (v *GoToBarView) Target(id string) *GoToBarView       { v.targetID = id; return v }
func (v *GoToBarView) Query(text string) *GoToBarView      { v.query = &text; return v }
func (v *GoToBarView) Open(open bool) *GoToBarView         { v.open = &open; return v }
func (v *GoToBarView) OnOpen(fn func(bool)) *GoToBarView   { v.onOpen = fn; return v }
func (v *GoToBarView) OnError(fn func(error)) *GoToBarView { v.onError = fn; return v }
func (v *GoToBarView) Build() base.View                    { return v }
func (v *GoToBarView) Mount(ctx base.BuildContext) gui.Widget {
	w := source.NewGoToBar()
	s := &findBarState{}
	s.handles = signal.Handles{w.ConnectOpen(func(open bool) {
		if s.onOpen != nil {
			s.onOpen(open)
		}
	}), w.ConnectError(func(err error) {
		if s.onError != nil {
			s.onError(err)
		}
	})}
	ctx.SetState(s)
	return w
}
func (v *GoToBarView) Update(ctx base.BuildContext, widget gui.Widget) {
	w, s := widget.(*source.GoToBar), ctx.State().(*findBarState)
	s.onOpen, s.onError = v.onOpen, v.onError
	ctx.AfterUpdate(func() {
		target, _ := gui.FindWidget(w.Root(), v.targetID).(*source.SourceView)
		w.SetTarget(target)
		s.handles.Block()
		defer s.handles.Unblock()
		if v.open != nil && w.Opened() != *v.open {
			if *v.open {
				if err := w.Open(); err != nil && s.onError != nil {
					s.onError(err)
				}
			} else {
				w.Close()
			}
		}
		if v.query != nil {
			w.SetQuery(*v.query)
		}
	})
}
func (v *GoToBarView) Unmount(ctx base.BuildContext, _ gui.Widget) {
	ctx.State().(*findBarState).handles.Disconnect()
}

var _ base.WidgetView = (*GoToBarView)(nil)
