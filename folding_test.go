package sourceview

import (
	"context"
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestFoldWorkerDiscardsStaleAndDetachedResults(t *testing.T) {
	v := foldingFixture(t)
	d := v.document
	ctx, cancel := context.WithCancel(context.Background())
	job := &foldingJob{revision: d.model.Revision(), epoch: d.foldGeneration, definition: d.definition, cancel: cancel}
	q := &foldingQueue{owner: d, busy: true, active: job}
	d.folding = q
	if err := d.model.Replace(textedit.Range{}, "x\n"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != context.Canceled || q.active != nil {
		t.Fatal("edit retained obsolete job")
	}
	q.finish([]language.FoldRange{{0, 5}}, nil)
	if len(v.folds) != 2 || v.folds[0].StartLine != 1 {
		t.Fatal("stale result replaced mapped regions", v.FoldingRegions())
	}
	q.owner = nil
	q.active = job
	q.finish([]language.FoldRange{{0, 5}}, nil)
	if len(v.folds) != 2 || v.folds[0].StartLine != 1 {
		t.Fatal("detached queue published result")
	}
}

func TestFoldNotificationReentryPreservesNewDocument(t *testing.T) {
	v := foldingFixture(t)
	other := NewDocument("other document")
	v.ConnectFoldingChanged(func() { v.SetDocument(other) })
	v.Fold(0)
	if v.document != other || len(v.hiddenLines) != 0 || len(v.folds) != 0 {
		t.Fatal("fold callback overwrote replacement document")
	}
}

func foldingFixture(t *testing.T) *SourceView {
	t.Helper()
	v := NewSourceView()
	v.Document().Model().SetText("{\n \"a\": [\n 1,\n 2\n ]\n}\ntail")
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	ranges, err := v.document.definition.Folding()(context.Background(), v.document.model.Text())
	if err != nil {
		t.Fatal(err)
	}
	ranges, err = normalizeFolds(ranges, v.document.model.LineCount())
	if err != nil {
		t.Fatal(err)
	}
	v.acceptFolds(ranges)
	v.context = &shapedContext{}
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 500, Height: 200}, geometry.Point{})
	return v
}
func TestFoldOwnershipVisibilitySelectionAndHistory(t *testing.T) {
	v := foldingFixture(t)
	m := v.document.model
	revision := m.Revision()
	text := m.Text()
	b := NewSourceView()
	b.SetDocument(v.document)
	observe(t, b)
	b.acceptFolds([]language.FoldRange{{0, 5}, {1, 4}})
	if !v.Fold(1) || !v.Fold(0) || v.heights.Count() != 2 || b.FoldingRegions()[0].Collapsed {
		t.Fatal("fold state/visibility isolation")
	}
	if !v.Unfold(0) || !v.FoldingRegions()[1].Collapsed || v.heights.Count() != 4 {
		t.Fatal("nested intent lost")
	}
	r, _ := m.LineRange(3)
	v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
	if v.FoldingRegions()[1].Collapsed {
		t.Fatal("program selection stayed hidden")
	}
	if !v.Fold(1) || v.Selection().Caret >= r.Start {
		t.Fatal("hidden caret not moved to header")
	}
	if m.Text() != text || m.Revision() != revision || m.CanUndo() {
		t.Fatal("fold changed text/history")
	}
	v.SetReadOnly(true)
	v.UnfoldAll()
	if !v.Fold(0) {
		t.Fatal("read-only cannot fold")
	}
	v.SetFoldingEnabled(false)
	if v.heights.Count() != m.LineCount() {
		t.Fatal("disabled folding still hides text")
	}
}
func TestFoldChangeMappingAndBoundaryDeletion(t *testing.T) {
	v := foldingFixture(t)
	m := v.document.model
	v.Fold(1)
	if err := m.Replace(textedit.Range{}, "before\n"); err != nil {
		t.Fatal(err)
	}
	if len(v.folds) != 2 || !v.folds[1].Collapsed || v.folds[1].StartLine != 2 {
		t.Fatal("unrelated edit lost fold", v.FoldingRegions())
	}
	r, _ := m.LineRange(2)
	v.SetSelection(textedit.Selection{Anchor: r.End, Caret: r.End})
	text := m.Text()
	if !v.unfoldDeleteBoundary(true) || m.Text() != text {
		t.Fatal("delete boundary did not only unfold")
	}
	v.Fold(2)
	r, _ = m.LineRange(3)
	if err := m.Replace(textedit.Range{Start: r.Start, End: r.Start}, "x"); err != nil {
		t.Fatal(err)
	}
	if len(v.hiddenLines) != 0 {
		t.Fatal("edit inside hidden content remained hidden")
	}
}
func TestFoldSnapshotHasTrueDiscontinuousText(t *testing.T) {
	v := foldingFixture(t)
	v.Fold(0)
	v.LayoutVisible(geometry.Size{Width: 500, Height: 200}, geometry.Point{})
	info := v.Snapshot()
	if info.TextEditing.VisibleText != "{" || info.TextEditing.VisibleRange != (textedit.Range{Start: 0, End: 1}) {
		t.Fatal("snapshot invented continuous text", info.TextEditing)
	}
	meta := info.Attributes["goui.sourceview"].(map[string]any)
	spans := meta["visibleSpans"].([]map[string]any)
	if len(spans) != 2 || spans[1]["text"] != "tail" || len(info.Children) != 1 {
		t.Fatal("missing visible fragments/fold control", spans)
	}
}
func TestFoldValidation(t *testing.T) {
	for _, bad := range [][]language.FoldRange{{{-1, 2}}, {{0, 4}}, {{2, 2}}, {{0, 2}, {1, 3}}} {
		if _, err := normalizeFolds(bad, 4); err == nil {
			t.Fatal("accepted invalid ranges", bad)
		}
	}
	got, err := normalizeFolds([]language.FoldRange{{1, 2}, {0, 3}, {0, 2}}, 4)
	if err != nil || len(got) != 2 || got[0] != (language.FoldRange{StartLine: 0, EndLine: 3}) {
		t.Fatal(got, err)
	}
}
