package sourceview

import (
	"image/color"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/typography"
)

// Exercise a >32-KiB paragraph through Context/TextLayout alone. Full native
// context, recent widths, color updates and snapshot lifetime remain supported
// after withdrawing the separate retained-glyph interfaces.
func TestNativeLongWidthsUseExistingLayoutContract(t *testing.T) {
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
	text := "start \u202e" + strings.Repeat("abc 123 العربية עברית e\u0301 ffi 👩‍💻\t ", 600) + "END\u202c"
	if len(text) < 32768 {
		t.Fatal("fixture must cover the removed long-paragraph branch")
	}
	v.Document().Model().SetText(text)
	show := func(width, height float32) *paragraph {
		v.LayoutVisible(geometry.Size{Width: width, Height: height}, geometry.Point{})
		return v.paragraph(0)
	}
	a := show(240, 200)
	if a == nil || a.fragment.LineCount() < 100 {
		t.Fatal("missing measured long paragraph")
	}
	position := textedit.Position{Offset: strings.Index(text, "END")}
	wantCaret := a.geometry.Caret(position)
	fragment := a.fragment
	w, h := a.layout.Size()
	reference, err := native.NewTextLayout(text, a.layout.Format(), w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer reference.Destroy()
	wantLines, wantClusters := copyMetrics(reference)
	gotLines, gotClusters := fragment.Metrics()
	if !reflect.DeepEqual(gotLines, wantLines) || !reflect.DeepEqual(gotClusters, wantClusters) {
		t.Fatal("cached values differ from complete native context")
	}
	b := show(310, 200)
	if b == a || b.layout == a.layout {
		t.Fatal("different widths shared mutable native resources")
	}
	before := *ctx
	for range 3 {
		if show(240, 280) != a || show(310, 320) != b || *ctx != before {
			t.Fatal("restoring width or changing height repeated native work", before, *ctx)
		}
	}
	style := b.style
	style.selection = textedit.Range{Start: 10, End: 100}
	style.focused = true
	v.colors.SelectionForeground = color.NRGBA{R: 220, A: 255}
	if !v.colorParagraph(v.paragraphKey(0), b, style, nil, activeFocus(v)) {
		t.Fatal("foreground update failed")
	}
	if len(b.colors) == 0 || ctx.colors == before.colors || ctx.created != before.created || ctx.metrics != before.metrics {
		t.Fatal("foreground update did not reuse the measured paragraph")
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("native width resources leaked", ctx.created, ctx.destroyed)
	}
	g := fragment.Geometry(v.lineHeight)
	if g.Caret(position) != wantCaret {
		t.Fatal("ordinary snapshot changed after native cache release")
	}
}

type estimateNativeContext struct {
	countedNativeContext
	lengths []int
}

func (c *estimateNativeContext) NewTextLayout(text string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	c.lengths = append(c.lengths, len(text))
	return c.countedNativeContext.NewTextLayout(text, f, w, h)
}

func TestNativeInitialWrappedEstimateAvoidsSecondScrollbarWidth(t *testing.T) {
	for index, text := range []string{
		strings.Repeat("long wrapped text with tabs\t ", 300),
		"\u202e" + strings.Repeat("中文 e\u0301 ffi العربية 👩‍💻 ", 300) + "\u202c",
		strings.Repeat("\u200b", 3000), // A byte estimate must not become final geometry.
	} {
		t.Run([]string{"tabs", "persistent-bidi", "zero-advance"}[index], func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			native, err := nativeContext()
			if err != nil {
				t.Fatal(err)
			}
			defer native.Destroy()
			ctx := &estimateNativeContext{countedNativeContext: countedNativeContext{Context: native}}
			v := NewSourceView()
			v.context = ctx
			v.SetShowLineNumbers(false)
			v.SetWrapMode(WrapWordChar)
			v.Document().Model().SetText(text)
			v.ensureMetrics()
			before := ctx.created
			v.Measure(layout.Tight(geometry.Size{Width: 220, Height: 240}))
			if ctx.created != before+1 || ctx.lengths[len(ctx.lengths)-1] > 512 || len(v.layouts) != 0 {
				t.Fatal("natural measure shaped more than its bounded estimate")
			}
			v.Measure(layout.Tight(geometry.Size{Width: 220, Height: 240}))
			if ctx.created != before+1 {
				t.Fatal("natural measurement repeated its provisional sample")
			}
			scroll := gui.NewScrollView()
			scroll.SetChild(v)
			scroll.Measure(layout.Tight(geometry.Size{Width: 220, Height: 240}))
			scroll.Arrange(geometry.Rect(0, 0, 220, 240))
			p := v.paragraph(0)
			if p == nil || ctx.created != before+2 || v.heights.Total() != p.height {
				t.Fatal("initial allocation repeated full shaping or retained estimated height")
			}
			// Independent full-context native reference, including persistent bidi
			// controls and text whose many bytes have zero advance.
			reference, err := native.NewTextLayout(text, p.layout.Format(), v.paragraphWidth(), 1<<24)
			if err != nil {
				t.Fatal(err)
			}
			w, h := reference.MeasureSize()
			wantLines, wantClusters := reference.MeasureMetrics()
			gotLines, gotClusters := p.layout.MeasureMetrics()
			reference.Destroy()
			if p.width != w || p.height != max(v.lineHeight, h) || !reflect.DeepEqual(gotLines, wantLines) || !reflect.DeepEqual(gotClusters, wantClusters) {
				t.Fatal("provisional sampling changed confirmed full-context geometry")
			}
			v.dropLayouts()
			if ctx.created != ctx.destroyed {
				t.Fatal("sampling or layout leaked native resources")
			}
		})
	}
}
