package sourceview

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
)

func BenchmarkParagraphSelection(b *testing.B) {
	for _, size := range []int{80, 4096} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			v := NewSourceView()
			v.Document().Model().SetText(fmt.Sprintf("%*s", size, "x"))
			ctx := &shapedContext{}
			v.context = ctx
			defer v.dropLayouts()
			v.LayoutVisible(geometry.Size{Width: 800, Height: 400}, geometry.Point{})
			created := ctx.created
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				v.SetSelection(textedit.Selection{Caret: i % size})
				v.paragraph(0)
			}
			b.StopTimer()
			b.ReportMetric(float64(ctx.created-created)/float64(b.N), "layouts/op")
		})
	}
}

func widthCacheFixture(t *testing.T) (*SourceView, *shapedContext) {
	t.Helper()
	v := NewSourceView()
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	v.Document().Model().SetText(strings.Repeat("one two three four ", 50))
	observe(t, v)
	v.Document().engine.Advance(time.Time{})
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(func() {
		v.dropLayouts()
		if ctx.created != ctx.destroyed {
			t.Errorf("layout ownership leak: %d created / %d destroyed", ctx.created, ctx.destroyed)
		}
	})
	return v, ctx
}

func TestRecentWidthsReuseParagraphs(t *testing.T) {
	v, ctx := widthCacheFixture(t)
	show := func(width, height float32) *paragraph {
		v.LayoutVisible(geometry.Size{Width: width, Height: height}, geometry.Point{})
		return v.paragraph(0)
	}
	a := show(220, 200)
	b := show(310, 200)
	if a == nil || b == nil || a == b || a.layout == b.layout {
		t.Fatal("width-dependent native resources must be independent")
	}
	created := ctx.created
	for range 5 {
		if show(220, 200) != a || show(310, 300) != b || show(310, 400) != b {
			t.Fatal("returning to a measured width did not reuse its paragraph")
		}
	}
	if ctx.created != created {
		t.Fatal("width restoration or height-only changes created native layouts")
	}
	// The third width evicts the least recently used one, rather than growing
	// a cache for every intermediate position during a window drag.
	show(400, 200)
	if !a.layout.(*shapedLayout).dead || b.layout.(*shapedLayout).dead || len(v.layouts) != 2 {
		t.Fatal("per-text width LRU did not evict the older width")
	}
	for i := range 40 {
		show(float32(500+i), 200)
		if len(v.layouts) != 2 || ctx.created-ctx.destroyed != 2 {
			t.Fatal("continuous resize retained more than two native paragraphs")
		}
	}
}

func TestRecentWidthsInvalidateTogether(t *testing.T) {
	for _, reason := range []string{"tabs", "wrap", "document", "font"} {
		t.Run(reason, func(t *testing.T) {
			v, _ := widthCacheFixture(t)
			scope := themedScope(v, style.Sheet(style.Name("source-view").FontSize(12)))
			v.StyleChanged()
			var old []*shapedLayout
			for _, width := range []float32{220, 310} {
				v.LayoutVisible(geometry.Size{Width: width, Height: 200}, geometry.Point{})
				old = append(old, v.paragraph(0).layout.(*shapedLayout))
			}
			switch reason {
			case "tabs":
				opts := v.IndentOptions()
				opts.TabWidth++
				if err := v.SetIndentOptions(opts); err != nil {
					t.Fatal(err)
				}
			case "wrap":
				v.SetWrapMode(WrapChar)
			case "document":
				v.SetDocument(NewDocument("replacement"))
			case "font":
				scope.SetStyleSheet(style.Sheet(style.Name("source-view").FontSize(17)))
				v.StyleChanged()
			}
			for _, l := range old {
				if !l.dead {
					t.Fatal("an obsolete width survived measurement-input invalidation")
				}
			}
			if len(v.layouts) != 0 {
				t.Fatal("invalidated native resources remain reachable")
			}
		})
	}
}

func TestRestoredWidthRecolorsWithoutChangingGeometry(t *testing.T) {
	v, ctx := widthCacheFixture(t)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	a := v.paragraph(0)
	v.LayoutVisible(geometry.Size{Width: 310, Height: 200}, geometry.Point{})
	v.SetScheme(scheme.Dark())
	created := ctx.created
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	if v.paragraph(0) != a || ctx.created != created {
		t.Fatal("returning after a color-only change discarded measured geometry")
	}
	for _, c := range a.layout.(*shapedLayout).colors {
		if c != v.colors.Foreground {
			t.Fatal("restored width retained obsolete colors")
		}
	}
}

func TestSnapshotDoesNotUseAnotherWidth(t *testing.T) {
	v, ctx := widthCacheFixture(t)
	v.Document().Model().SetText(strings.Repeat("a", 80000))
	v.Document().engine.Advance(time.Time{})
	v.viewport = geometry.Size{Width: 220, Height: 200}
	v.offset.X = 700000
	// 直接准备当前宽度的已测片段，避免让不支持软折行的替身驱动分页。
	// 缓存位置与全文起点相距足够远，以独立验证快照确实使用了它。
	v.layouts[v.paragraphKey(0)] = &paragraph{
		geometry: textedit.NewGeometry(
			[]typography.TextLine{{Start: 70000, Length: 100, Width: 1000, Height: 20}},
			[]typography.TextCluster{{Start: 70000, Length: 100, Width: 1000, Height: 20}},
			80000, 20),
		geometryX: 700000,
	}
	if v.Snapshot().TextEditing.VisibleRange.Start == 0 {
		t.Fatal("fixture did not anchor its measured width")
	}
	// A snapshot between allocation and measurement must use no other width's
	// hit data. It must not trigger a measurement to fill that gap either.
	v.viewport.Width = 310
	created := ctx.created
	if v.Snapshot().TextEditing.VisibleRange.Start != 0 || ctx.created != created {
		t.Fatal("snapshot reused another width or shaped text")
	}
}

func TestSnapshotWithPendingGeometryDoesNotMeasure(t *testing.T) {
	v, ctx := widthCacheFixture(t)
	v.Document().Model().SetText(strings.Repeat("a", 80000))
	v.Document().engine.Advance(time.Time{})
	v.viewport = geometry.Size{Width: 220, Height: 200}
	// 分页尚未就绪时缓存条目已存在，但没有可命中的显示行。
	v.layouts[v.paragraphKey(0)] = &paragraph{}
	created := ctx.created
	info := v.Snapshot().TextEditing
	if info == nil || info.VisibleRange != (textedit.Range{End: 64 * 1024}) || info.VisibleText != strings.Repeat("a", 64*1024) {
		t.Fatal("pending geometry lost the bounded text snapshot")
	}
	if ctx.created != created {
		t.Fatal("snapshot measured pending geometry")
	}
}

type widthCacheDestroyCallback struct {
	typography.TextLayout
	after func()
}

func (l *widthCacheDestroyCallback) Destroy() {
	l.TextLayout.Destroy()
	l.after()
}

func TestWidthEvictionReentryDiscardsReleasedParagraph(t *testing.T) {
	v, _ := widthCacheFixture(t)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	a := v.paragraph(0)
	a.layout = &widthCacheDestroyCallback{TextLayout: a.layout, after: func() {
		v.SetDocument(NewDocument("replacement from destroy callback"))
	}}
	v.LayoutVisible(geometry.Size{Width: 310, Height: 200}, geometry.Point{})
	// Publishing a third width evicts the first, whose callback releases all
	// other widths. The interrupted call must not publish/use its dead result.
	v.viewport.Width = 400
	if p := v.paragraph(0); p != nil {
		t.Fatal("eviction callback returned a paragraph released by document switch")
	}
	if v.Document().Model().Text() != "replacement from destroy callback" || len(v.layouts) != 0 {
		t.Fatal("eviction reentry did not complete or left stale cache entries")
	}
}

func TestRecentWidthsKeepUnchangedLineIdentities(t *testing.T) {
	v, _ := widthCacheFixture(t)
	v.Document().Model().SetText("prefix\nunchanged suffix")
	v.Document().engine.Advance(time.Time{})
	saved := make(map[float32]*paragraph)
	for _, width := range []float32{220, 310} {
		v.LayoutVisible(geometry.Size{Width: width, Height: 200}, geometry.Point{})
		saved[width] = v.paragraph(1)
	}
	// A new first line shifts the unchanged suffix and gives the changed line
	// a new identity. Neither width may reuse the edited line's old content.
	err := v.Document().Model().Replace(textedit.Range{End: len("prefix")}, "new\nfirst")
	if err != nil {
		t.Fatal(err)
	}
	v.Document().engine.Advance(time.Time{})
	for _, width := range []float32{220, 310} {
		v.LayoutVisible(geometry.Size{Width: width, Height: 200}, geometry.Point{})
		if p := v.paragraph(2); p == nil || p != saved[width] || p.text != "unchanged suffix" {
			t.Fatal("unchanged line lost its width-specific identity")
		}
		if p := v.paragraph(0); p == nil || p.text != "new" {
			t.Fatal("edited line reused obsolete text")
		}
	}
}

func TestColdIndexAdoptsWidthsWithinCacheBound(t *testing.T) {
	v := NewSourceView()
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	v.Document().Model().SetText("cold paragraph")
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(func() {
		v.dropLayouts()
		if ctx.created != ctx.destroyed {
			t.Fatal("adopted width resource leak")
		}
	})
	show := func(width float32) *paragraph {
		v.LayoutVisible(geometry.Size{Width: width, Height: 200}, geometry.Point{})
		return v.paragraph(0)
	}
	a, b := show(220), show(310)
	v.Document().engine.Advance(time.Time{})
	c := show(400)
	created := ctx.created
	if show(220) != a || show(310) != b || ctx.created != created {
		t.Fatal("cold widths were remeasured instead of adopted")
	}
	if len(v.layouts) != 2 || !c.layout.(*shapedLayout).dead {
		t.Fatal("adoption retained more than two widths of one stable line")
	}
}

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

func TestUncollapsedFoldCandidatesPreserveLayoutAfterExternalEdit(t *testing.T) {
	for _, insertion := range []string{"共享", "new\n"} {
		t.Run(insertion, func(t *testing.T) {
			v, scroll := revealFixture(t)
			v.document.engine.Advance(time.Time{})
			v.acceptFolds([]language.FoldRange{{StartLine: 0, EndLine: 199}})
			v.frameLayout = true
			settle := func() {
				for range 20 {
					scroll.Arrange(scroll.Rect())
					v.layoutStep(time.Time{})
				}
				if v.layoutPending || v.reveal != nil || v.pendingAnchor {
					t.Fatal("external edit/reveal did not settle")
				}
			}
			r, _ := v.Document().Model().LineRange(180)
			v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
			settle()
			paragraph := v.cachedParagraph(180)
			if paragraph == nil {
				t.Fatal("destination not prepared")
			}
			// This offscreen measured height must survive an inserted LF too;
			// visible cache hits alone must not conceal a full height reset.
			v.paragraph(100)
			if err := v.Document().Model().ReplaceAtomic(textedit.Range{}, insertion); err != nil {
				t.Fatal(err)
			}
			shift := 0
			if insertion == "new\n" {
				shift = 1
			}
			if v.cachedParagraph(180+shift) != paragraph {
				t.Fatal("uncollapsed fold metadata discarded an unchanged suffix layout")
			}
			settle()
			if got := v.paragraphTop(101+shift) - v.paragraphTop(100+shift); got != 32 {
				t.Fatal("line-count mapping reset an offscreen measured height", got)
			}
			caret, ok := v.cachedCaretRect()
			if !ok || caret.Y < 0 || caret.Y+caret.Height > v.viewport.Height {
				t.Fatal("external edit moved the settled caret out of view", caret, v.viewport)
			}
		})
	}
}

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

func TestNativeRecolorMatchesFreshLayout(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	v.Document().Model().SetText("true")
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.mounted = true
	v.Document().acquire(v)
	defer func() { v.dropLayouts(); v.Document().release(v); v.mounted = false }()
	v.context = ctx
	v.Document().engine.Advance(time.Time{})
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	p := v.paragraph(0)
	for _, plain := range []bool{false, true} {
		v.SetScheme(scheme.Dark())
		if plain {
			if err := v.Document().SetLanguage(language.Plain); err != nil {
				t.Fatal(err)
			}
		}
		if got := v.paragraph(0); got != p {
			t.Fatal("native recolor replaced the paragraph")
		}
		foreground := v.colors.Tokens[language.KindKeyword]
		if plain {
			foreground = v.colors.Foreground
		}
		fresh, err := ctx.NewTextLayout("true", typography.TextFormat{Font: v.font, TextColor: foreground, TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}, 1<<20, 1<<24)
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.layout.Rasterize(1, rasterRegion(p.layout, 1), nil)
		if err != nil {
			fresh.Destroy()
			t.Fatal(err)
		}
		// Keep an independent pixel expectation across later rasterizations.
		pixels := bytes.Clone(got.Pixels)
		want, err := fresh.Rasterize(1, rasterRegion(fresh, 1), nil)
		if err != nil {
			fresh.Destroy()
			t.Fatal(err)
		}
		if got.Width != want.Width || got.Height != want.Height || got.Stride != want.Stride || !bytes.Equal(pixels, want.Pixels) {
			fresh.Destroy()
			t.Fatalf("%s plain=%v: cached recolor differs from independent base-color layout", ctx.Name(), plain)
		}
		fresh.Destroy()
	}
}

func TestNativeForegroundMetrics(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	text := "ffi e\u0301 中文 👩‍💻\tשלום xyz"
	for _, family := range []string{"monospace", "serif"} {
		for _, wrap := range []WrapMode{WrapNone, WrapWordChar} {
			l, err := ctx.NewTextLayout(text, typography.TextFormat{Font: typography.FontInfo{Family: family, Size: 12}, WrapMode: wrap, TabWidth: 64}, 160, 800)
			if err != nil {
				t.Fatal(err)
			}
			lines, clusters := copyMetrics(l)
			width, height := l.MeasureSize()
			for _, span := range []struct{ start, end int }{{0, len(text)}, {1, 2}, {4, 7}, {0, len(text)}} {
				l.SetTextColor(span.start, span.end-span.start, color.NRGBA{R: 125, G: 60, B: 210, A: 255})
				gotLines, gotClusters := copyMetrics(l)
				w, h := l.MeasureSize()
				if !reflect.DeepEqual(lines, gotLines) || !reflect.DeepEqual(clusters, gotClusters) || w != width || h != height {
					l.Destroy()
					t.Fatalf("%s %s wrap=%v color [%d,%d) changes metrics: size %g/%g -> %g/%g; lines %v -> %v; clusters %v -> %v", ctx.Name(), family, wrap, span.start, span.end, width, height, w, h, lines, gotLines, clusters, gotClusters)
				}
			}
			l.Destroy()
		}
	}
}

func TestNativeRecolorViewIsolation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	d := NewDocument("true")
	if err := d.SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	a, b := NewSourceView(), NewSourceView()
	for _, v := range []*SourceView{a, b} {
		v.SetDocument(d)
		v.SetScheme(scheme.Light())
		v.mounted = true
		d.acquire(v)
		v.context = ctx
		defer func(v *SourceView) { v.dropLayouts(); d.release(v); v.mounted = false }(v)
	}
	d.engine.Advance(time.Time{})
	for _, v := range []*SourceView{a, b} {
		v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	}
	peer := b.paragraph(0)
	before, err := peer.layout.Rasterize(1, rasterRegion(peer.layout, 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	pixels := bytes.Clone(before.Pixels)
	a.SetScheme(scheme.Dark())
	a.paragraph(0)
	if b.paragraph(0) != peer {
		t.Fatal("another view's recolor invalidated peer layout")
	}
	after, err := peer.layout.Rasterize(1, rasterRegion(peer.layout, 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if before.Width != after.Width || before.Height != after.Height || !bytes.Equal(pixels, after.Pixels) {
		t.Fatal("recolor contaminated another view's native state")
	}
}

// Compare glyph pixels against a separately constructed, single-color layout.
// Each fixture's visible glyphs have an independently specified lexical color.
// Same backend/font/scale on both sides; borrowed bitmap pixels are copied.
func TestNativeConfigurationAndShellColors(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	for _, tc := range []struct {
		id         language.ID
		text, line string
		row        int
		kind       language.Kind
	}{
		{language.TOML, "text = '''\nbody\n'''", "body", 1, language.KindString},
		{language.YAML, "text: |\n  body\nnext: true", "  body", 1, language.KindString},
		{language.Shell, "$HOME", "$HOME", 0, language.KindVariable},
		{language.Shell, "cat <<END\n${HOME}\nEND", "${HOME}", 1, language.KindVariable},
	} {
		for _, colors := range []*scheme.Scheme{scheme.Light(), scheme.Dark()} {
			func() {
				v := NewSourceView()
				v.SetScheme(colors)
				v.Document().Model().SetText(tc.text)
				if err := v.Document().SetLanguage(tc.id); err != nil {
					t.Fatal(err)
				}
				v.mounted = true
				v.Document().acquire(v)
				defer func() { v.dropLayouts(); v.Document().release(v); v.mounted = false }()
				v.context = ctx
				v.Document().engine.Advance(time.Time{})
				v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
				p := v.paragraph(tc.row)
				fresh, err := ctx.NewTextLayout(tc.line, typography.TextFormat{Font: v.font, TextColor: colors.Color(tc.kind), TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}, 1<<20, 1<<24)
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Destroy()
				got, err := p.layout.Rasterize(1, rasterRegion(p.layout, 1), nil)
				if err != nil {
					t.Fatal(err)
				}
				pixels := bytes.Clone(got.Pixels)
				want, err := fresh.Rasterize(1, rasterRegion(fresh, 1), nil)
				if err != nil {
					t.Fatal(err)
				}
				if got.Width != want.Width || got.Height != want.Height || got.Stride != want.Stride || !bytes.Equal(pixels, want.Pixels) {
					t.Fatalf("%s %s %s: glyph color differs from %s", ctx.Name(), tc.id, colors.Definition().Name, tc.kind)
				}
			}()
		}
	}
}
