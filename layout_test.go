package sourceview

import (
	"errors"
	"fmt"
	"image/color"
	"math/rand"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
	"golang.org/x/text/unicode/bidi"
)

func TestAnalysisOnlyInvalidatesVisibleTokenChanges(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText(strings.Repeat("<p>value</p>\n", 1000))
	if err := v.Document().SetLanguage(language.XML); err != nil {
		t.Fatal(err)
	}
	v.SetFoldingEnabled(false)
	observe(t, v)
	v.Document().engine.Advance(time.Time{})
	ctx := &shapedContext{}
	v.context, v.frameLayout = ctx, true
	t.Cleanup(func() {
		v.dropLayouts()
		if ctx.created != ctx.destroyed {
			t.Fatal("analysis view leaked native layouts", ctx.created, ctx.destroyed)
		}
	})
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	v.layoutStep(time.Time{})
	created := ctx.created
	if v.analysisPaintNeeded() {
		t.Fatal("unchanged published colors request another paint")
	}
	edit := func(row int) {
		r, _ := v.Document().Model().LineRange(row)
		if err := v.Document().Model().ReplaceAtomic(textedit.Range{Start: r.Start + 3, End: r.Start + 8}, "other"); err != nil {
			t.Fatal(err)
		}
	}
	q := &frameQueue{views: []*SourceView{v}}
	edit(900)
	before := v.Document().engine.ScannedLines()
	if !q.advanceAnalysis(time.Now().Add(-time.Second)) || v.Document().engine.ScannedLines() != before {
		t.Fatal("expired analysis batch performed scanning")
	}
	if q.advanceAnalysis(time.Time{}) || v.analysisPaintNeeded() || ctx.created != created {
		t.Fatal("offscreen analysis repainted or shaped the viewport")
	}
	edit(3)
	q.advanceAnalysis(time.Time{})
	if !v.analysisPaintNeeded() || ctx.created != created {
		t.Fatal("visible token changes were missed or measured by pure analysis")
	}
	v.layoutStep(time.Time{}) // Only frame work may prepare changed text/geometry.
	v.LayoutVisible(v.viewport, v.offset)
	v.layoutStep(time.Time{})
	if v.analysisPaintNeeded() {
		t.Fatal("frame did not publish current visible colors")
	}
}

type analysisRoot struct {
	gui.Root
	paints int
}

func (r *analysisRoot) RequestPaint() error { r.paints++; return nil }

func TestAnalysisTimerFailureKeepsFrameFallback(t *testing.T) {
	root := &analysisRoot{}
	q := &frameQueue{root: root, analysisTimer: &gui.Timer{}, analysisHandle: signal.Handles(nil)}
	q.requestAnalysis()
	if q.analysisTimer != nil || root.paints != 1 {
		t.Fatal("unavailable timer left pending work without a frame", q.analysisTimer, root.paints)
	}
}

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

func TestVisibleLineMapAgainstEnumeration(t *testing.T) {
	rng := rand.New(rand.NewSource(7331))
	for range 2000 {
		n := 1 + rng.Intn(250)
		var hidden []lineInterval
		var visible []int
		for row := 0; row < n; {
			if row != 0 && rng.Intn(4) == 0 {
				end := min(n, row+1+rng.Intn(20))
				hidden = append(hidden, lineInterval{row, end})
				row = end
			} else {
				visible = append(visible, row)
				row++
			}
		}
		var m visibleLineMap
		m.reset(n, hidden)
		if m.count != len(visible) || m.line(m.count) != n {
			t.Fatalf("counts: %+v want %v", m, visible)
		}
		for i, line := range visible {
			if got := m.line(i); got != line {
				t.Fatalf("line(%d)=%d want %d", i, got, line)
			}
		}
		previous := 0
		for row := range n {
			wantVisible := previous+1 < len(visible) && visible[previous+1] == row || visible[previous] == row
			if previous+1 < len(visible) && visible[previous+1] == row {
				previous++
			}
			index, ok := m.index(row)
			if index != previous || ok != wantVisible {
				t.Fatalf("index(%d)=%d,%v want %d,%v", row, index, ok, previous, wantVisible)
			}
		}
	}
}

func TestHiddenParagraphsUseVisibleHeightsAndSourceCoordinates(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("head\nhidden\nhidden\nhidden\ntail")
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	tail := v.paragraph(4)
	v.hiddenLines = []lineInterval{{1, 4}}
	v.resetHeights()
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	if v.heights.Count() != 2 || v.heights.Total() != 40 || v.paragraph(2) != nil || v.paragraph(4) != tail {
		t.Fatalf("visibility affected paragraph identity or heights: %d %g", v.heights.Count(), v.heights.Total())
	}
	pos := v.hit(geometry.Point{X: v.gutter + padding, Y: padding + 21})
	r, _ := v.Document().Model().LineRange(4)
	if pos.Offset != r.Start || v.paragraphTop(4) != 20 || v.neighboringRow(0, 1) != 4 || v.neighboringRow(4, -1) != 0 {
		t.Fatalf("source/display mapping: hit=%v range=%v", pos, r)
	}
}

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

// Ordinary snapshots must retain native Unicode geometry without retaining a
// native layout. This is a metric/cache regression, not a claim that separately
// shaped substrings have inherited a paragraph's complete context.
func TestNativeFragmentsSurviveLayoutRelease(t *testing.T) {
	for _, sample := range []struct{ name, text string }{
		{"json", strings.Repeat(`{"name":"hello world","value":1234},`, 130)},
		{"unicode", strings.Repeat("中文 e\u0301 ffi 👩‍💻 العربية עברית\t ", 130)},
		{"override", "start \u202e" + strings.Repeat("abcd 1234 ", 410) + "XYZ\u202c end"},
		{"isolate", strings.Repeat("a \u2067עברית 123 العربية\u2069 b ", 150)},
	} {
		t.Run(sample.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ctx, err := nativeContext()
			if err != nil {
				t.Fatal(err)
			}
			defer ctx.Destroy()
			format := typography.TextFormat{Font: typography.FontInfo{Family: "DejaVu Sans", Size: 12}, WrapMode: typography.WrapWordChar, TabWidth: 40}
			layout, err := ctx.NewTextLayout(sample.text, format, 220, 1<<24)
			if err != nil {
				t.Fatal(err)
			}
			width, height := layout.MeasureSize()
			lines, clusters := copyMetrics(layout)
			f, err := textedit.NewFragment(textedit.Range{End: len(sample.text)}, geometry.Size{Width: width, Height: height}, lines, clusters)
			layout.Destroy()
			if err != nil {
				t.Fatal(err)
			}
			if f.LineCount() <= 64 {
				t.Fatalf("fixture did not cover pages: %d lines", f.LineCount())
			}
			got, want := f.Geometry(20), textedit.NewGeometry(lines, clusters, len(sample.text), 20)
			for i, line := range lines {
				for _, x := range []float32{-5, 0, 30, 100, 225} {
					point := geometry.Point{X: x, Y: line.Y + line.Height/2}
					if got.Hit(point) != want.Hit(point) {
						t.Fatalf("hit at line %d x=%g: %+v want %+v", i, x, got.Hit(point), want.Hit(point))
					}
				}
				for _, pos := range []textedit.Position{{Offset: line.Start}, {Offset: line.Start, Upstream: true}, {Offset: line.Start + line.Length, Upstream: true}} {
					if got.Caret(pos) != want.Caret(pos) || got.LineIndex(pos) != want.LineIndex(pos) {
						t.Fatalf("caret at %+v differs: %v want %v", pos, got.Caret(pos), want.Caret(pos))
					}
					for _, forward := range []bool{false, true} {
						if got.Snap(pos.Offset, forward) != want.Snap(pos.Offset, forward) || got.Adjacent(pos.Offset, forward) != want.Adjacent(pos.Offset, forward) {
							t.Fatalf("boundary at %+v forward=%v", pos, forward)
						}
					}
				}
			}
			for first := 0; first < len(lines); first += 31 {
				end := min(first+35, len(lines))
				rng := textedit.Range{Start: lines[first].Start, End: lines[end-1].Start + lines[end-1].Length}
				if !reflect.DeepEqual(got.SelectionLines(rng, first, end), want.SelectionLines(rng, first, end)) {
					t.Fatalf("selection [%d,%d) differs", first, end)
				}
			}
			// Keep the independent semantic assertion, not just path agreement.
			if sample.name == "override" {
				offset := strings.Index(sample.text, "XYZ")
				_, cs := f.Metrics()
				covered := 0
				for _, c := range cs {
					if c.Start < offset+3 && offset < c.Start+c.Length {
						if c.Direction != typography.TextRightToLeft {
							t.Fatal("RLO context lost in snapshot")
						}
						// CoreText may keep the following PDF in Z's native
						// cluster. Count only the independently known XYZ bytes,
						// while checking the direction of every overlapping run.
						covered += min(c.Start+c.Length, offset+3) - max(c.Start, offset)
					}
				}
				if covered != 3 {
					t.Fatal("RLO target coverage", covered)
				}
			}
		})
	}
}

func TestSourceViewRetainsFragmentAfterNativeCacheRelease(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	v := NewSourceView()
	v.context = ctx
	defer v.dropLayouts()
	v.Document().Model().SetText(strings.Repeat("one two three four five ", 300))
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 240}, geometry.Point{})
	p := v.paragraph(0)
	if p == nil || p.fragment.LineCount() < 64 {
		t.Fatal("missing measured fragment")
	}
	f := p.fragment
	want := p.geometry.Caret(textedit.Position{Offset: 500})
	v.dropLayouts()
	g := f.Geometry(v.lineHeight)
	if got := g.Caret(textedit.Position{Offset: 500}); got != want {
		t.Fatal("native cache release lost fragment values", got, want)
	}
}

// A copied window owns its values but cannot manufacture the context omitted
// from its input. Keep this explicit guard against enabling independent-window
// shaping as a universally correct editor path.
func TestFragmentSnapshotDoesNotInventMissingParagraphContext(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	text := "start \u202e" + strings.Repeat("abcd 1234 ", 410) + "XYZ\u202c end"
	format := typography.TextFormat{Font: typography.FontInfo{Family: "DejaVu Sans", Size: 12}, WrapMode: typography.WrapWordChar}
	whole, err := ctx.NewTextLayout(text, format, 220, 1<<24)
	if err != nil {
		t.Fatal(err)
	}
	_, full := whole.MeasureMetrics()
	whole.Destroy()
	at := strings.Index(text, "XYZ")
	window, err := ctx.NewTextLayout(text[at:], format, 220, 1<<24)
	if err != nil {
		t.Fatal(err)
	}
	w, h := window.MeasureSize()
	ls, cs := window.MeasureMetrics()
	f, err := textedit.NewFragment(textedit.Range{Start: at, End: len(text)}, geometry.Size{Width: w, Height: h}, ls, cs)
	window.Destroy()
	if err != nil {
		t.Fatal(err)
	}
	_, partial := f.Metrics()
	seenFull, seenPartial := false, false
	for _, c := range full {
		if c.Start == at {
			seenFull = true
			if c.Direction != typography.TextRightToLeft {
				t.Fatal("full paragraph lost RLO")
			}
		}
	}
	for _, c := range partial {
		if c.Start == 0 {
			seenPartial = true
			if c.Direction != typography.TextLeftToRight {
				t.Fatal("fixture no longer demonstrates missing RLO context")
			}
		}
	}
	if !seenFull || !seenPartial {
		t.Fatal("missing fixture target")
	}
}

func TestNativeCompletionEllipsisAndBaseline(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	v.context = ctx
	s := &completionSession{items: []Completion{{Label: "example"}}}
	l := newCompletionList(v, s)
	s.list = l
	v.completion = s
	defer l.release()
	for _, family := range []string{"monospace", "sans-serif"} {
		for _, prefix := range []string{"e\u0301", "你好", "👩‍💻", "שלום"} {
			func() {
				b := l.build()
				defer b.destroy()
				font := typography.FontInfo{Family: family, Size: 12}
				reference, err := b.text(prefix+"…", font, l.colors.Foreground)
				if err != nil {
					t.Fatal(err)
				}
				width := reference.width
				got, err := b.fit(prefix+strings.Repeat("ABC", 30), font, l.colors.Foreground, width+.01)
				if err != nil || got == nil {
					t.Fatal("fit", err)
				}
				if got.native.Text() != prefix+"…" || got.width > width+.01 || got.kept != len(prefix) {
					t.Fatalf("%s: got %q at width %g/%g", ctx.Name(), got.native.Text(), got.width, width)
				}
				if got.baseline <= 0 || got.baseline > got.height {
					t.Fatalf("%s: baseline %g outside height %g", ctx.Name(), got.baseline, got.height)
				}
			}()
		}
	}
}

// 这是未接入控件的有限输入实验，发布独立测量窗口前必须先验证上下文等价。
// TestFragmentSnapshotDoesNotInventMissingParagraphContext 的长 RLO 反例说明，
// 仅通过 ASCII 样本不能证明方案可用于任意段落。
type boundedFragmentProbe struct {
	pieces         []probeFragment
	maxInput       int
	first, longest time.Duration
}

// 片段的占用范围不包含下一行的间距。单独保存由重叠行基线确定的放置坐标。
type probeFragment struct {
	textedit.Fragment
	y float64
}

func probeBoundedFragments(ctx typography.Context, text string, format typography.TextFormat, width float32, limit int) (boundedFragmentProbe, error) {
	return probeBoundedFragmentsWithPrefix(ctx, text, format, width, limit, nil)
}

func probeBoundedFragmentsWithPrefix(ctx typography.Context, text string, format typography.TextFormat, width float32, limit int, prefixAt func(int) (string, error)) (boundedFragmentProbe, error) {
	var result boundedFragmentProbe
	var nextBaseline float64
	publishStart := 0
	for start := 0; publishStart < len(text); {
		began := time.Now()
		prefix := ""
		if prefixAt != nil {
			var err error
			prefix, err = prefixAt(start)
			if err != nil {
				return result, err
			}
		}
		if len(prefix) >= limit {
			return result, fmt.Errorf("probe: context consumes input limit")
		}
		end := min(len(text), start+limit-len(prefix))
		for end > start && end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		if end == start {
			return result, fmt.Errorf("probe: input limit cannot contain next rune")
		}
		native, err := ctx.NewTextLayout(prefix+text[start:end], format, width, 1<<24)
		if err != nil {
			return result, err
		}
		w, h := native.MeasureSize()
		lines, clusters := native.MeasureMetrics()
		lines, clusters, err = removeProbePrefix(lines, clusters, len(prefix))
		if err != nil {
			native.Destroy()
			return result, err
		}
		fragment, err := textedit.NewFragment(textedit.Range{Start: start, End: end}, geometry.Size{Width: w, Height: h}, lines, clusters)
		native.Destroy()
		if err != nil {
			return result, err
		}
		first := 0
		for first < len(lines) && start+lines[first].Start < publishStart {
			first++
		}
		if first == len(lines) || start+lines[first].Start != publishStart {
			return result, fmt.Errorf("probe: overlapping line changed its boundary")
		}
		keep := len(lines)
		if end < len(text) {
			// 保留下一条完整重叠行，并排除可能被输入上限截断的末行。
			keep -= 2
		}
		if keep <= first {
			return result, fmt.Errorf("probe: no complete line within bounded input")
		}
		fragment, err = fragment.LineRange(first, keep)
		if err != nil {
			return result, err
		}
		if fragment.Range().End <= publishStart {
			return result, fmt.Errorf("probe: no progress")
		}
		origin := fragment.Origin().Y
		y := float64(0)
		if len(result.pieces) > 0 {
			y = nextBaseline - float64(lines[first].Baseline-origin)
		}
		result.pieces = append(result.pieces, probeFragment{Fragment: fragment, y: y})
		if end < len(text) {
			nextBaseline = y + float64(lines[keep].Baseline-origin)
		}
		result.maxInput = max(result.maxInput, len(prefix)+end-start)
		elapsed := time.Since(began)
		if len(result.pieces) == 1 {
			result.first = elapsed
		}
		result.longest = max(result.longest, elapsed)
		publishStart = fragment.Range().End
		// 下一步从最后一条已发布的完整行开始；真实文本恢复数字等弱类型的
		// 近处上下文。首行仅供测量，不重复发布，也不插入可能改变字体度量的标记。
		start += lines[keep-1].Start
	}
	return result, nil
}

func TestBoundedFragmentProbeMatchesSimpleParagraphs(t *testing.T) {
	for _, sample := range []struct{ name, text string }{
		{"word", strings.Repeat("a", 8192)},
		{"json", strings.Repeat(`{"title":"hello world","value":1234},`, 150)},
	} {
		t.Run(sample.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ctx, err := nativeContext()
			if err != nil {
				t.Fatal(err)
			}
			defer ctx.Destroy()
			format := typography.TextFormat{Font: typography.FontInfo{Family: "monospace", Size: 12}, WrapMode: typography.WrapWordChar, TabWidth: 40}
			whole, err := ctx.NewTextLayout(sample.text, format, 220, 1<<24)
			if err != nil {
				t.Fatal(err)
			}
			wantLines, wantClusters := copyMetrics(whole)
			whole.Destroy()
			for _, limit := range []int{512, 2048} {
				result, err := probeBoundedFragments(ctx, sample.text, format, 220, limit)
				if err != nil {
					t.Fatal(err)
				}
				if result.maxInput > limit || len(result.pieces) < 2 {
					t.Fatal("input was not bounded", result.maxInput, len(result.pieces))
				}
				lineAt, clusterAt := 0, 0
				for _, piece := range result.pieces {
					y := piece.y
					lines, clusters := piece.Metrics()
					firstLine := lineAt
					for _, line := range lines {
						if lineAt >= len(wantLines) {
							t.Fatal("extra fragment line")
						}
						want := wantLines[lineAt]
						if line.Start+piece.Range().Start != want.Start || line.Length != want.Length || line.X != want.X || line.Width != want.Width || line.Height != want.Height || float32(float64(line.Y)+y) != want.Y || float32(float64(line.Baseline)+y) != want.Baseline {
							t.Fatalf("limit %d line %d: %+v y=%g want %+v", limit, lineAt, line, y, want)
						}
						lineAt++
					}
					for _, cluster := range clusters {
						if clusterAt >= len(wantClusters) {
							t.Fatal("extra fragment cluster")
						}
						want := wantClusters[clusterAt]
						if cluster.Start+piece.Range().Start != want.Start || cluster.Length != want.Length || cluster.X != want.X || cluster.Width != want.Width || cluster.Height != want.Height || cluster.Direction != want.Direction || cluster.LineIndex+firstLine != want.LineIndex || float32(float64(cluster.Y)+y) != want.Y {
							t.Fatalf("limit %d cluster %d differs: %+v y=%g want %+v", limit, clusterAt, cluster, y, want)
						}
						clusterAt++
					}
				}
				if lineAt != len(wantLines) || clusterAt != len(wantClusters) {
					t.Fatalf("incomplete output: %d/%d lines %d/%d clusters", lineAt, len(wantLines), clusterAt, len(wantClusters))
				}
			}
		})
	}
}

// 每行四字节、字形高 12、行距 15、首行顶部 2.75。占用高度与行推进不同，
// 不依赖宿主字体或原生库也能复现按片段 Size.Height 累加导致的位置漂移。
type probeGapContext struct{ typography.Context }

type probeGapLayout struct {
	typography.TextLayout
	text string
}

func (probeGapContext) NewTextLayout(text string, _ typography.TextFormat, _, _ float32) (typography.TextLayout, error) {
	return &probeGapLayout{text: text}, nil
}

func (p *probeGapLayout) Destroy() {}

func (p *probeGapLayout) MeasureSize() (float32, float32) {
	return 20, float32((len(p.text)+3)/4-1)*15 + 14.75
}

func (p *probeGapLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	var lines []typography.TextLine
	var clusters []typography.TextCluster
	for start := 0; start < len(p.text); start += 4 {
		index := len(lines)
		y, baseline := float32(index*15)+2.75, float32(index*15)+12
		length := min(4, len(p.text)-start)
		lines = append(lines, typography.TextLine{Start: start, Length: length, Y: y, Width: float32(length * 5), Height: 12, Baseline: baseline})
		for i := range length {
			clusters = append(clusters, typography.TextCluster{Start: start + i, Length: 1, X: float32(i * 5), Y: y, Width: 5, Height: 12, LineIndex: index})
		}
	}
	return lines, clusters
}

func TestBoundedProbeKeepsInterlineGaps(t *testing.T) {
	result, err := probeBoundedFragments(probeGapContext{}, strings.Repeat("a", 80), typography.TextFormat{}, 20, 32)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.pieces) < 2 || result.maxInput > 32 {
		t.Fatal("未保留有界的独立输入", len(result.pieces), result.maxInput)
	}
	next := 0
	for _, piece := range result.pieces {
		if piece.Range().Start != next {
			t.Fatal("片段遗漏或重复源文本", piece.Range(), next)
		}
		lines, clusters := piece.Metrics()
		for _, line := range lines {
			index := (piece.Range().Start + line.Start) / 4
			if float32(float64(line.Y)+piece.y) != float32(index*15)+2.75 || float32(float64(line.Baseline)+piece.y) != float32(index*15)+12 {
				t.Fatalf("行距丢失：byte=%d y=%g baseline=%g", piece.Range().Start+line.Start, float64(line.Y)+piece.y, float64(line.Baseline)+piece.y)
			}
		}
		for _, cluster := range clusters {
			index := (piece.Range().Start + cluster.Start) / 4
			if float32(float64(cluster.Y)+piece.y) != float32(index*15)+2.75 {
				t.Fatal("簇未使用与行相同的放置坐标", cluster)
			}
		}
		next = piece.Range().End
	}
	if next != 80 {
		t.Fatal("未覆盖完整文本", next)
	}
}

// Native timing probe only. Windows are not a general Unicode layout algorithm.
// No event-loop waits or painting are included; max-step-ms includes creation,
// measurement, metric export, snapshot construction and native destruction.
func BenchmarkBoundedFragmentProbe(b *testing.B) {
	text := strings.Repeat("a", 32768)
	for _, limit := range []int{0, 512, 2048} {
		b.Run(fmt.Sprintf("input-%d", limit), func(b *testing.B) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ctx, err := nativeContext()
			if err != nil {
				b.Fatal(err)
			}
			defer ctx.Destroy()
			format := typography.TextFormat{Font: typography.FontInfo{Family: "DejaVu Sans", Size: 12}, WrapMode: typography.WrapWordChar}
			warm, err := ctx.NewTextLayout("warm", format, 520, 1<<24)
			if err != nil {
				b.Fatal(err)
			}
			warm.MeasureMetrics()
			warm.Destroy()
			b.ResetTimer()
			var longest, first time.Duration
			for i := 0; i < b.N; i++ {
				if limit == 0 {
					start := time.Now()
					native, err := ctx.NewTextLayout(text, format, 520, 1<<24)
					if err != nil {
						b.Fatal(err)
					}
					w, h := native.MeasureSize()
					lines, clusters := native.MeasureMetrics()
					_, err = textedit.NewFragment(textedit.Range{End: len(text)}, geometry.Size{Width: w, Height: h}, lines, clusters)
					native.Destroy()
					if err != nil {
						b.Fatal(err)
					}
					elapsed := time.Since(start)
					first, longest = max(first, elapsed), max(longest, elapsed)
				} else {
					result, err := probeBoundedFragments(ctx, text, format, 520, limit)
					if err != nil {
						b.Fatal(err)
					}
					first, longest = max(first, result.first), max(longest, result.longest)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(first)/float64(time.Millisecond), "first-step-ms")
			b.ReportMetric(float64(longest)/float64(time.Millisecond), "max-step-ms")
		})
	}
}

// This is deliberately a test-only hypothesis: replaying explicit controls is
// not a Unicode bidi resolver. In particular it cannot resolve arbitrarily
// distant neutral characters, paired brackets, or first-strong isolates. Do
// not publish its results in a control based on the successful fixtures below.
type explicitContextProbe struct {
	text     string
	at       int
	base     rune
	controls []rune
}

func newExplicitContextProbe(text string) *explicitContextProbe {
	p := &explicitContextProbe{text: text, base: '\u200e'}
	isolateDepth := 0
	for _, r := range text {
		prop, _ := bidi.LookupRune(r)
		switch prop.Class() {
		case bidi.LRI, bidi.RLI, bidi.FSI:
			isolateDepth++
		case bidi.PDI:
			isolateDepth = max(0, isolateDepth-1)
		case bidi.L, bidi.R, bidi.AL:
			if isolateDepth == 0 {
				if prop.Class() != bidi.L {
					p.base = '\u200f'
				}
				return p
			}
		}
	}
	return p
}

func (p *explicitContextProbe) prefix(at int) (string, error) {
	if at < p.at || at > len(p.text) {
		return "", fmt.Errorf("context probe: nonmonotonic input")
	}
	for _, r := range p.text[p.at:at] {
		switch r {
		case '\u202a', '\u202b', '\u202d', '\u202e', '\u2066', '\u2067':
			// Overflow counters are intentionally not implemented by this
			// hypothesis. Refuse instead of silently dropping a control.
			if len(p.controls) == 60 {
				return "", fmt.Errorf("context probe: explicit depth not supported")
			}
			p.controls = append(p.controls, r)
		case '\u2068':
			return "", fmt.Errorf("context probe: first-strong isolate not resolved")
		case '\u202c':
			if n := len(p.controls); n > 0 && p.controls[n-1] != '\u2066' && p.controls[n-1] != '\u2067' {
				p.controls = p.controls[:n-1]
			}
		case '\u2069':
			for i := len(p.controls) - 1; i >= 0; i-- {
				if p.controls[i] == '\u2066' || p.controls[i] == '\u2067' {
					p.controls = p.controls[:i]
					break
				}
			}
		}
	}
	p.at = at
	return string(p.base) + string(p.controls), nil
}

// Map a synthetic, zero-width prefix out of native metrics. A prefix that
// acquires width, crosses a shaping cluster, or occupies a separate line is
// not safely removable. Do not discard genuine document controls/characters.
func removeProbePrefix(lines []typography.TextLine, clusters []typography.TextCluster, prefix int) ([]typography.TextLine, []typography.TextCluster, error) {
	if prefix == 0 {
		return lines, clusters, nil
	}
	if len(lines) == 0 || lines[0].Start != 0 || lines[0].Length < prefix || len(lines) > 1 && lines[1].Start <= prefix {
		return nil, nil, fmt.Errorf("context probe: prefix is not contained in first line")
	}
	mappedLines := make([]typography.TextLine, len(lines))
	for i, line := range lines {
		end := line.Start + line.Length
		line.Start = max(prefix, line.Start) - prefix
		line.Length = end - prefix - line.Start
		line.Clusters = nil
		mappedLines[i] = line
	}
	mappedClusters := make([]typography.TextCluster, 0, len(clusters))
	for _, cluster := range clusters {
		if cluster.Start < prefix {
			if cluster.Start+cluster.Length > prefix || cluster.Width != 0 {
				return nil, nil, fmt.Errorf("context probe: prefix changes a document cluster")
			}
			continue
		}
		cluster.Start -= prefix
		mappedClusters = append(mappedClusters, cluster)
	}
	return mappedLines, mappedClusters, nil
}

func fragmentProbeDifference(pieces []probeFragment, wantLines []typography.TextLine, wantClusters []typography.TextCluster) error {
	lineAt, clusterAt := 0, 0
	for _, piece := range pieces {
		y := piece.y
		lines, clusters := piece.Metrics()
		firstLine := lineAt
		for _, line := range lines {
			if lineAt >= len(wantLines) {
				return fmt.Errorf("extra fragment line")
			}
			line.Start += piece.Range().Start
			line.Y = float32(float64(line.Y) + y)
			line.Baseline = float32(float64(line.Baseline) + y)
			want := wantLines[lineAt]
			if line.Start != want.Start || line.Length != want.Length || line.X != want.X || line.Y != want.Y || line.Width != want.Width || line.Height != want.Height || line.Baseline != want.Baseline {
				return fmt.Errorf("line %d: %+v want %+v", lineAt, line, want)
			}
			lineAt++
		}
		for _, cluster := range clusters {
			if clusterAt >= len(wantClusters) {
				return fmt.Errorf("extra fragment cluster")
			}
			cluster.Start += piece.Range().Start
			cluster.Y = float32(float64(cluster.Y) + y)
			cluster.LineIndex += firstLine
			if cluster != wantClusters[clusterAt] {
				return fmt.Errorf("cluster %d: %+v want %+v", clusterAt, cluster, wantClusters[clusterAt])
			}
			clusterAt++
		}
	}
	if lineAt != len(wantLines) || clusterAt != len(wantClusters) {
		return fmt.Errorf("incomplete metrics: %d/%d lines, %d/%d clusters", lineAt, len(wantLines), clusterAt, len(wantClusters))
	}
	return nil
}

func TestExplicitContextFragmentProbe(t *testing.T) {
	for _, sample := range []struct{ name, text string }{
		{"longRLO", "start \u202e" + strings.Repeat("abcd 1234 ", 410) + "XYZ\u202c end"},
		{"longLRO", "אבג \u202d" + strings.Repeat("abcd 1234 ", 410) + "XYZ\u202c סוף"},
		{"longRLI", "start \u2067" + strings.Repeat("עברית العربية 123 ", 180) + "XYZ\u2069 end"},
		{"unicode", strings.Repeat("中文 e\u0301 ffi 👩‍💻 العربية עברית\t ", 130)},
	} {
		t.Run(sample.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ctx, err := nativeContext()
			if err != nil {
				t.Fatal(err)
			}
			defer ctx.Destroy()
			format := typography.TextFormat{Font: typography.FontInfo{Family: "DejaVu Sans", Size: 12}, WrapMode: typography.WrapWordChar, TabWidth: 40}
			whole, err := ctx.NewTextLayout(sample.text, format, 220, 1<<24)
			if err != nil {
				t.Fatal(err)
			}
			ls, cs := copyMetrics(whole)
			whole.Destroy()
			for _, limit := range []int{512, 2048} {
				context := newExplicitContextProbe(sample.text)
				result, err := probeBoundedFragmentsWithPrefix(ctx, sample.text, format, 220, limit, context.prefix)
				if err != nil {
					t.Fatal(err)
				}
				if result.maxInput > limit || len(result.pieces) < 2 {
					t.Fatal("context replay exceeded native input limit or did not split")
				}
				if err := fragmentProbeDifference(result.pieces, ls, cs); err != nil {
					t.Fatalf("limit %d: %v", limit, err)
				}
			}
		})
	}
}

func TestExplicitContextProbeKeepsNestedScopes(t *testing.T) {
	text := "a \u202eX\u2066Y\u202cZ\u2069W\u202cQ"
	p := newExplicitContextProbe(text)
	for _, step := range []struct{ through, want string }{
		{"X", "\u200e\u202e"},
		{"Y", "\u200e\u202e\u2066"},
		{"Z", "\u200e\u202e\u2066"}, // PDF cannot pop across the isolate.
		{"W", "\u200e\u202e"},
		{"Q", "\u200e"},
	} {
		at := strings.Index(text, step.through) + len(step.through)
		got, err := p.prefix(at)
		if err != nil || got != step.want {
			t.Fatalf("after %s: %q / %v, want %q", step.through, got, err, step.want)
		}
	}
}

func TestProbePrefixMappingPreservesDocumentControls(t *testing.T) {
	// Three injected bytes precede a genuine RLO and X in the document. Only
	// the injection may disappear; both document clusters retain their bytes.
	ls := []typography.TextLine{{Length: 7, Width: 10, Height: 20, Baseline: 15}}
	cs := []typography.TextCluster{
		{Start: 0, Length: 3, Height: 20},
		{Start: 3, Length: 3, X: 10, Height: 20, Direction: typography.TextRightToLeft},
		{Start: 6, Length: 1, Width: 10, Height: 20, Direction: typography.TextRightToLeft},
	}
	lines, clusters, err := removeProbePrefix(ls, cs, 3)
	if err != nil || len(clusters) != 2 || clusters[0].Start != 0 || clusters[0].Length != 3 || clusters[1].Start != 3 || lines[0].Length != 4 {
		t.Fatal("prefix mapping lost document bytes", lines, clusters, err)
	}
	if ls[0].Length != 7 || cs[1].Start != 3 {
		t.Fatal("prefix mapping mutated its native input")
	}
	for _, bad := range []struct {
		name string
		cs   []typography.TextCluster
	}{
		{"width", []typography.TextCluster{{Length: 3, Width: 1}}},
		{"crossing", []typography.TextCluster{{Length: 4}}},
	} {
		t.Run(bad.name, func(t *testing.T) {
			if _, _, err := removeProbePrefix(ls, bad.cs, 3); err == nil {
				t.Fatal("accepted a prefix that changes document geometry")
			}
		})
	}
}

// Passing explicit-control samples must not conceal implicit bidi context.
// These fixtures intentionally require more than the active-control prefix.
func TestExplicitContextProbeDoesNotResolveImplicitContext(t *testing.T) {
	for _, sample := range []struct{ name, text string }{
		{"neutral", "start אבג " + strings.Repeat(" . ", 1500) + "ד end"},
		{"numbers", "start אבג " + strings.Repeat("1234 ", 900) + "tail"},
		{"brackets", "start אבג (" + strings.Repeat(" . ", 1500) + "ד) end"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ctx, err := nativeContext()
			if err != nil {
				t.Fatal(err)
			}
			defer ctx.Destroy()
			format := typography.TextFormat{Font: typography.FontInfo{Family: "DejaVu Sans", Size: 12}, WrapMode: typography.WrapWordChar, TabWidth: 40}
			whole, err := ctx.NewTextLayout(sample.text, format, 220, 1<<24)
			if err != nil {
				t.Fatal(err)
			}
			ls, cs := copyMetrics(whole)
			whole.Destroy()
			context := newExplicitContextProbe(sample.text)
			result, err := probeBoundedFragmentsWithPrefix(ctx, sample.text, format, 220, 512, context.prefix)
			if err != nil {
				t.Fatal(err)
			}
			if err := fragmentProbeDifference(result.pieces, ls, cs); err == nil {
				t.Fatal("fixture no longer exposes implicit context dependency")
			} else {
				t.Log(err)
			}
		})
	}
}

func TestPlainIndexCompletionDoesNotMutateNativeColors(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("ordinary text without syntax colors")
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	if native.colorCalls != 0 {
		t.Fatal("plain layout overwrote the foreground already supplied in TextFormat")
	}
	v.Document().engine.Advance(time.Time{})
	if v.paragraph(0) != p || native.colorCalls != 0 || !p.style.tokenValid {
		t.Fatal("index completion changed native colors despite identical rendered text")
	}
}

func TestEquivalentTokenColorsDoNotMutateNativeLayout(t *testing.T) {
	v, _ := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	tokens, _, _ := v.Document().engine.Tokens(0)
	tokens = append([]highlight.Token(nil), tokens...)
	before := append([]color.NRGBA(nil), native.colors...)
	calls := native.colorCalls
	// Different lexical categories can resolve to the same theme color. Split
	// one token into adjacent ranges as well: rendered colors are unchanged.
	for i, token := range tokens {
		if token.End-token.Start > 1 {
			tokens = append(tokens[:i:i], append([]highlight.Token{{Start: token.Start, End: token.Start + 1, Kind: token.Kind}, {Start: token.Start + 1, End: token.End, Kind: token.Kind}}, tokens[i+1:]...)...)
			break
		}
	}
	style := p.style
	style.tokenVersion++
	if !v.colorParagraph(v.paragraphKey(0), p, style, tokens, false) || native.colorCalls != calls || p.style != style {
		t.Fatal("equivalent color ranges invalidated native text or failed to advance style identity")
	}
	for i, c := range before {
		if native.colors[i] != c {
			t.Fatal("unchanged colors were lost", i)
		}
	}
}

func TestParagraphColorsPreserveSelectionAndClearObsoleteTokens(t *testing.T) {
	v, _ := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	tokens, _, _ := v.Document().engine.Tokens(0)
	v.colors.SelectionForeground = color.NRGBA{R: 100, G: 10, B: 50, A: 255}
	// Exercise effective range construction directly; focus/lifetime validation
	// still uses the same inactive fixture as the other color tests.
	style := p.style
	style.focused, style.selection = true, textedit.Range{Start: 3, End: 9}
	if !v.colorParagraph(v.paragraphKey(0), p, style, tokens, false) {
		t.Fatal("selection color application failed")
	}
	for i := 3; i < 9; i++ {
		if native.colors[i] != v.colors.SelectionForeground {
			t.Fatal("selection did not override token color", i)
		}
	}
	style.focused, style.selection, style.tokenValid = false, textedit.Range{}, false
	style.language = language.JSON
	if !v.colorParagraph(v.paragraphKey(0), p, style, nil, false) {
		t.Fatal("clearing token and selection colors failed")
	}
	for i, c := range native.colors {
		if c != v.colors.Foreground {
			t.Fatal("obsolete token or selection foreground survived", i)
		}
	}
}

func TestColorChangesReuseParagraph(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	original := p.layout.(*shapedLayout)
	created := ctx.created
	height, width := v.heights.Total(), v.contentWidth
	for i := 0; i < 20; i++ {
		v.SetSelection(textedit.Selection{Anchor: i % 4, Caret: 8})
		v.SetScheme([]*scheme.Scheme{scheme.Dark(), scheme.Light()}[i%2])
		if got := v.paragraph(0); got != p || got.layout != original || ctx.created != created {
			t.Fatal("color-only change recreated native paragraph", ctx.created, created)
		}
		if v.heights.Total() != height || v.contentWidth != width {
			t.Fatal("color-only change invalidated size estimates")
		}
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("native layout leak", ctx.created, ctx.destroyed)
	}
}

func TestLanguageChangePreservesTextLayoutAndClearsTokens(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	if native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("fixture lacks property color", native.colors[2])
	}
	created := ctx.created
	id := v.Document().engine.Line(0).ID
	if err := v.Document().SetLanguage(language.Plain); err != nil {
		t.Fatal(err)
	}
	v.Document().engine.Advance(time.Time{})
	if v.Document().engine.Line(0).ID != id {
		t.Fatal("language changed text-line identity")
	}
	if got := v.paragraph(0); got != p || ctx.created != created {
		t.Fatal("language change recreated paragraph")
	}
	for _, c := range native.colors {
		if c != v.colors.Foreground {
			t.Fatal("obsolete token color remained", c)
		}
	}
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.Document().engine.Advance(time.Time{})
	if got := v.paragraph(0); got != p || ctx.created != created || native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("language recolor failed")
	}
}

func TestUnchangedColorStateDoesNoNativeWork(t *testing.T) {
	v, _ := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	calls := native.colorCalls
	for i := 0; i < 10; i++ {
		if v.paragraph(0) != p {
			t.Fatal("cache miss")
		}
	}
	if native.colorCalls != calls {
		t.Fatal("unchanged paragraph recolored")
	}
}

func TestColdIndexAdoptsPlainLayout(t *testing.T) {
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	v.Document().Model().SetText("true")
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	p := v.paragraph(0)
	created := ctx.created
	v.Document().engine.Advance(time.Time{})
	if got := v.paragraph(0); got != p || ctx.created != created {
		t.Fatal("initial index completion reshaped unchanged text")
	}
	native := p.layout.(*shapedLayout)
	for _, c := range native.colors {
		if c != v.colors.Tokens[language.KindKeyword] {
			t.Fatal("first lexical colors were not applied")
		}
	}
}

func TestRecolorReentryStopsUsingReleasedLayout(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	native.onColor = func() { native.onColor = nil; v.SetDocument(NewDocument("replacement")) }
	v.SetScheme(scheme.Dark())
	if got := v.paragraph(0); got != nil {
		t.Fatal("reentrant document switch returned stale paragraph")
	}
	if !native.dead || v.Document().Model().Text() != "replacement" {
		t.Fatal("reentry was not completed")
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("layout ownership leak", ctx.created, ctx.destroyed)
	}
}

func TestGutterRecolorKeepsNativeNumber(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	number, err := ctx.NewTextLayout("1", typography.TextFormat{TextColor: v.colors.GutterForeground}, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	p.number, p.numberText = number, "1"
	v.SetScheme(scheme.Dark())
	if v.paragraph(0) != p || p.number != number {
		t.Fatal("gutter color rebuilt native number")
	}
	if got := number.(*shapedLayout).colors[0]; got != v.colors.GutterForeground {
		t.Fatal("gutter retained old scheme", got)
	}
}

func TestSameIDWithoutTokenizerClearsNativeColors(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	if native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("fixture has no syntax color")
	}
	created := ctx.created
	id := v.document.engine.Line(0).ID
	uncolored, err := language.New(language.Definition{Info: language.Info{ID: language.JSON, Name: "Uncolored JSON"}})
	if err != nil {
		t.Fatal(err)
	}
	v.document.SetLanguageDefinition(uncolored)
	if got := v.paragraph(0); got != p || ctx.created != created || v.document.engine.Line(0).ID != id {
		t.Fatal("definition recreated text layout or line identity")
	}
	for _, c := range native.colors {
		if c != v.colors.Foreground {
			t.Fatal("same-ID definition retained obsolete syntax colors", c)
		}
	}
	if err := v.document.SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.document.engine.Advance(time.Time{})
	if got := v.paragraph(0); got != p || ctx.created != created || native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("same-ID tokenizer restore failed")
	}
}

func TestSelectionStylesPreserveSyntaxByDefault(t *testing.T) {
	for _, sheet := range []style.StyleSheet{basic.Sheet(), modern.Sheet(modern.Options{}), modern.Sheet(modern.Options{Dark: true})} {
		selected := sheet.Resolve(style.Sel{Name: "source-view", Part: "selection"})
		fg, ok := selected.ForegroundColor()
		if !ok || nrgba(fg) != (color.NRGBA{}) {
			t.Fatal("default selection inherited a uniform foreground", fg)
		}
		if c := background(selected); c.A != 64 {
			t.Fatal("selection background is preblended or has wrong alpha", c)
		}
		if c := background(sheet.Resolve(style.Sel{Name: "source-view", Part: "inactive-selection"})); c.A != 32 {
			t.Fatal("inactive selection is opaque or too strong", c)
		}
	}
}

func TestSelectionForegroundOverridesAndReset(t *testing.T) {
	rules := modern.Rules(modern.Options{Dark: true})
	rules = append(rules, style.Name("source-view").ForegroundColor(rgb(0xeeddcc)))
	v := NewSourceView()
	scope := themedScope(v, style.Sheet(rules...))
	v.StyleChanged()
	if v.colors.SelectionForeground != (color.NRGBA{}) {
		t.Fatal("root foreground leaked into selection")
	}
	rules = append(rules, style.Name("source-view").Part("selection").ForegroundColor(rgb(0xffeeaa)))
	scope.SetStyleSheet(style.Sheet(rules...))
	v.StyleChanged()
	if v.colors.SelectionForeground != rgb(0xffeeaa) {
		t.Fatal("explicit selection foreground was ignored")
	}
	rules = append(rules, style.Name("source-view").Part("selection").ForegroundColor(color.NRGBA{R: 255}))
	scope.SetStyleSheet(style.Sheet(rules...))
	v.StyleChanged()
	if v.colors.SelectionForeground != (color.NRGBA{}) {
		t.Fatal("reset did not restore syntax foregrounds")
	}
	before := v.appliedScheme
	v.StyleChanged()
	if v.appliedScheme != before {
		t.Fatal("ignored RGB in an unset foreground invalidated the scheme cache")
	}
}

func TestSelectionMovementWithoutOverrideDoesNotRecolor(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	calls, created := native.colorCalls, ctx.created
	height, width := v.heights.Total(), v.contentWidth
	for _, selected := range []textedit.Selection{{Anchor: 0, Caret: 7}, {Anchor: 12, Caret: 2}, {}, {Anchor: 0, Caret: 15}} {
		v.SetSelection(selected)
		if v.paragraph(0) != p || native.colorCalls != calls || ctx.created != created || v.heights.Total() != height || v.contentWidth != width {
			t.Fatal("background-only selection changed native colors or geometry")
		}
		if native.colors[2] != scheme.Light().Color(language.KindProperty) || native.colors[10] != scheme.Light().Color(language.KindString) {
			t.Fatal("selection changed property/string syntax colors")
		}
	}
}
