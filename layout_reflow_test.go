package sourceview

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestEditCancelsReflowForRetainedLineIDs(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("prefix\n" + strings.Repeat("a", 8192) + "\ntail")
	observe(t, v)
	v.Document().engine.Advance(time.Time{})
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	p := v.paragraph(1)
	if p == nil {
		t.Fatal("missing retained paragraph")
	}
	cancelled := 0
	p.cancelReflow = func() { cancelled++ }
	if err := v.Document().Model().Replace(textedit.Range{}, "inserted\n"); err != nil {
		t.Fatal(err)
	}
	if cancelled != 1 || p.cancelReflow != nil {
		t.Fatal("line splice retained a callback bound to the previous row")
	}
	if p.layout.(*shapedLayout).dead {
		t.Fatal("line splice discarded unchanged text measurements")
	}
}
