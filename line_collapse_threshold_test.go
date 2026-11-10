package sourceview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

// The digit run has no whitespace/ligatures. Sum native line advances to choose
// widths safely inside each wrap-count interval; verify the native count before
// testing eligibility rather than predicting row count from paragraph height.
func thresholdFixture(t *testing.T, count int) (*SourceView, *countedNativeContext, float32) {
	t.Helper()
	v, ctx := collapsedLineFixture(t, strings.Repeat("1", 120))
	p := v.cachedParagraph(0)
	advance := float32(0)
	for i := 0; i < p.geometry.LineCount(); i++ {
		advance += p.geometry.LineMetrics(i).Width
	}
	v.SetFoldingEnabled(true)
	v.updateGutter()
	v.Arrange(geometry.Rect(0, 0, advance/(float32(count)-.5)+v.gutter+2*padding, 240))
	if got := v.cachedParagraph(0).geometry.LineCount(); got != count {
		t.Fatalf("fixture native display rows=%d, want %d", got, count)
	}
	return v, ctx, advance
}

func TestLongLineCollapseUsesThreeMeasuredDisplayLines(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 8} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			v, ctx, _ := thresholdFixture(t, count)
			created := ctx.created
			want := count >= 3
			if v.canCollapseLine(0) != want || v.canCommand(CommandCollapseLine) != want {
				t.Fatal("API/menu eligibility does not match actual display rows", count)
			}
			if _, ok := v.foldHit(v.foldButtonRect(0).Center()); ok != want {
				t.Fatal("gutter eligibility does not match actual display rows", count)
			}
			arrows := 0
			for _, c := range v.Snapshot().Children {
				if _, ok := c.Attributes["goui.sourceview.fold"]; ok {
					arrows++
				}
			}
			if (arrows == 1) != want || arrows > 1 || ctx.created != created {
				t.Fatal("snapshot changed eligibility or created native layouts", arrows)
			}
			if v.CollapseLine(0) != want {
				t.Fatal("collapse result does not match actual display rows", count)
			}
		})
	}
}

func TestLongLineCollapseIgnoresHeightEstimate(t *testing.T) {
	for _, count := range []int{2, 3} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			v, ctx, _ := thresholdFixture(t, count)
			p := v.cachedParagraph(0)
			if count == 2 {
				p.height = 100 * v.lineHeight
			} else {
				p.height = v.lineHeight
			}
			created := ctx.created
			if v.canCollapseLine(0) != (count >= 3) || ctx.created != created {
				t.Fatal("height estimate replaced measured display row count", count)
			}
		})
	}
}

func TestLongLineCollapseThresholdResizeKeepsExpandEntry(t *testing.T) {
	v, _, advance := thresholdFixture(t, 3)
	original, revision, undo := v.Document().Model().Text(), v.Document().Model().Revision(), v.Document().Model().CanUndo()
	if !v.toggleGutterFold(0) {
		t.Fatal("three-row line has no collapse action")
	}
	v.Arrange(geometry.Rect(0, 0, advance+v.gutter+2*padding+20, 240))
	if v.cachedParagraph(0).geometry.LineCount() != 1 {
		t.Fatal("wide fixture did not fit one native row")
	}
	if collapsed, ok := v.gutterFold(0); !ok || !collapsed {
		t.Fatal("resize removed the existing expansion arrow")
	}
	if !v.canCommand(CommandExpandLine) || v.lineCollapseBadge(0, v.cachedParagraph(0)).Width <= 0 {
		t.Fatal("resize removed menu/badge expansion")
	}
	if !v.toggleGutterFold(0) || len(v.CollapsedLines()) != 0 {
		t.Fatal("wide line cannot expand")
	}
	if _, ok := v.gutterFold(0); ok {
		t.Fatal("one-row expanded line still has a collapse entry")
	}
	if v.Document().Model().Text() != original || v.Document().Model().Revision() != revision || v.Document().Model().CanUndo() != undo {
		t.Fatal("threshold/resize changed source or history")
	}
}

func TestLongLineCollapseThresholdPagedGeometry(t *testing.T) {
	v, ctx := collapsedLineFixture(t, strings.Repeat("paged words ", 10000))
	p := v.cachedParagraph(0)
	if p.paged == nil || p.geometry.LineCount() < 3 {
		t.Fatal("fixture has no measured paged rows")
	}
	p.height = v.lineHeight // Unmeasured global extent must not control eligibility.
	created := ctx.created
	if !v.canCollapseLine(0) || ctx.created != created {
		t.Fatal("paged eligibility ignored measured local rows or allocated a layout")
	}
	// The painting window may have moved to a small final page while page zero
	// is still cached. Existing first-page geometry remains valid evidence.
	p.geometry = textedit.NewGeometry(nil, nil, len(v.Document().Model().Text()), v.lineHeight)
	if !v.canCollapseLine(0) || ctx.created != created {
		t.Fatal("eligibility lost a cached first page or shaped text")
	}
}

func TestLongLineCollapseThresholdJSONArrowsStayOnTheirRows(t *testing.T) {
	v, _, advance := thresholdFixture(t, 3)
	text := "{ \"test\":\n\"" + strings.Repeat("1", 120) + "\" }"
	v.Document().Model().SetText(text)
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.acceptFolds([]language.FoldRange{{StartLine: 0, EndLine: 1}})
	v.Arrange(geometry.Rect(0, 0, advance/2.5+v.gutter+2*padding, 240))
	if v.cachedParagraph(1).geometry.LineCount() != 3 {
		t.Fatal("JSON string does not span three native rows")
	}
	entries := map[int]string{}
	for _, c := range v.Snapshot().Children {
		if meta, ok := c.Attributes["goui.sourceview.fold"].(map[string]any); ok {
			entries[meta["startLine"].(int)] = meta["kind"].(string)
		}
	}
	if len(entries) != 2 || entries[0] != "structure" || entries[1] != "line" {
		t.Fatal("JSON structure and string entries do not have distinct logical rows", entries)
	}
	if !v.toggleGutterFold(1) || v.folds[0].Collapsed || !v.manuallyCollapsed(1) {
		t.Fatal("string arrow collapsed the JSON root")
	}
	if v.foldButtonRect(1).Y <= v.foldButtonRect(0).Y {
		t.Fatal("string arrow moved onto the root row")
	}
	if !v.toggleGutterFold(1) || !v.toggleGutterFold(0) || !v.folds[0].Collapsed || len(v.CollapsedLines()) != 0 {
		t.Fatal("root arrow failed to control the structure separately")
	}
}
