package sourceview

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
)

// ShowFind opens the built-in find UI and focuses its query. A short,
// single-line editor selection seeds the query. It requires a mounted view.
func (v *SourceView) ShowFind() error { return v.showFind(false) }

// ShowReplace opens the built-in find UI with replacement controls visible.
// It otherwise follows ShowFind's selection and focus rules.
func (v *SourceView) ShowReplace() error { return v.showFind(true) }

func (v *SourceView) showFind(replace bool) error {
	if err := v.companionAvailable(); err != nil {
		return err
	}
	v.companionEpoch++
	epoch, state := v.companionEpoch, v.operationState()
	if v.goToBar != nil {
		v.goToBar.Close()
	}
	if !v.companionCurrent(epoch, state) {
		return ErrOperationChanged
	}
	if v.findBar == nil {
		bar := newFindBar()
		bar.SetTarget(v)
		bar.ConnectError(v.report)
		v.findBar = bar
		v.WidgetBase.AddChild(v, bar)
	}
	if !v.companionCurrent(epoch, state) {
		return ErrOperationChanged
	}
	err := v.findBar.Open(replace)
	if err == nil && !v.companionCurrent(epoch, state) {
		return ErrOperationChanged
	}
	return err
}

// ShowGoTo opens the built-in line/column input, initialized from the caret.
// It requires a mounted view and closes the find/replace UI.
func (v *SourceView) ShowGoTo() error {
	if err := v.companionAvailable(); err != nil {
		return err
	}
	v.companionEpoch++
	epoch, state := v.companionEpoch, v.operationState()
	if v.findBar != nil {
		v.findBar.Close()
	}
	if !v.companionCurrent(epoch, state) {
		return ErrOperationChanged
	}
	if v.goToBar == nil {
		bar := newGoToBar()
		bar.SetTarget(v)
		bar.ConnectError(v.report)
		v.goToBar = bar
		v.WidgetBase.AddChild(v, bar)
	}
	if !v.companionCurrent(epoch, state) {
		return ErrOperationChanged
	}
	err := v.goToBar.Open()
	if err == nil && !v.companionCurrent(epoch, state) {
		return ErrOperationChanged
	}
	return err
}

// HidePanel hides the built-in find/replace or go-to UI. Query and replacement
// text survive. Focus returns to the editor only when it was inside that UI.
func (v *SourceView) HidePanel() {
	v.companionEpoch++
	epoch := v.companionEpoch
	if v.findBar != nil {
		v.findBar.Close()
	}
	if v.companionEpoch != epoch {
		return
	}
	if v.goToBar != nil {
		v.goToBar.Close()
	}
}

func (v *SourceView) companionAvailable() error {
	if v.Destroyed() || !v.mounted || v.Root() == nil {
		return ErrNotMounted
	}
	if !gui.IsEnabled(v) {
		return ErrDisabled
	}
	if _, ok := v.Root().(gui.EventTarget); !ok {
		return ErrNotMounted
	}
	return nil
}

func (v *SourceView) companionCurrent(epoch uint64, state operationState) bool {
	return !v.Destroyed() && v.mounted && v.companionEpoch == epoch && state.valid(v)
}

func (v *SourceView) arrangeCompanions() {
	if v.Destroyed() {
		return
	}
	const margin float32 = 8
	available := geometry.Size{Width: max(0, v.viewport.Width-2*margin), Height: max(0, v.viewport.Height-2*margin)}
	arrange := func(bar interface {
		gui.Widget
		Destroyed() bool
	}) {
		if !bar.Visible() || bar.Destroyed() {
			return
		}
		size := bar.Measure(layout.Loose(available)).Size
		if v.Destroyed() || bar.Destroyed() {
			return
		}
		bar.Arrange(geometry.Rect(max(0, v.viewport.Width-size.Width-margin), min(margin, v.viewport.Height), size.Width, size.Height))
	}
	if v.findBar != nil {
		arrange(v.findBar)
	}
	if v.goToBar != nil {
		arrange(v.goToBar)
	}
}

// Cursor applies the editor's preferred pointer to the code area. Internal
// controls resolve their own cursors without inheriting the code's text cursor.
func (v *SourceView) Cursor() gui.Cursor {
	if !v.dragging && (v.findBar != nil && v.findBar.Visible() && v.findBar.pointer.ContainsHover() ||
		v.goToBar != nil && v.goToBar.Visible() && v.goToBar.pointer.ContainsHover()) {
		return nil
	}
	return v.WidgetBase.Cursor()
}
