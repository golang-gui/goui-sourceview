package sourceview

import (
	"bytes"
	"image"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestNativeParagraphHistoryPreservesCompleteBidiLayout(t *testing.T) {
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
	text := "abc \u202e" + strings.Repeat("中文 e\u0301 👩‍💻 ffi abc 123 ", 300) + "אבג\u202c العربية end"
	v.Document().Model().SetText(text)
	observe(t, v)
	v.Document().engine.Advance(time.Time{})
	v.context = ctx
	defer v.dropLayouts()
	v.LayoutVisible(geometry.Size{Width: 310, Height: 200}, geometry.Point{})
	original := v.paragraph(0)
	wantLines, wantClusters := original.fragment.Metrics()
	region := image.Rect(0, 0, 310, 200)
	want, err := original.layout.Rasterize(1, region, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantPixels := bytes.Clone(want.Pixels)
	if err := v.Document().Model().ReplaceAtomic(textedit.Range{}, "x"); err != nil {
		t.Fatal(err)
	}
	v.paragraph(0)
	before := *ctx
	if !v.Undo() || v.paragraph(0) != original || ctx.created != before.created || ctx.sizes != before.sizes || ctx.metrics != before.metrics {
		t.Fatal("restoring identical complex text performed native remeasurement")
	}
	lines, clusters := original.fragment.Metrics()
	if !reflect.DeepEqual(lines, wantLines) || !reflect.DeepEqual(clusters, wantClusters) {
		t.Fatal("history changed ordinary measured values")
	}
	got, err := original.layout.Rasterize(1, region, nil)
	if err != nil || !bytes.Equal(got.Pixels, wantPixels) {
		t.Fatal("history changed native pixels", err)
	}
	w, h := original.layout.Size()
	fresh, err := native.NewTextLayout(text, original.layout.Format(), w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Destroy()
	freshLines, freshClusters := copyMetrics(fresh)
	if !reflect.DeepEqual(lines, freshLines) || !reflect.DeepEqual(clusters, freshClusters) {
		t.Fatal("reused bidi context differs from a fresh complete native layout")
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("history cache leaked or aliased native ownership", ctx.created, ctx.destroyed)
	}
}
