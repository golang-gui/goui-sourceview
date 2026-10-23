package sourceview

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

type layoutWorkRequest struct {
	text  string
	width float32
}

type layoutWorkContext struct {
	shapedContext
	requests []layoutWorkRequest
	delay    time.Duration
	hook     func()
	fail     bool
}

func (c *layoutWorkContext) NewTextLayout(text string, format typography.TextFormat, width, height float32) (typography.TextLayout, error) {
	if strings.HasPrefix(text, "row ") {
		c.requests = append(c.requests, layoutWorkRequest{text, width})
		if c.delay > 0 {
			time.Sleep(c.delay)
		}
		if c.hook != nil {
			fn := c.hook
			c.hook = nil
			fn()
		}
		if c.fail {
			return nil, errors.New("native layout failure fixture")
		}
	}
	return c.shapedContext.NewTextLayout(text, format, width, height)
}

func layoutWorkFixture(t *testing.T, rows int) (*SourceView, *layoutWorkContext) {
	t.Helper()
	v := NewSourceView()
	var text strings.Builder
	for i := 0; i < rows; i++ {
		if i != 0 {
			text.WriteByte('\n')
		}
		fmt.Fprintf(&text, "row %05d", i)
	}
	v.Document().Model().SetText(text.String())
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	observe(t, v)
	v.Document().engine.Advance(time.Time{})
	ctx := &layoutWorkContext{}
	v.context = ctx
	v.ensureMetrics()
	v.frameLayout = true // Drive the same private frame step without a desktop.
	t.Cleanup(func() {
		v.dropLayouts()
		if ctx.created != ctx.destroyed {
			t.Errorf("layout work leaked resources: %d/%d", ctx.created, ctx.destroyed)
		}
	})
	return v, ctx
}

func TestViewportAndPaintQueueMissingMeasurements(t *testing.T) {
	v, ctx := layoutWorkFixture(t, 100)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	v.Paint(&recordingPainter{})
	if len(ctx.requests) != 0 || !v.layoutPending || len(v.layouts) != 0 {
		t.Fatal("layout/paint synchronously measured missing paragraphs")
	}
	v.layoutStep(time.Time{})
	if len(ctx.requests) < 10 || v.layoutPending || len(v.layouts) != len(ctx.requests) {
		t.Fatal("frame did not publish the visible paragraphs", len(ctx.requests), len(v.layouts))
	}
	before := len(ctx.requests)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 300}, geometry.Point{})
	if len(ctx.requests) != before || !v.layoutPending {
		t.Fatal("growing height measured new rows synchronously")
	}
	v.layoutStep(time.Time{})
	if len(ctx.requests) <= before {
		t.Fatal("new visible rows were not prepared")
	}
	before = len(ctx.requests)
	v.Paint(&recordingPainter{})
	v.LayoutVisible(geometry.Size{Width: 220, Height: 100}, geometry.Point{})
	if len(ctx.requests) != before || v.layoutPending {
		t.Fatal("paint or shrinking height remeasured ready paragraphs")
	}
}

func TestLayoutStepRespectsBatchDeadline(t *testing.T) {
	v, ctx := layoutWorkFixture(t, 100)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	v.layoutStep(time.Now().Add(-time.Second))
	if len(ctx.requests) != 0 || !v.layoutPending {
		t.Fatal("expired batch budget performed native work")
	}
	// The deliberately slow indivisible call may exceed the soft budget. The
	// batch must yield after it, not continue with every other visible row.
	ctx.delay = 60 * time.Millisecond
	v.layoutStep(time.Now().Add(40 * time.Millisecond))
	if len(ctx.requests) > 1 || !v.layoutPending {
		t.Fatal("batch continued native work after its deadline", len(ctx.requests))
	}
	ctx.delay = 0
	v.layoutStep(time.Time{})
	if len(ctx.requests) < 10 || v.layoutPending {
		t.Fatal("yielded work did not resume")
	}
}

func TestLayoutWorkOnlyMeasuresLatestWidthAndViewport(t *testing.T) {
	v, ctx := layoutWorkFixture(t, 1000)
	for _, width := range []float32{220, 310, 400} {
		v.LayoutVisible(geometry.Size{Width: width, Height: 200}, geometry.Point{})
	}
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{Y: 10000})
	v.layoutStep(time.Time{})
	if len(ctx.requests) == 0 {
		t.Fatal("latest demand was not processed")
	}
	for _, r := range ctx.requests {
		// Native fixture rows are 20 DIP. The 6-DIP content inset makes the
		// first containing row floor((10000-6)/20)=499; include one overscan row.
		if r.width != 400-2*padding || r.text < "row 00499" || r.text > "row 00510" {
			t.Fatal("measured obsolete width/viewport", r)
		}
	}
}

func TestDeferredRecolorKeepsMeasuredGeometry(t *testing.T) {
	v, ctx := layoutWorkFixture(t, 1)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	v.layoutStep(time.Time{})
	p := v.cachedParagraph(0)
	native := p.layout.(*shapedLayout)
	calls, created := native.colorCalls, ctx.created
	v.SetScheme(scheme.Dark())
	v.Paint(&recordingPainter{})
	if native.colorCalls != calls || !v.layoutPending || v.cachedParagraph(0) != p {
		t.Fatal("paint synchronously recolored or discarded geometry")
	}
	v.layoutStep(time.Time{})
	if native.colorCalls <= calls || ctx.created != created || v.cachedParagraph(0) != p {
		t.Fatal("frame recolor changed measurement identity")
	}
}

func TestLayoutWorkStopsAfterNativeReentry(t *testing.T) {
	v, ctx := layoutWorkFixture(t, 100)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	ctx.hook = func() { v.SetDocument(NewDocument("replacement")) }
	v.layoutStep(time.Time{})
	if len(ctx.requests) != 1 || len(v.layouts) != 0 || v.Document().Model().Text() != "replacement" {
		t.Fatal("stale layout work continued or published after document replacement")
	}
	if ctx.created != ctx.destroyed {
		t.Fatal("interrupted unpublished layout was not released")
	}
}

func TestFailedAutomaticLayoutDoesNotRetryEveryFrame(t *testing.T) {
	v, ctx := layoutWorkFixture(t, 1)
	ctx.fail = true
	reported := 0
	v.ConnectError(func(error) { reported++ })
	for range 5 {
		v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
		v.layoutStep(time.Time{})
		v.Paint(&recordingPainter{})
	}
	if len(ctx.requests) != 1 || reported != 1 || v.layoutPending {
		t.Fatal("failed native request retried/reported continuously", len(ctx.requests), reported)
	}
	ctx.fail = false
	if v.paragraph(0) == nil || len(ctx.requests) != 2 {
		t.Fatal("explicit geometry request could not retry a transient failure")
	}
}

func TestFrameLayoutRotatesViews(t *testing.T) {
	a, ca := layoutWorkFixture(t, 100)
	b, cb := layoutWorkFixture(t, 100)
	for _, v := range []*SourceView{a, b} {
		v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	}
	ca.delay, cb.delay = 6*time.Millisecond, 6*time.Millisecond
	q := &frameQueue{views: []*SourceView{a, b}}
	for range 4 {
		q.frame(time.Now())
	}
	if len(ca.requests) == 0 || len(cb.requests) == 0 {
		t.Fatal("one busy view starved another view's layout")
	}
}

func TestDeferredRevealPreparesCaretAndEntireViewport(t *testing.T) {
	v, _ := layoutWorkFixture(t, 1000)
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	settle := func() {
		for range 12 {
			scroll.Arrange(geometry.Rect(0, 0, 220, 200))
			v.layoutStep(time.Time{})
		}
		if v.layoutPending || v.reveal != nil {
			t.Fatal("reveal did not finish after preparing its viewport", v.layoutPending, v.reveal)
		}
	}
	settle()
	end := v.Document().Model().Len()
	v.SetSelection(textedit.Selection{Anchor: end, Caret: end})
	settle()
	if scroll.ScrollY() == 0 {
		t.Fatal("document end did not scroll")
	}
	v.SetSelection(textedit.Selection{})
	settle()
	if scroll.ScrollY() > padding || v.Selection().Caret != 0 {
		t.Fatal("home did not restore the document start")
	}
}

func TestTallViewportPinsItsWorkAndReleasesAfterShrinking(t *testing.T) {
	v, ctx := layoutWorkFixture(t, 1000)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 6000}, geometry.Point{})
	v.layoutStep(time.Time{})
	v.LayoutVisible(geometry.Size{Width: 220, Height: 6000}, geometry.Point{})
	if v.layoutPending || len(v.layouts) < 300 {
		t.Fatal("viewport larger than the soft cache bound kept evicting visible work")
	}
	before := len(ctx.requests)
	v.Paint(&recordingPainter{})
	if len(ctx.requests) != before || v.layoutPending {
		t.Fatal("painting a ready tall viewport restarted measurement")
	}
	v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
	if len(v.layouts) > maxLayouts {
		t.Fatal("shrinking did not release the previously pinned working set")
	}
}

func TestViewportPinningDoesNotKeepEditedLineHistory(t *testing.T) {
	v, _ := layoutWorkFixture(t, 100)
	for i := 0; i < 2*maxLayouts; i++ {
		r, _ := v.Document().Model().LineRange(0)
		if err := v.Document().Model().ReplaceAtomic(r, fmt.Sprintf("row edit %d", i)); err != nil {
			t.Fatal(err)
		}
		v.LayoutVisible(geometry.Size{Width: 220, Height: 200}, geometry.Point{})
		v.layoutStep(time.Time{})
		if len(v.layouts) > maxLayouts {
			t.Fatal("obsolete line identities remained pinned after editing")
		}
	}
}
