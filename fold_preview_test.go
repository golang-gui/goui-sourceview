package sourceview

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

func wrappedFoldFixture(t *testing.T) (*SourceView, string) {
	t.Helper()
	text := `{"meta":"` + strings.Repeat("wrapped header ", 120) + `HEADER_NEEDLE","content":` + "\n\n" + `"` + strings.Repeat("PNG_PAYLOAD", 100) + `"}` + "\ntail"
	v, _ := collapsedLineFixture(t, text)
	v.SetFoldingEnabled(true)
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.acceptFolds([]language.FoldRange{{StartLine: 0, EndLine: 2}})
	v.Arrange(v.Rect())
	return v, text
}

func TestWrappedLongLineHasGutterEntryWithoutLanguageFolds(t *testing.T) {
	v, ctx := collapsedLineFixture(t, strings.Repeat("plain wrapped text ", 500)+"\ntail")
	v.SetFoldingEnabled(true)
	v.Arrange(v.Rect())
	gutter, created := v.gutter, ctx.created
	r := v.foldButtonRect(0)
	if v.foldColumn == 0 {
		t.Fatal("wrapped plain text has no fold column")
	}
	if row, ok := v.foldHit(r.Center()); !ok || row != 0 {
		t.Fatal("long line has no gutter target")
	}
	if _, ok := v.foldHit(geometry.Point{X: r.Center().X, Y: r.Y + r.Height*2}); ok {
		t.Fatal("continuation row repeats the logical-line arrow")
	}
	info := v.Snapshot()
	if len(info.Children) != 1 || info.Children[0].Attributes["goui.sourceview.fold"].(map[string]any)["kind"] != "line" || ctx.created != created {
		t.Fatal("candidate snapshot omitted the real entry or created a layout")
	}
	if !v.toggleGutterFold(0) || !slices.Equal(v.CollapsedLines(), []int{0}) {
		t.Fatal("gutter did not shorten the long line")
	}
	if v.gutter != gutter || !v.toggleGutterFold(0) || len(v.CollapsedLines()) != 0 {
		t.Fatal("expansion changed gutter width or retained intent")
	}
	v.SetFoldingEnabled(false)
	v.Arrange(v.Rect())
	if v.foldColumn != 0 || !v.CollapseLine(0) {
		t.Fatal("hiding the folding column broke explicit long-line collapse")
	}
}

func TestWrappedStructuralFoldShortensHeaderAndHasOneArrow(t *testing.T) {
	v, original := wrappedFoldFixture(t)
	m := v.Document().Model()
	revision, undo := m.Revision(), m.CanUndo()
	header, _ := m.LineRange(0)
	v.SetSelection(textedit.Selection{Anchor: header.End - 3, Caret: header.End - 3})
	if !v.toggleGutterFold(0) {
		t.Fatal("structure did not collapse")
	}
	v.Arrange(v.Rect())
	p := v.paragraph(0)
	_, height := v.linePreview(p)
	if !v.folds[0].Collapsed || !v.lineCollapsed(0) || v.paragraphTop(3) != height || p.height <= height {
		t.Fatal("structural fold kept the wrapped header's full height")
	}
	if len(v.CollapsedLines()) != 0 || v.collapsedOffsetHidden(0, p, v.Selection().Caret, v.upstream) {
		t.Fatal("derived preview leaked manual intent or left a hidden caret")
	}
	arrows, badges := 0, 0
	info := v.Snapshot()
	for _, c := range info.Children {
		if meta, ok := c.Attributes["goui.sourceview.fold"].(map[string]any); ok {
			arrows++
			if meta["startLine"] != 0 || meta["endLine"] != 2 || meta["kind"] != "structure" || meta["collapsed"] != true {
				t.Fatal("incorrect logical structural range", meta)
			}
		}
		if _, ok := c.Attributes["goui.sourceview.collapsedLine"]; ok {
			badges++
		}
	}
	if arrows != 1 || badges != 1 || strings.Contains(info.TextEditing.VisibleText, "HEADER_NEEDLE") || strings.Contains(info.TextEditing.VisibleText, "PNG_PAYLOAD") {
		t.Fatal("duplicate controls or hidden text exposed", arrows, badges)
	}
	if v.foldBadgeRect(0, p).Width != 0 {
		t.Fatal("legacy structure badge duplicates preview badge")
	}
	if !v.expandLinePreview(0) || v.folds[0].Collapsed || v.lineCollapsed(0) || m.Text() != original || m.Revision() != revision || m.CanUndo() != undo {
		t.Fatal("expansion did not restore source-preserving structure state")
	}
}

func TestWrappedFoldCombinedIntentExpandsAtomically(t *testing.T) {
	v, _ := wrappedFoldFixture(t)
	if !v.CollapseLine(0) || !v.Fold(0) {
		t.Fatal("fixture cannot hold both intents")
	}
	changes := 0
	v.ConnectFoldingChanged(func() {
		changes++
		if v.folds[0].Collapsed || len(v.CollapsedLines()) != 0 {
			t.Error("callback observed partly expanded state")
		}
	})
	if !v.toggleGutterFold(0) || changes != 1 {
		t.Fatal("combined gutter expansion was not atomic", changes)
	}
}

func TestWrappedStructuralHeaderReveal(t *testing.T) {
	for _, action := range []string{"selection", "search", "down", "end", "delete boundary"} {
		t.Run(action, func(t *testing.T) {
			v, original := wrappedFoldFixture(t)
			if !v.Fold(0) {
				t.Fatal("fixture cannot fold")
			}
			switch action {
			case "selection":
				at := strings.Index(original, "HEADER_NEEDLE")
				v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
			case "search":
				v.SetSearch(SearchQuery{Text: "HEADER_NEEDLE"})
				if _, ok := v.FindNext(false); !ok {
					t.Fatal("hidden header cannot be found")
				}
			case "down":
				v.navigate(events.KeyArrowDown, false)
			case "end":
				v.navigate(events.KeyEnd, false)
			case "delete boundary":
				if !v.unfoldDeleteBoundary(true) {
					t.Fatal("delete did not reveal a shortened header")
				}
			}
			if v.folds[0].Collapsed || v.lineCollapsed(0) || v.Document().Model().Text() != original {
				t.Fatal("hidden-header reveal failed or changed source")
			}
		})
	}
}

func TestWrappedFoldPreviewResizeWrapAndFirstPaint(t *testing.T) {
	v, _ := wrappedFoldFixture(t)
	v.frameLayout = true
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	for range 4 {
		scroll.Arrange(geometry.Rect(0, 0, 400, 180))
		v.layoutStep(time.Time{})
	}
	for _, collapsed := range []bool{true, false, true} {
		if collapsed {
			if !v.Fold(0) {
				t.Fatal("fold failed")
			}
		} else if !v.Unfold(0) {
			t.Fatal("unfold failed")
		}
		scroll.Arrange(scroll.Rect())
		if v.cachedParagraph(0) == nil {
			t.Fatal("first structure transition paint lacks current-width paragraph", collapsed)
		}
		paint := &recordingPainter{}
		v.Paint(paint)
		if !slices.Contains(paint.order, "text") {
			t.Fatal("structure transition paint lost text")
		}
	}
	for _, width := range []float32{220.4, 600, 310.7} {
		v.Arrange(geometry.Rect(0, 0, width, 180))
		p := v.cachedParagraph(0)
		if p == nil || v.paragraphTop(3) > v.lineHeight*2 {
			t.Fatal("resize expanded or lost folded header")
		}
	}
	v.SetWrapMode(WrapNone)
	v.Arrange(v.Rect())
	if v.lineCollapsed(0) || !v.folds[0].Collapsed {
		t.Fatal("wrap-off changed structural intent")
	}
	v.SetWrapMode(WrapWordChar)
	v.Arrange(v.Rect())
	if !v.lineCollapsed(0) {
		t.Fatal("wrap-on failed to restore structural preview")
	}
}

func TestWrappedFoldPreviewCallbackAndOtherParagraphIME(t *testing.T) {
	v, _ := wrappedFoldFixture(t)
	v.Fold(0)
	// Composition in a subsequent visible paragraph preserves this preview.
	at := v.Document().Model().Len()
	v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
	focusBracketFixture(t, v)
	v.applyPreedit("中\n文", 7)
	v.Arrange(v.Rect())
	if !v.lineCollapsed(0) || v.paragraphTop(3) > v.lineHeight*2 {
		t.Fatal("unrelated preedit expanded the structural preview")
	}
	for _, c := range v.Snapshot().Children {
		if c.Enabled || len(c.Actions) > 0 {
			t.Fatal("preedit left an active folding action")
		}
	}
	v.cancelPreedit()
	v.ConnectFoldingChanged(func() { v.SetDocument(NewDocument("replacement")) })
	v.expandLinePreview(0)
	if v.Document().Model().Text() != "replacement" {
		t.Fatal("expansion callback was overwritten")
	}
}

func TestWrappedFoldDisabledControlsRemainVisible(t *testing.T) {
	v, _ := wrappedFoldFixture(t)
	v.SetEnabled(false)
	info := v.Snapshot()
	arrows := 0
	for _, c := range info.Children {
		if _, ok := c.Attributes["goui.sourceview.fold"]; ok {
			arrows++
			if c.Enabled || len(c.Actions) > 0 {
				t.Fatal("disabled folding entry is actionable")
			}
		}
	}
	if arrows == 0 {
		t.Fatal("disabled state removed visible folding entries")
	}
	if _, ok := v.foldHit(v.foldButtonRect(0).Center()); ok || v.toggleGutterFold(0) {
		t.Fatal("disabled folding entry accepted input")
	}
}

func TestWrappedCombinedFoldSearchExpandsBothIntents(t *testing.T) {
	v, _ := wrappedFoldFixture(t)
	v.CollapseLine(0)
	v.Fold(0)
	if v.canCommand(CommandExpandLine) {
		t.Fatal("combined state offered a manual-only expansion that leaves the preview unchanged")
	}
	v.SetSearch(SearchQuery{Text: "PNG_PAYLOAD"})
	if _, ok := v.FindNext(false); !ok || v.folds[0].Collapsed || len(v.CollapsedLines()) != 0 {
		t.Fatal("search only partly expanded a combined fold")
	}
}

func TestWrappedFoldCompositionExpandsTargetBeforeCallback(t *testing.T) {
	for _, reenter := range []bool{false, true} {
		t.Run(map[bool]string{false: "target", true: "callback"}[reenter], func(t *testing.T) {
			v, original := wrappedFoldFixture(t)
			focusBracketFixture(t, v)
			v.Fold(0)
			if reenter {
				v.ConnectFoldingChanged(func() { v.SetDocument(NewDocument("replacement")) })
			}
			v.applyPreedit("中", 3)
			if reenter {
				if v.Document().Model().Text() != "replacement" || v.projection != nil {
					t.Fatal("preedit continued after the expansion callback replaced its document")
				}
			} else {
				if v.folds[0].Collapsed || v.lineCollapsed(0) || v.projection == nil || v.Document().Model().Text() != original {
					t.Fatal("IME did not reveal its structural header before projection")
				}
				v.cancelPreedit()
			}
		})
	}
}

func TestWrappedPagedStructuralHeaderKeepsPreviewClip(t *testing.T) {
	text := strings.Repeat("paged words ", 10000) + "HEADER_END\n child\nend\ntail"
	v, _ := collapsedLineFixture(t, text)
	v.SetFoldingEnabled(true)
	v.acceptFolds([]language.FoldRange{{StartLine: 0, EndLine: 2}})
	v.Arrange(v.Rect())
	if v.paragraph(0).paged == nil || !v.Fold(0) {
		t.Fatal("paged structural fixture did not fold")
	}
	v.Arrange(v.Rect())
	p := v.cachedParagraph(0)
	want := v.linePreviewRect(0, p)
	draw := &previewClipPainter{}
	v.Paint(draw)
	if len(draw.bodyClips) == 0 || v.paragraphTop(3) != want.Height {
		t.Fatal("paged structural header was absent or retained its full height")
	}
	for _, clip := range draw.bodyClips {
		if clip.Width <= 0 || clip.Height <= 0 || clip.Intersect(want) != clip {
			t.Fatal("paged structural preview leaked its hidden header", clip, want)
		}
	}
	v.SetSearch(SearchQuery{Text: "HEADER_END"})
	if _, ok := v.FindNext(false); !ok || v.folds[0].Collapsed || v.Selection().Caret < 100000 || v.Document().Model().Text() != text {
		t.Fatal("search did not reveal a distant structural header page")
	}
}
