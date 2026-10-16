package sourceview

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"testing"
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
