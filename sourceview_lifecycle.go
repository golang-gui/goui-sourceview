package sourceview

import (
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

func (v *SourceView) changed(c textedit.Change, splices []highlight.Splice, reset bool) {
	defer v.searchState.Emit()
	epoch, d := v.mountEpoch, v.document
	v.observed = c.Revision
	v.reveal = nil
	v.closeCompletion()
	if v.document != d || v.mountEpoch != epoch || !v.mounted {
		return
	}
	hadHiddenRows := len(v.hiddenLines) > 0
	v.mapFolds(c, splices)
	v.mapParagraphs(splices)
	v.mapPairs(c)
	v.mapSearchRange(c)
	if c.Reset {
		v.anchor.valid = false
		v.pendingAnchor = false
	} else if v.anchor.valid && v.anchorRevision < c.Revision {
		for _, edit := range c.Edits {
			v.anchor.offset = textedit.MapOffset(v.anchor.offset, edit)
		}
		v.anchorRevision = c.Revision
		v.pendingAnchor = true
	}
	v.cancelPreedit()
	if v.document != d || v.mountEpoch != epoch || !v.mounted {
		return
	}
	v.cancelPaste()
	v.currentMatch = nil
	v.updateMenu()
	old := v.selection
	if v.selectionRevision < c.Revision {
		if c.Reset {
			v.selection = textedit.Selection{}
		} else {
			for _, e := range c.Edits {
				v.selection.Anchor = textedit.MapOffset(v.selection.Anchor, e)
				v.selection.Caret = textedit.MapOffset(v.selection.Caret, e)
			}
		}
		v.selectionRevision = c.Revision
	}
	if reset || c.Reset || hadHiddenRows {
		v.invalidateLayout()
	} else {
		for _, s := range splices {
			estimate := v.lineHeight
			if s.Removed == 1 && s.Added == 1 {
				// A local edit invalidates geometry, not the previous scrolling
				// estimate. Keeping it avoids hiding the scrollbar and laying
				// out the replacement at an unnecessary intermediate width.
				estimate = v.heights.Top(s.Index+1) - v.heights.Top(s.Index)
			}
			v.heights.Splice(s.Index, s.Removed, s.Added, estimate)
		}
		// With no collapsed ranges, logical/display rows still correspond
		// directly. Update their count without throwing away spliced heights.
		v.visibleLines.reset(v.displayCount(), nil)
		v.contentWidth = 0
		// Stable line IDs retain layouts of untouched suffixes; changed paragraphs
		// receive new IDs and are evicted by the bounded view-local cache.
		v.RequestLayout()
		v.RequestPaint()
	}
	if c.Reset {
		v.offset = geometry.Point{}
		v.scrollSignal.Emit(geometry.Rect(0, 0, v.viewport.Width, v.viewport.Height))
	}
	if v.mountEpoch != epoch || v.document != d || !v.mounted {
		return
	}
	if old != v.selection {
		v.selectionSignal.Emit(v.Selection())
	}
	if v.mounted && v.mountEpoch == epoch && v.document == d {
		v.changeSignal.Emit(c)
	}
}

func (v *SourceView) mount() {
	request := v.reveal
	if request != nil && !request.valid(v) {
		v.reveal = nil
	}
	v.mounted = true
	v.mountEpoch++
	if v.reveal != nil {
		// A selection requested before first allocation still needs revealing.
		*v.reveal = v.operationState()
	}
	if win := v.Window(); win != nil {
		v.windowFocus = win.ConnectFocus(func(focused bool) {
			if !focused {
				v.closeCompletion()
			}
			v.RequestPaint()
		})
	}
	if v.observed != v.document.model.Revision() {
		v.searchRange = nil
		if v.search != nil {
			v.search.SetRange(nil)
		}
		v.anchor.valid = false
		v.pendingAnchor = false
		v.offset = geometry.Point{}
	}
	if v.foldRevision != v.document.model.Revision() || v.foldDefinition != v.document.definition {
		v.clearFolds()
	}
	v.context = gui.App.Typography()
	state := v.operationState()
	v.document.acquire(v)
	if !state.valid(v) || !v.mounted || v.Destroyed() {
		return
	}
	attachFrame(v)
	v.updateScheme()
	v.invalidateLayout()
	v.blink = gui.App.NewTimer()
	v.blinkHandle = v.blink.ConnectTimeout(func() {
		if !v.mounted || !activeFocus(v) || !visible(v) {
			v.blink.Stop()
			return
		}
		v.caretOn = !v.caretOn
		if v.dragging {
			v.dragSelection(v.dragPoint)
		}
		v.RequestPaint()
	})
	v.resetBlink()
}

func (v *SourceView) unmount() {
	v.mounted = false
	v.mountEpoch++
	v.reveal = nil
	v.cancelPaste()
	v.hideMenu()
	v.closeCompletion()
	if v.completionQueue != nil {
		v.completionQueue.owner = nil
	}
	if v.windowFocus != nil {
		v.windowFocus.Disconnect()
		v.windowFocus = nil
	}
	if v.menu != nil && v.menu.language != nil {
		v.menu.language.Disconnect()
		v.menu.language = nil
	}
	v.dragging = false
	v.pairs = nil
	v.brackets = bracketState{}
	v.tagMatches = tagMatchState{}
	if v.blink != nil {
		v.blink.Stop()
		v.blinkHandle.Disconnect()
		v.blink = nil
	}
	detachFrame(v)
	v.document.release(v)
	v.cancelPreedit()
	v.dropLayouts()
	v.context = nil
}

func (v *SourceView) resetBlink() {
	v.caretOn = true
	if v.blink == nil {
		return
	}
	if activeFocus(v) && visible(v) {
		_ = v.blink.Start(500 * time.Millisecond)
	} else {
		v.blink.Stop()
	}
}

func visible(v gui.Widget) bool {
	for w := v; w != nil; w = w.Parent() {
		if !w.Visible() {
			return false
		}
	}
	return true
}

func activeFocus(v *SourceView) bool {
	return v.Focused() && (v.Window() == nil || v.Window().Focused() || v.menu != nil && v.menu.popup.Visible())
}
