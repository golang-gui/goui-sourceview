package sourceview

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/typography"
)

type estimateNativeContext struct {
	widthCacheNativeContext
	lengths []int
}

func (c *estimateNativeContext) NewTextLayout(text string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	c.lengths = append(c.lengths, len(text))
	return c.widthCacheNativeContext.NewTextLayout(text, f, w, h)
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
			ctx := &estimateNativeContext{widthCacheNativeContext: widthCacheNativeContext{Context: native}}
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
