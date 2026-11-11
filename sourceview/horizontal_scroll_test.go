package sourceview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

// Fixed 10-DIP glyph metrics: the last text stop is exactly length*10. Keep
// the caret at that stop while scrolling away; no Home/selection workaround.
func TestHorizontalScrollAfterTypingAtEnd(t *testing.T) {
	for _, length := range []int{80, 65530, 66000} {
		for _, width := range []float32{400, 401.5} {
			t.Run(fmt.Sprintf("%d/%g", length, width), func(t *testing.T) {
				v := NewSourceView()
				v.Document().Model().SetText(strings.Repeat("x", length))
				v.SetShowLineNumbers(false)
				v.SetFoldingEnabled(false)
				v.SetWrapMode(WrapNone)
				observe(t, v)
				v.document.engine.Advance(time.Time{})
				ctx := &shapedContext{}
				v.context = ctx
				t.Cleanup(func() {
					v.dropLayouts()
					if ctx.created != ctx.destroyed {
						t.Fatal("layout resource leak", ctx.created, ctx.destroyed)
					}
				})
				scroll := gui.NewScrollView()
				scroll.SetChild(v)
				scroll.Arrange(geometry.Rect(0, 0, width, 120))
				v.SetSelection(textedit.Selection{Anchor: length, Caret: length})
				// Input uses the same Model.Apply/selection/reveal transaction as an
				// ordinary commit, without requiring a desktop just for these metrics.
				if err := v.replace(v.Selection().Range(), strings.Repeat("y", 12), false); err != nil {
					t.Fatal(err)
				}
				for range 24 {
					v.layoutStep(time.Time{})
					scroll.Arrange(scroll.Rect())
				}
				if v.reveal != nil {
					t.Fatalf("line-end reveal never settled: caret=%v content=%v viewport=%v scroll=%g", v.reveal, v.ContentSize(), v.viewport, scroll.ScrollX())
				}
				end := v.Selection()
				if scroll.ScrollX() <= 0 {
					t.Fatal("fixture did not scroll to line end")
				}
				scroll.SetScrollX(0)
				for range 8 {
					v.layoutStep(time.Time{})
					scroll.Arrange(scroll.Rect())
				}
				if scroll.ScrollX() != 0 || v.Selection() != end {
					t.Fatalf("manual scroll was pulled back: offset=%g selection=%v", scroll.ScrollX(), v.Selection())
				}
				text := v.Document().Model().Text()
				scroll.SetScrollX(scroll.Snapshot().MaxScrollX / 2)
				for range 4 {
					v.layoutStep(time.Time{})
					scroll.Arrange(scroll.Rect())
				}
				scroll.SetScrollX(0)
				for range 4 {
					v.layoutStep(time.Time{})
					scroll.Arrange(scroll.Rect())
				}
				if scroll.ScrollX() != 0 || v.Selection() != end || v.Document().Model().Text() != text {
					t.Fatal("repeated scroll changed source/selection or failed to return")
				}
			})
		}
	}
}
