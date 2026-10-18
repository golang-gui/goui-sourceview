package sourceview

import (
	"strings"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

// ASCII probes measure 20 DIP, while fallback glyphs make document rows 32 DIP.
// The expectation is independent of HeightIndex's initial estimates.
type fallbackContext struct{ shapedContext }

func (c *fallbackContext) NewTextLayout(text string, format typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	l, err := c.shapedContext.NewTextLayout(text, format, w, h)
	if err == nil && strings.Contains(text, "中") {
		return &fallbackLayout{TextLayout: l}, nil
	}
	return l, err
}

type fallbackLayout struct{ typography.TextLayout }

func (l *fallbackLayout) MeasureSize() (float32, float32) {
	w, _ := l.TextLayout.MeasureSize()
	return w, 32
}
func (l *fallbackLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	lines, clusters := l.TextLayout.MeasureMetrics()
	for i := range lines {
		lines[i].Height = 32
	}
	for i := range clusters {
		clusters[i].Height = 32
	}
	return lines, clusters
}

func revealFixture(t *testing.T) (*SourceView, *gui.ScrollView) {
	t.Helper()
	v := NewSourceView()
	v.Document().Model().SetText(strings.Repeat("中文 row\n", 200))
	observe(t, v)
	ctx := &fallbackContext{}
	v.context = ctx
	t.Cleanup(func() {
		v.dropLayouts()
		if ctx.created != ctx.destroyed {
			t.Fatalf("native layout leak: %d created, %d destroyed", ctx.created, ctx.destroyed)
		}
	})
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	scroll.Arrange(geometry.Rect(0, 0, 400, 100))
	return v, scroll
}

func assertRevealed(t *testing.T, v *SourceView, scroll *gui.ScrollView) {
	t.Helper()
	// Model normal subsequent layout passes, bounded independently of the code.
	for range 8 {
		scroll.Arrange(scroll.Rect())
	}
	r, ok := v.caretRect()
	if !ok || r.Height != 32 || r.Y < 0 || r.Y+r.Height > v.viewport.Height || r.X < v.gutter || r.X+r.Width > v.viewport.Width {
		t.Fatalf("caret outside viewport after actual row heights: caret=%v viewport=%v offset=%v", r, v.viewport, v.offset)
	}
}

func TestRevealSelectionAfterColdLayout(t *testing.T) {
	v, scroll := revealFixture(t)
	r, _ := v.Document().Model().LineRange(180)
	v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
	assertRevealed(t, v, scroll)
	// A finished reveal must not pin the caret when the user scrolls elsewhere.
	scroll.SetScrollY(0)
	scroll.Arrange(scroll.Rect())
	if scroll.ScrollY() != 0 {
		t.Fatal("completed reveal overrode subsequent scrolling", scroll.ScrollY())
	}
}

func TestRevealSelectionAfterLayoutReset(t *testing.T) {
	v, scroll := revealFixture(t)
	r, _ := v.Document().Model().LineRange(100)
	v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
	assertRevealed(t, v, scroll)
	// Reattachment keeps the logical selection but loses measured paragraphs.
	// Its retained local offset is close to the new estimate, while the host's
	// offset resets to zero. Arrange's provisional offset cannot confirm reveal.
	scroll.SetScrollY(2000)
	scroll.Arrange(scroll.Rect())
	scroll.SetChild(nil)
	v.invalidateLayout()
	scroll.SetChild(v)
	v.RevealSelection()
	assertRevealed(t, v, scroll)
}

func TestRevealSelectionBeforeAllocation(t *testing.T) {
	v, scroll := revealFixture(t)
	scroll.SetChild(nil)
	v.LayoutVisible(geometry.Size{}, geometry.Point{})
	r, _ := v.Document().Model().LineRange(180)
	v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
	if v.reveal == nil {
		t.Fatal("zero allocation lost the logical reveal request")
	}
	scroll.SetChild(v)
	scroll.Arrange(scroll.Rect())
	caret, ok := v.caretRect()
	if !ok || caret.Y < 0 || caret.Y+caret.Height > v.viewport.Height {
		t.Fatal("first host allocation left destination rows estimated", caret, v.viewport)
	}
	assertRevealed(t, v, scroll)
	if v.reveal != nil {
		t.Fatal("visible caret did not finish the reveal request")
	}
}

func TestRevealSelectionInShortViewport(t *testing.T) {
	v, scroll := revealFixture(t)
	scroll.Arrange(geometry.Rect(0, 0, 400, 16))
	r, _ := v.Document().Model().LineRange(180)
	v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
	for range 8 {
		scroll.Arrange(scroll.Rect())
	}
	caret, ok := v.caretRect()
	if !ok || caret.Y != 0 || v.reveal != nil {
		t.Fatal("oversized caret did not settle at its leading edge", caret, v.reveal)
	}
}

func TestPendingRevealCancellation(t *testing.T) {
	for _, action := range []string{"pointer selection", "document", "model reset", "shared edit", "unmount"} {
		t.Run(action, func(t *testing.T) {
			v, scroll := revealFixture(t)
			scroll.SetChild(nil) // No host consumes the request yet.
			r, _ := v.Document().Model().LineRange(180)
			v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
			if v.reveal == nil {
				t.Fatal("fixture has no pending reveal")
			}
			switch action {
			case "pointer selection":
				v.setSelection(textedit.Selection{}, false, false)
			case "document":
				v.SetDocument(NewDocument("replacement"))
			case "model reset":
				v.Document().Model().SetText("replacement")
			case "shared edit":
				if err := v.Document().Model().Apply(textedit.Edit{Range: textedit.Range{}, Text: "x"}, textedit.EditOptions{}); err != nil {
					t.Fatal(err)
				}
			case "unmount":
				v.unmount()
			}
			if v.reveal != nil {
				t.Fatal("obsolete reveal survived", action)
			}
		})
	}
}

func TestRevealCallbackCanReplaceSelection(t *testing.T) {
	v, scroll := revealFixture(t)
	scroll.SetChild(nil)
	r, _ := v.Document().Model().LineRange(180)
	h := v.ConnectScrollIntoView(func(geometry.Rectangle) {
		v.setSelection(textedit.Selection{}, false, false)
	})
	v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
	h.Disconnect()
	if v.Selection() != (textedit.Selection{}) || v.reveal != nil {
		t.Fatal("callback's newer selection was overwritten", v.Selection())
	}
	scroll.SetChild(v)
	scroll.Arrange(scroll.Rect())
	if scroll.ScrollY() != 0 {
		t.Fatal("old reveal resumed after the callback", scroll.ScrollY())
	}
}
