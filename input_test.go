package sourceview

import (
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

func TestWordPointerAndDragUnits(t *testing.T) {
	v, _ := colorFixture(t)
	v.Document().Model().SetText("alpha beta\nlast\n")
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	point := func(x float32) geometry.Point { return geometry.Point{X: v.gutter + padding + x, Y: padding + 10} }
	// Last half of the final 'a' still selects alpha, not the following space.
	if got := v.wordSelection(point(48)); got != (textedit.Range{Start: 0, End: 5}) {
		t.Fatal(got)
	}
	v.dragging, v.dragUnit, v.dragRange = true, 1, textedit.Range{Start: 0, End: 5}
	v.setSelection(textedit.Selection{Caret: 5}, false, false)
	v.dragSelection(point(48))
	if v.Selection() != (textedit.Selection{Caret: 5}) {
		t.Fatal("release shrank word", v.Selection())
	}
	v.dragSelection(point(85))
	if v.Selection() != (textedit.Selection{Caret: 10}) {
		t.Fatal("word drag", v.Selection())
	}
	v.dragRange = textedit.Range{Start: 6, End: 10}
	v.dragSelection(point(18))
	if v.Selection() != (textedit.Selection{Anchor: 10, Caret: 0}) {
		t.Fatal("backward word drag", v.Selection())
	}
	v.dragUnit, v.dragRange = 2, v.lineSelection(2)
	v.dragSelection(point(10))
	if v.Selection().Range() != (textedit.Range{Start: 0, End: 11}) {
		t.Fatal("line release lost LF", v.Selection())
	}
	if got := v.lineSelection(v.Document().Model().Len()); got.Start != got.End {
		t.Fatal("last empty line", got)
	}
}

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
