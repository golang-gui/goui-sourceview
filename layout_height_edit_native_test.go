package sourceview

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestNativeLocalEditRetainsHeightUntilMeasurement(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	ctx := &widthCacheNativeContext{Context: native}
	v := NewSourceView()
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	v.Document().Model().SetText(strings.Repeat("text 中文 العربية e\u0301 ffi 👩‍💻 ", 300))
	observe(t, v)
	v.Document().engine.Advance(time.Time{})
	v.context = ctx
	v.frameLayout = true
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	settle := func() {
		t.Helper()
		for range 8 {
			scroll.Arrange(geometry.Rect(0, 0, 310, 240))
			v.layoutStep(time.Time{})
		}
		if v.layoutPending {
			t.Fatal("layout did not settle")
		}
	}
	settle()
	height, before := v.ContentSize().Height, ctx.created
	if height <= 240 {
		t.Fatal("fixture must have a vertical scrollbar")
	}
	if err := v.Document().Model().ReplaceAtomic(textedit.Range{}, "changed "); err != nil {
		t.Fatal(err)
	}
	if v.ContentSize().Height != height || ctx.created != before {
		t.Fatal("local edit collapsed the scrolling estimate or synchronously shaped text")
	}
	settle()
	if ctx.created != before+1 {
		t.Fatal("local edit measured an unnecessary intermediate scrollbar width", ctx.created-before)
	}
	if p := v.paragraph(0); p.text != v.Document().Model().Text() || p.height != v.heights.Total() {
		t.Fatal("measured replacement did not replace the estimated height")
	}
	if err := v.Document().Model().ReplaceAtomic(textedit.Range{End: v.Document().Model().Len()}, ""); err != nil {
		t.Fatal(err)
	}
	settle()
	if v.ContentSize().Height > 240 || scroll.ScrollY() != 0 || v.paragraph(0).text != "" {
		t.Fatal("deleting the long paragraph retained the estimated scrollbar")
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("local editing leaked native layouts")
	}
}
