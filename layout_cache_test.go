package sourceview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
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
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{X: 700000})
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
