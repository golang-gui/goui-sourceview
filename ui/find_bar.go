package ui

import (
	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	base "github.com/golang-gui/goui/ui"
)

// FindBar declares the optional companion for the SourceView with targetID in
// the same Root. The explicit reference is resolved after each tree update.
func FindBar(targetID string) *FindBarView {
	v := &FindBarView{targetID: targetID}
	v.Self = v
	return v
}

type FindBarView struct {
	base.ViewBase[FindBarView]
	targetID          string
	replacement       *string
	open, replaceMode *bool
	onOpen            func(bool)
	onError           func(error)
}

func (v *FindBarView) Target(id string) *FindBarView         { v.targetID = id; return v }
func (v *FindBarView) Replacement(text string) *FindBarView  { v.replacement = &text; return v }
func (v *FindBarView) Open(open bool) *FindBarView           { v.open = &open; return v }
func (v *FindBarView) ReplaceMode(replace bool) *FindBarView { v.replaceMode = &replace; return v }
func (v *FindBarView) OnOpen(fn func(bool)) *FindBarView     { v.onOpen = fn; return v }
func (v *FindBarView) OnError(fn func(error)) *FindBarView   { v.onError = fn; return v }
func (v *FindBarView) Build() base.View                      { return v }

type findBarState struct {
	onOpen  func(bool)
	onError func(error)
	handles signal.Handles
}

func (v *FindBarView) Mount(ctx base.BuildContext) gui.Widget {
	w := source.NewFindBar()
	s := &findBarState{}
	s.handles = signal.Handles{
		w.ConnectOpen(func(open bool) {
			if s.onOpen != nil {
				s.onOpen(open)
			}
		}),
		w.ConnectError(func(err error) {
			if s.onError != nil {
				s.onError(err)
			}
		}),
	}
	ctx.SetState(s)
	return w
}
func (v *FindBarView) Update(ctx base.BuildContext, widget gui.Widget) {
	w, s := widget.(*source.FindBar), ctx.State().(*findBarState)
	if v.replacement != nil {
		w.SetReplacement(*v.replacement)
	}
	if v.replaceMode != nil {
		w.SetReplaceMode(*v.replaceMode)
	}
	s.onOpen, s.onError = v.onOpen, v.onError
	ctx.AfterUpdate(func() {
		target, _ := gui.FindWidget(w.Root(), v.targetID).(*source.SourceView)
		w.SetTarget(target)
		if v.open != nil && w.Opened() != *v.open {
			s.handles.Block()
			defer s.handles.Unblock()
			if *v.open {
				if err := w.Open(w.ReplaceMode()); err != nil && s.onError != nil {
					s.onError(err)
				}
			} else {
				w.Close()
			}
		}
	})
}
func (v *FindBarView) Unmount(ctx base.BuildContext, _ gui.Widget) {
	s := ctx.State().(*findBarState)
	s.handles.Disconnect()
}

var _ base.WidgetView = (*FindBarView)(nil)
