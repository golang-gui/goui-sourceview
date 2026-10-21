package sourceview

import (
	"bytes"
	"image"
	"image/color"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

// Count native API entry, not a guessed amount of work from source length.
type widthCacheNativeContext struct {
	typography.Context
	created, sizes, metrics, resizes, colors, destroyed int
}

func (c *widthCacheNativeContext) NewTextLayout(text string, format typography.TextFormat, width, height float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(text, format, width, height)
	if err != nil {
		return nil, err
	}
	c.created++
	return &widthCacheNativeLayout{TextLayout: l, owner: c}, nil
}

type widthCacheNativeLayout struct {
	typography.TextLayout
	owner *widthCacheNativeContext
}

func (l *widthCacheNativeLayout) MeasureSize() (float32, float32) {
	l.owner.sizes++
	return l.TextLayout.MeasureSize()
}
func (l *widthCacheNativeLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	l.owner.metrics++
	return l.TextLayout.MeasureMetrics()
}
func (l *widthCacheNativeLayout) SetSize(width, height float32) {
	l.owner.resizes++
	l.TextLayout.SetSize(width, height)
}
func (l *widthCacheNativeLayout) SetTextColor(start, length int, c color.Color) {
	l.owner.colors++
	l.TextLayout.SetTextColor(start, length, c)
}
func (l *widthCacheNativeLayout) Destroy() {
	l.owner.destroyed++
	l.TextLayout.Destroy()
}

func TestNativeRecentWidthsKeepMetricsAndPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	ctx := &widthCacheNativeContext{Context: native}
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
	ctx := &widthCacheNativeContext{Context: native}
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
	ctx := &widthCacheNativeContext{Context: native}
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
