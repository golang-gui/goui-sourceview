package sourceview

import (
	"bytes"
	"math"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
)

func collapsedLineFixture(t *testing.T, text string) (*SourceView, *countedNativeContext) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	native, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(native.Destroy)
	ctx := &countedNativeContext{Context: native}
	v := NewSourceView()
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false)
	v.SetWrapMode(WrapWordChar)
	v.Document().Model().SetText(text)
	observe(t, v)
	v.context = ctx
	t.Cleanup(func() {
		v.dropLayouts()
		if ctx.created != ctx.destroyed {
			t.Errorf("line collapse leaked native layouts: %d created / %d destroyed", ctx.created, ctx.destroyed)
		}
	})
	v.Arrange(geometry.Rect(0, 0, 400, 180))
	return v, ctx
}

func TestLongLineCollapsePreservesSourceAndNativeLayout(t *testing.T) {
	text := "prefix\n" + strings.Repeat("wrapped 中文 word ", 500) + "\nsuffix"
	v, ctx := collapsedLineFixture(t, text)
	r, _ := v.Document().Model().LineRange(1)
	p := v.paragraph(1)
	fullHeight, created := p.height, ctx.created
	revision, undo := v.Document().Model().Revision(), v.Document().Model().CanUndo()
	v.SetSelection(textedit.Selection{Anchor: r.End - 10, Caret: r.End - 10})
	if !v.CollapseLine(1) || !slices.Equal(v.CollapsedLines(), []int{1}) {
		t.Fatal("long wrapped line was not shortened")
	}
	if v.Selection().Caret < r.Start || v.collapsedOffsetHidden(1, p, v.Selection().Caret, v.upstream) {
		t.Fatal("collapse left the selection endpoint hidden")
	}
	_, height := v.linePreview(p)
	if v.paragraphTop(2)-v.paragraphTop(1) != height || v.paragraph(1) != p || p.height != fullHeight || ctx.created != created {
		t.Fatal("collapse discarded native geometry or retained the full display height")
	}
	// Endpoints in surrounding visible rows select the entire original line,
	// including its hidden suffix. Selection and export use model coordinates.
	v.SetSelection(textedit.Selection{Caret: len(text)})
	selected, err := v.Document().Model().Slice(v.Selection().Range())
	if err != nil || selected != text || !v.lineCollapsed(1) {
		t.Fatal("cross-line selection lost hidden source or unexpectedly expanded it", err)
	}
	var saved bytes.Buffer
	if _, err := v.Document().WriteTo(&saved); err != nil || saved.String() != text {
		t.Fatal("collapsed export changed source", err)
	}
	if v.Document().Model().Revision() != revision || v.Document().Model().CanUndo() != undo {
		t.Fatal("display collapse changed revision or history")
	}
	if !v.ExpandLine(1) || v.paragraphTop(2)-v.paragraphTop(1) != fullHeight || v.paragraph(1) != p || ctx.created != created {
		t.Fatal("expansion did not restore the original measured paragraph")
	}
}

func TestLongLineCollapseResizeAndWrapIntent(t *testing.T) {
	v, _ := collapsedLineFixture(t, strings.Repeat("wrapped words ", 500)+"\ntail")
	if !v.CollapseLine(0) {
		t.Fatal("fixture cannot collapse")
	}
	for _, width := range []float32{220, 650, 300} {
		v.Arrange(geometry.Rect(0, 0, width, 180))
		p := v.paragraph(0)
		_, height := v.linePreview(p)
		if !v.lineCollapsed(0) || v.paragraphTop(1) != height || v.lineCollapseBadge(0, p).X+lineCollapseBadgeWidth > v.textRight() {
			t.Fatal("resize lost collapse intent or preview geometry", width)
		}
	}
	v.SetWrapMode(WrapNone)
	v.Arrange(v.Rect())
	if v.lineCollapsed(0) || !slices.Equal(v.CollapsedLines(), []int{0}) {
		t.Fatal("temporarily disabling wrap discarded intent or kept clipping")
	}
	v.SetWrapMode(WrapWordChar)
	v.Arrange(v.Rect())
	if !v.lineCollapsed(0) || !v.ExpandLine(0) || len(v.CollapsedLines()) != 0 {
		t.Fatal("restoring wrapping lost the manual collapse")
	}
}

func TestLongLineCollapseSelectionSearchAndNavigation(t *testing.T) {
	for _, action := range []string{"selection", "search", "down", "end"} {
		t.Run(action, func(t *testing.T) {
			text := strings.Repeat("wrapped words ", 400) + "HIDDEN_NEEDLE\ntail"
			v, _ := collapsedLineFixture(t, text)
			if !v.CollapseLine(0) {
				t.Fatal("fixture cannot collapse")
			}
			switch action {
			case "selection":
				at := strings.Index(text, "HIDDEN_NEEDLE")
				v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
			case "search":
				v.SetSearch(SearchQuery{Text: "HIDDEN_NEEDLE"})
				if _, ok := v.FindNext(false); !ok {
					t.Fatal("hidden source could not be found")
				}
			case "down":
				v.navigate(events.KeyArrowDown, false)
			case "end":
				v.navigate(events.KeyEnd, false)
			}
			if v.lineCollapsed(0) {
				t.Fatal("navigation into hidden content did not expand the line")
			}
		})
	}
}

func TestLongLineCollapseEditAnchorsAndReset(t *testing.T) {
	v, _ := collapsedLineFixture(t, "prefix\n"+strings.Repeat("wrapped words ", 500)+"\ntail")
	v.CollapseLine(1)
	if err := v.Document().Model().Replace(textedit.Range{}, "new prefix\n"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(v.CollapsedLines(), []int{2}) {
		t.Fatal("edit before a collapsed line lost its byte anchor", v.CollapsedLines())
	}
	r, _ := v.Document().Model().LineRange(2)
	if err := v.Document().Model().Replace(textedit.Range{Start: r.Start + 5, End: r.Start + 5}, "x"); err != nil {
		t.Fatal(err)
	}
	if len(v.CollapsedLines()) != 0 {
		t.Fatal("editing a collapsed line retained obsolete preview state")
	}
	v.Arrange(v.Rect())
	if !v.CollapseLine(2) {
		t.Fatal("edited line could not be collapsed again")
	}
	v.Document().Model().SetText(strings.Repeat("replacement words ", 400))
	if len(v.CollapsedLines()) != 0 {
		t.Fatal("reset retained stale collapse state")
	}
	v.Arrange(v.Rect())
	v.CollapseLine(0)
	v.SetDocument(NewDocument("other"))
	if len(v.CollapsedLines()) != 0 {
		t.Fatal("document switch retained collapse state")
	}
}

func TestLongLineCollapseCachedHitSnapshotAndEligibility(t *testing.T) {
	v, ctx := collapsedLineFixture(t, strings.Repeat("wrapped words ", 500)+"\ntail")
	v.SetReadOnly(true)
	if !v.CollapseLine(0) || v.CollapseLine(0) || v.CollapseLine(1) || v.CollapseLine(-1) {
		t.Fatal("collapse eligibility or read-only behavior is incorrect")
	}
	p := v.cachedParagraph(0)
	badge := v.lineCollapseBadge(0, p)
	point := geometry.Point{X: badge.X + badge.Width/2, Y: badge.Y + badge.Height/2}
	before := ctx.created
	if row, hit := v.collapsedLineHit(point); !hit || row != 0 || v.cursorAt(point) != gui.CursorPointing {
		t.Fatal("visible expand badge is not interactive")
	}
	info := v.Snapshot()
	if info.TextEditing.VisibleRange.End >= len(v.Document().Model().Text()) || len(info.Children) != 1 || info.Children[0].Role != gui.RoleButton || len(info.Children[0].Actions) != 1 {
		t.Fatal("snapshot included hidden content or omitted the visible expand control", info)
	}
	if ctx.created != before {
		t.Fatal("hover or semantic snapshot created native layouts")
	}
	copy := v.CollapsedLines()
	copy[0] = 100
	if !v.lineCollapsed(0) {
		t.Fatal("caller mutated the view through a detached snapshot")
	}
	if !v.ExpandLine(0) {
		t.Fatal("read-only view could not expand")
	}
}

func TestLongLineCollapseBidiHitAndCaret(t *testing.T) {
	v, _ := collapsedLineFixture(t, strings.Repeat("مرحبا بالعالم ", 800)+"\ntail")
	if !v.CollapseLine(0) {
		t.Fatal("RTL long line could not collapse")
	}
	p := v.cachedParagraph(0)
	if v.collapsedOffsetHidden(0, p, v.Selection().Caret, v.upstream) {
		t.Fatal("collapse left the RTL caret outside the preview")
	}
	r := v.linePreviewRect(0, p)
	pos, _, ok := v.hitCaret(geometry.Point{X: r.X + r.Width - 1, Y: r.Y + r.Height/2})
	if !ok || v.collapsedOffsetHidden(0, p, pos.Offset, pos.Upstream) {
		t.Fatal("preview hit selected an invisible bidi caret")
	}
}

func TestLongLineCollapsePreviewDoesNotCutClusters(t *testing.T) {
	v, _ := collapsedLineFixture(t, strings.Repeat("wrapped 中文 e\u0301 words ", 500))
	v.CollapseLine(0)
	for _, width := range []float32{210.3, 317.4, 400, 518.7} {
		v.Arrange(geometry.Rect(0, 0, width, 180))
		p := v.paragraph(0)
		right := v.linePreviewRect(0, p).Width
		_, clusters := p.layout.MeasureMetrics()
		for _, c := range clusters {
			if c.LineIndex == 0 && c.Width > 0 && c.X < right-.01 && c.X+c.Width > right+.01 {
				t.Fatal("preview ends inside a native shaping cluster", width, right, c)
			}
		}
		g, _ := v.linePreview(p)
		pos := g.HitLine(0, right)
		if math.Abs(float64(g.Caret(pos).X-right)) > .01 {
			t.Fatal("preview edge does not match an actual visual stop", width)
		}
	}
}

func TestLongLineCollapsePagedText(t *testing.T) {
	text := strings.Repeat("paged words ", 10000) + "\ntail"
	v, _ := collapsedLineFixture(t, text)
	if v.paragraph(0).paged == nil || !v.CollapseLine(0) {
		t.Fatal("paged fixture did not collapse")
	}
	v.Arrange(v.Rect())
	p := v.paragraph(0)
	_, height := v.linePreview(p)
	if v.paragraphTop(1) != height || p.geometryY != 0 || v.Snapshot().TextEditing.VisibleRange.End > 1024 {
		t.Fatal("paged preview used distant geometry or exposed its hidden suffix")
	}
	v.SetSelection(textedit.Selection{Anchor: 100000, Caret: 100000})
	if len(v.CollapsedLines()) != 0 || v.Selection().Caret != 100000 || v.Document().Model().Text() != text {
		t.Fatal("navigation into a distant page failed to expand or changed source")
	}
}

type previewClipPainter struct {
	recordingPainter
	clip      geometry.Rectangle
	stack     []geometry.Rectangle
	bodyClips []geometry.Rectangle
}

func (p *previewClipPainter) Save() { p.stack = append(p.stack, p.clip) }
func (p *previewClipPainter) Restore() {
	p.clip = p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
}
func (p *previewClipPainter) SetClipRect(r geometry.Rectangle) { p.clip = r }
func (p *previewClipPainter) DrawTextLayout(at geometry.Point, l typography.TextLayout) {
	if strings.HasPrefix(l.Text(), "paged words") {
		p.bodyClips = append(p.bodyClips, p.clip)
	}
	p.recordingPainter.DrawTextLayout(at, l)
}

func TestLongLineCollapsePagedPaintKeepsPreviewClip(t *testing.T) {
	v, _ := collapsedLineFixture(t, strings.Repeat("paged words ", 10000)+"\ntail")
	v.CollapseLine(0)
	v.Arrange(v.Rect())
	want := v.linePreviewRect(0, v.paragraph(0))
	p := &previewClipPainter{}
	v.Paint(p)
	if len(p.bodyClips) == 0 || len(p.stack) != 0 {
		t.Fatal("paged preview was not painted or left saved state")
	}
	for _, clip := range p.bodyClips {
		if clip.Width <= 0 || clip.Height <= 0 || clip.Intersect(want) != clip {
			t.Fatal("inner page clip widened the one-row preview", clip, want)
		}
	}
}

func TestLongLineCollapseUpFromFollowingParagraph(t *testing.T) {
	text := strings.Repeat("paged words ", 10000) + "\ntail"
	v, _ := collapsedLineFixture(t, text)
	v.CollapseLine(0)
	v.SetSelection(textedit.Selection{Anchor: len(text) - 1, Caret: len(text) - 1})
	v.navigate(events.KeyArrowUp, false)
	if len(v.CollapsedLines()) != 0 || v.Selection().Caret < len(text)-256 || v.Selection().Caret >= len(text)-4 {
		t.Fatal("Up into a collapsed paged line did not reach its last display line", v.Selection())
	}
}

func TestLongLineCollapseCompositionAndReentry(t *testing.T) {
	for _, reenter := range []bool{false, true} {
		t.Run(map[bool]string{false: "preedit", true: "document callback"}[reenter], func(t *testing.T) {
			text := strings.Repeat("wrapped words ", 500)
			v, _ := collapsedLineFixture(t, text)
			focusBracketFixture(t, v)
			v.CollapseLine(0)
			if reenter {
				v.ConnectFoldingChanged(func() { v.SetDocument(NewDocument("other")) })
			}
			v.applyPreedit("中", 3)
			if len(v.CollapsedLines()) != 0 {
				t.Fatal("composition retained collapsed content")
			}
			if reenter {
				if v.projection != nil || v.Document().Model().Text() != "other" {
					t.Fatal("composition continued after its document was replaced")
				}
			} else {
				if v.projection == nil || v.Document().Model().Text() != text {
					t.Fatal("preedit changed committed text or did not create a projection")
				}
				v.cancelPreedit()
			}
		})
	}
}

func TestLongLineCollapseSurvivesOtherParagraphComposition(t *testing.T) {
	text := "short\n" + strings.Repeat("wrapped words ", 500) + "\ntail"
	v, _ := collapsedLineFixture(t, text)
	focusBracketFixture(t, v)
	v.CollapseLine(1)
	v.SetSelection(textedit.Selection{})
	v.applyPreedit("中\n文", 7)
	v.Arrange(v.Rect())
	if !v.lineCollapsed(2) || !slices.Equal(v.CollapsedLines(), []int{1}) {
		t.Fatal("preedit lost another paragraph's source collapse identity")
	}
	_, height := v.linePreview(v.paragraph(2))
	if v.paragraphTop(3)-v.paragraphTop(2) != height {
		t.Fatal("composition expanded an unrelated long paragraph")
	}
	info := v.Snapshot()
	if info.TextEditing.Preedit == nil || info.TextEditing.VisibleRange.Start != info.TextEditing.VisibleRange.End {
		t.Fatal("composition snapshot exposed hidden committed text")
	}
	if len(info.Children) != 1 || info.Children[0].Enabled || len(info.Children[0].Actions) != 0 {
		t.Fatal("composition preview semantics did not suspend its action")
	}
	v.cancelPreedit()
	v.Arrange(v.Rect())
	if !v.lineCollapsed(1) || v.Document().Model().Text() != text {
		t.Fatal("composition cancellation changed collapse intent or source")
	}
}

func TestLongLineCollapseFirstPaintUsesFinalViewport(t *testing.T) {
	v, ctx := collapsedLineFixture(t, strings.Repeat("wrapped words ", 500)+"\ntail")
	v.frameLayout = true
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	for range 4 {
		scroll.Arrange(geometry.Rect(0, 0, 400, 180))
		v.layoutStep(time.Time{})
	}
	for _, collapsed := range []bool{true, false} {
		if collapsed {
			if !v.CollapseLine(0) {
				t.Fatal("fixture could not collapse")
			}
		} else if !v.ExpandLine(0) {
			t.Fatal("fixture could not expand")
		}
		v.layoutStep(time.Time{})
		scroll.Arrange(scroll.Rect())
		if v.cachedParagraph(0) == nil {
			t.Fatal("first collapse/expand paint has no final-width paragraph", collapsed)
		}
		before := ctx.created
		p := &recordingPainter{}
		v.Paint(p)
		if !slices.Contains(p.order, "text") || ctx.created != before {
			t.Fatal("collapse/expand paint lost text or shaped it synchronously")
		}
	}
}

func TestLongLineCollapseResizesWithoutEmptyPaint(t *testing.T) {
	v, ctx := collapsedLineFixture(t, strings.Repeat("wrapped words ", 500)+"\ntail")
	v.frameLayout = true
	v.CollapseLine(0)
	v.Paint(&recordingPainter{})
	for _, width := range []float32{220.4, 310.7, 400, 220.4} {
		v.layoutStep(time.Time{})
		v.Arrange(geometry.Rect(0, 0, width, 180))
		if v.cachedParagraph(0) == nil {
			t.Fatal("collapsed preview waits for a later frame after resizing", width)
		}
		before := ctx.created
		v.Paint(&recordingPainter{})
		if ctx.created != before {
			t.Fatal("collapsed resize moved native work into Paint")
		}
	}
}
