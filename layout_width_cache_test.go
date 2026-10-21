package sourceview

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
)

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
