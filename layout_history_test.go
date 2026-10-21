package sourceview

import (
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestParagraphHistoryReusesExactTextMeasurements(t *testing.T) {
	v, ctx := widthCacheFixture(t)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	original := v.paragraph(0)
	if err := v.Document().Model().ReplaceAtomic(textedit.Range{}, "x"); err != nil {
		t.Fatal(err)
	}
	edited := v.paragraph(0)
	if edited == original {
		t.Fatal("changed text reused stale geometry")
	}
	created := ctx.created
	for range 5 {
		if !v.Undo() || v.paragraph(0) != original {
			t.Fatal("undo remeasured identical text")
		}
		if !v.Redo() || v.paragraph(0) != edited {
			t.Fatal("redo remeasured identical text")
		}
	}
	if ctx.created != created {
		t.Fatal("history created new native resources", created, ctx.created)
	}
	v.SetScheme(scheme.Dark())
	if !v.Undo() || v.paragraph(0) != original {
		t.Fatal("theme change discarded text measurement reuse")
	}
	for _, c := range original.layout.(*shapedLayout).colors {
		if c != v.colors.Foreground {
			t.Fatal("restored paragraph retained stale colors")
		}
	}
}

func TestRestoredTextDoesNotShareLiveParagraphResources(t *testing.T) {
	v, _ := layoutWorkFixture(t, 2)
	v.Document().Model().SetText("same\nsame")
	v.Document().engine.Advance(time.Time{})
	v.frameLayout = false
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	a, b := v.paragraph(0), v.paragraph(1)
	if a == b || a.layout == b.layout {
		t.Fatal("equal live paragraphs share a mutable native object")
	}
	if err := v.Document().Model().ReplaceAtomic(textedit.Range{}, "x"); err != nil {
		t.Fatal(err)
	}
	v.paragraph(0)
	if !v.Undo() || v.paragraph(0) != a || v.paragraph(1) != b {
		t.Fatal("restoring one paragraph stole another live paragraph's layout")
	}
	if a == b || a.layout == b.layout {
		t.Fatal("restoration introduced resource aliasing")
	}
}

func TestRestoredParagraphStopsAfterRecolorReentry(t *testing.T) {
	v, ctx := widthCacheFixture(t)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	original := v.paragraph(0)
	if err := v.Document().Model().ReplaceAtomic(textedit.Range{}, "x"); err != nil {
		t.Fatal(err)
	}
	v.paragraph(0)
	v.SetScheme(scheme.Dark())
	original.layout.(*shapedLayout).onColor = func() {
		v.SetDocument(NewDocument("replacement during restored recolor"))
	}
	if !v.Document().Model().Undo() {
		t.Fatal("fixture undo failed")
	}
	if got := v.paragraph(0); got != nil || v.Document().Model().Text() != "replacement during restored recolor" {
		t.Fatal("restored recolor returned a released native object")
	}
	if ctx.created != ctx.destroyed {
		t.Fatal("restored recolor leaked native ownership", ctx.created, ctx.destroyed)
	}
}
