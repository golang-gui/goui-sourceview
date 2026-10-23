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
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestNativeRecentWidthsKeepMetricsAndPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	ctx := &countedNativeContext{Context: native}
	v := NewSourceView()
	v.context = ctx
	defer v.dropLayouts()
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	text := "start \u202e" + strings.Repeat("abcd 1234 中文 e\u0301 ffi 👩‍💻\t ", 120) + "XYZ\u202c עברית العربية end"
	v.Document().Model().SetText(text)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	a := v.paragraph(0)
	if a == nil || a.fragment.LineCount() < 64 {
		t.Fatal("missing wrapped native fixture")
	}
	wantLines, wantClusters := copyMetrics(a.layout)
	wantCaret := a.geometry.Caret(textedit.Position{Offset: strings.Index(text, "XYZ")})
	region := image.Rect(0, 0, 220, 120)
	wantBitmap, err := a.layout.Rasterize(1, region, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantPixels := bytes.Clone(wantBitmap.Pixels)
	v.LayoutVisible(geometry.Size{Width: 310, Height: 200}, geometry.Point{})
	if p := v.paragraph(0); p == nil || p == a {
		t.Fatal("second width did not create independent native metrics")
	}
	before := *ctx
	v.LayoutVisible(geometry.Size{Width: 220, Height: 280}, geometry.Point{})
	if p := v.paragraph(0); p != a || *ctx != before {
		t.Fatal("restored width performed native measurement/formatting", before, *ctx)
	}
	gotLines, gotClusters := a.fragment.Metrics()
	if !reflect.DeepEqual(gotLines, wantLines) || !reflect.DeepEqual(gotClusters, wantClusters) || a.geometry.Caret(textedit.Position{Offset: strings.Index(text, "XYZ")}) != wantCaret {
		t.Fatal("restored metrics or caret changed")
	}
	gotBitmap, err := a.layout.Rasterize(1, region, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotBitmap.Width != wantBitmap.Width || gotBitmap.Height != wantBitmap.Height || gotBitmap.Stride != wantBitmap.Stride || !bytes.Equal(gotBitmap.Pixels, wantPixels) {
		t.Fatal("restoring width changed native pixels")
	}
	// Independent native input at this width remains the authority, including
	// bidi context. This is no substring-shaping approximation.
	boxWidth, boxHeight := a.layout.Size()
	fresh, err := native.NewTextLayout(text, a.layout.Format(), boxWidth, boxHeight)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Destroy()
	fullLines, fullClusters := copyMetrics(fresh)
	if !reflect.DeepEqual(gotLines, fullLines) || !reflect.DeepEqual(gotClusters, fullClusters) {
		t.Fatal("restored values differ from an independent complete layout")
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("native cache leaked layouts", ctx.created, ctx.destroyed)
	}
}

func TestNativeRecentWidthsWithScrollView(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	ctx := &countedNativeContext{Context: native}
	v := NewSourceView()
	v.context = ctx
	defer v.dropLayouts()
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	v.Document().Model().SetText(strings.Repeat("one two three four five ", 300))
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	for _, width := range []float32{220, 310} {
		scroll.Arrange(geometry.Rect(0, 0, width, 240))
	}
	before := *ctx
	scroll.Arrange(geometry.Rect(0, 0, 220, 240))
	if *ctx != before {
		t.Fatal("ScrollView width restoration remeasured the paragraph", before, *ctx)
	}
}

func TestDeferredResizeKeepsMeasuredHeightAsEstimate(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	ctx := &countedNativeContext{Context: native}
	v := NewSourceView()
	v.context = ctx
	defer v.dropLayouts()
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	v.Document().Model().SetText(strings.Repeat("one two three four five ", 300))
	observe(t, v)
	v.Document().engine.Advance(time.Time{})
	v.frameLayout = true
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	settle := func(width float32) {
		for range 8 {
			scroll.Arrange(geometry.Rect(0, 0, width, 240))
			v.layoutStep(time.Time{})
		}
		if v.layoutPending {
			t.Fatal("layout did not settle")
		}
	}
	settle(220)
	before := ctx.created
	oldHeight := v.ContentSize().Height
	scroll.Arrange(geometry.Rect(0, 0, 310, 240))
	if v.ContentSize().Height != oldHeight {
		t.Fatal("width change collapsed the prior height estimate")
	}
	settle(310)
	if ctx.created != before+1 {
		t.Fatal("deferred resize measured an unnecessary scrollbar width", before, ctx.created)
	}
	before = ctx.created
	settle(220)
	if ctx.created != before {
		t.Fatal("deferred restore evicted the recent useful width")
	}
}

// Alternates two already measured widths, as when a user restores a split pane.
// Excludes Context/font setup and initial measurement of the two widths. Includes
// LayoutVisible, height/anchor updates and paragraph lookup; excludes painting,
// native windows and event-loop waiting. Run identical source at both revisions.
func BenchmarkNativeRecentWidths(b *testing.B) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := nativeContext()
	if err != nil {
		b.Fatal(err)
	}
	defer native.Destroy()
	ctx := &countedNativeContext{Context: native}
	v := NewSourceView()
	v.context = ctx
	defer v.dropLayouts()
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	v.Document().Model().SetText(strings.Repeat("a", 8192))
	for _, width := range []float32{520, 480} {
		v.LayoutVisible(geometry.Size{Width: width, Height: 240}, geometry.Point{})
	}
	beforeCreated, beforeMetrics := ctx.created, ctx.metrics
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.LayoutVisible(geometry.Size{Width: float32(520 - 40*(i%2)), Height: 240}, geometry.Point{})
	}
	b.StopTimer()
	b.ReportMetric(float64(ctx.created-beforeCreated)/float64(b.N), "native-layouts/op")
	b.ReportMetric(float64(ctx.metrics-beforeMetrics)/float64(b.N), "metric-exports/op")
}

func TestNativeParagraphHistoryPreservesCompleteBidiLayout(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	ctx := &countedNativeContext{Context: native}
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

func TestNativeLocalEditRetainsHeightUntilMeasurement(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	ctx := &countedNativeContext{Context: native}
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
