package sourceview

import (
	"testing"

	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

// Exercise the navigation calculation with pure mock typography, not simulated
// platform input. Window key dispatch is covered by tests/window/editor/shortcuts.
func TestNavigationCancelsProjectionBeforeReadingGeometry(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("abcdef")
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	v.context = &shapedContext{}
	t.Cleanup(v.dropLayouts)
	v.projection = textedit.NewProjection(v.document.model, v.Selection())
	v.projection.Update("p\nq", 1)
	resets := 0
	v.im = &resetContext{reset: func() { resets++ }}
	v.navigate(events.KeyEnd, false)
	if v.Selection() != (textedit.Selection{Anchor: 6, Caret: 6}) || v.projection != nil || resets != 1 {
		t.Fatal("navigation used temporary paragraph geometry", v.Selection(), resets)
	}
}

func TestNavigationKeepsStateChosenByResetCallback(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("abcdef")
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	v.projection = textedit.NewProjection(v.document.model, v.Selection())
	v.projection.Update("p\nq", 1)
	d := NewDocument("keep")
	v.im = &resetContext{reset: func() { v.SetDocument(d); v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2}) }}
	v.navigate(events.KeyEnd, false)
	if v.Document() != d || v.Selection() != (textedit.Selection{Anchor: 2, Caret: 2}) {
		t.Fatal("reset callback state overwritten", v.Selection())
	}
}
