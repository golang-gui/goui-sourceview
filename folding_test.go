package sourceview

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
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
	q.finish([]language.FoldRange{{StartLine: 0, EndLine: 5}}, nil)
	if len(v.folds) != 2 || v.folds[0].StartLine != 1 {
		t.Fatal("stale result replaced mapped regions", v.FoldingRegions())
	}
	q.owner = nil
	q.active = job
	q.finish([]language.FoldRange{{StartLine: 0, EndLine: 5}}, nil)
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
	b.acceptFolds([]language.FoldRange{{StartLine: 0, EndLine: 5}, {StartLine: 1, EndLine: 4}})
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
	for _, bad := range [][]language.FoldRange{{{StartLine: -1, EndLine: 2}}, {{StartLine: 0, EndLine: 4}}, {{StartLine: 2, EndLine: 2}}, {{StartLine: 0, EndLine: 2}, {StartLine: 1, EndLine: 3}}} {
		if _, err := normalizeFolds(bad, 4); err == nil {
			t.Fatal("accepted invalid ranges", bad)
		}
	}
	got, err := normalizeFolds([]language.FoldRange{{StartLine: 1, EndLine: 2}, {StartLine: 0, EndLine: 3}, {StartLine: 0, EndLine: 2}}, 4)
	if err != nil || len(got) != 2 || got[0] != (language.FoldRange{StartLine: 0, EndLine: 3}) {
		t.Fatal(got, err)
	}
}

func TestFoldAnalysisPreservesUnchangedVisibleGeometry(t *testing.T) {
	v, scroll := revealFixture(t)
	at := v.document.model.Len() - 1
	v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
	assertRevealed(t, v, scroll)
	size, offset, epoch := v.ContentSize(), v.offset, v.layoutEpoch
	v.acceptFolds([]language.FoldRange{{StartLine: 0, EndLine: 199}})
	if v.ContentSize() != size || v.offset != offset || v.layoutEpoch != epoch || v.pendingAnchor {
		t.Fatal("uncollapsed fold analysis reset measured rows or queued an anchor restore", size, v.ContentSize(), epoch, v.layoutEpoch, v.pendingAnchor)
	}
	scroll.Arrange(scroll.Rect())
	assertRevealed(t, v, scroll)
}

func TestFoldBadgeExtentDoesNotDependOnWhitespaceFlag(t *testing.T) {
	v := foldingFixture(t)
	if !v.Fold(0) {
		t.Fatal("fold")
	}
	v.LayoutVisible(geometry.Size{Width: 30, Height: 200}, geometry.Point{})
	before := v.ContentSize()
	v.SetShowLineEndings(true)
	p := v.paragraph(0)
	r := v.foldBadgeRect(0, p)
	if v.ContentSize() != before || r.X+r.Width > v.ContentSize().Width {
		t.Fatal("badge clipped or marker changed content width", r, v.ContentSize(), before)
	}
}

func TestFoldMappingUsesSequentialLineChanges(t *testing.T) {
	const original = "prefix\nx\n{\n one\n two\n}\ntail\nz"
	for _, tc := range []struct {
		name  string
		edits []textedit.Edit
		lines [2]int // Negative means the fold intersects an edit and is removed.
	}{
		{"prefix insertion", []textedit.Edit{{Text: "a\nb\n"}}, [2]int{4, 7}},
		{"same line replacement", []textedit.Edit{{Range: textedit.Range{End: 6}, Text: "p"}}, [2]int{2, 5}},
		{"prefix deletion", []textedit.Edit{{Range: textedit.Range{End: 7}}}, [2]int{1, 4}},
		{"header boundary newline", []textedit.Edit{{Range: textedit.Range{Start: 9, End: 9}, Text: "new\n"}}, [2]int{3, 6}},
		{"header boundary character", []textedit.Edit{{Range: textedit.Range{Start: 9, End: 9}, Text: "x"}}, [2]int{-1, -1}},
		{"inside fold", []textedit.Edit{{Range: textedit.Range{Start: 12, End: 12}, Text: "\n"}}, [2]int{-1, -1}},
		{"suffix insertion", []textedit.Edit{{Range: textedit.Range{Start: len(original), End: len(original)}, Text: "\na\nb"}}, [2]int{2, 5}},
		{"sequential edits", []textedit.Edit{{Text: "a\nb\n"}, {Range: textedit.Range{Start: 4, End: 11}, Text: ""}}, [2]int{3, 6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, incremental := range []bool{true, false} {
				v := NewSourceView()
				m := v.Document().Model()
				m.SetText(original)
				v.acceptFolds([]language.FoldRange{{StartLine: 2, EndLine: 5}})
				var splices []highlight.Splice
				for _, e := range tc.edits {
					first, last := m.LineAt(e.Range.Start), m.LineAt(e.Range.End)
					splices = append(splices, highlight.Splice{Index: first, Removed: last - first + 1, Added: strings.Count(e.Text, "\n") + 1})
					if err := m.ReplaceAtomic(e.Range, e.Text); err != nil {
						t.Fatal(err)
					}
				}
				// A Change can contain multiple ordered edits (undo/redo). The
				// fixture builds their final text with separate model operations.
				v.foldRevision = m.Revision() - 1
				if !incremental {
					splices = nil // Analysis was still rebuilding: use exact fallback.
				}
				v.mapFolds(textedit.Change{Revision: m.Revision(), Edits: tc.edits}, splices)
				if tc.lines[0] < 0 {
					if len(v.folds) != 0 {
						t.Fatal("intersected fold survived", v.folds)
					}
					continue
				}
				if len(v.folds) != 1 || v.folds[0].StartLine != tc.lines[0] || v.folds[0].EndLine != tc.lines[1] {
					t.Fatalf("incremental=%v: wanted lines %v, got %v", incremental, tc.lines, v.folds)
				}
				r := v.folds[0]
				text, err := m.Slice(textedit.Range{Start: r.start, End: r.end})
				if err != nil || text != "{\n one\n two\n}" || m.LineAt(r.start) != r.StartLine || m.LineAt(r.end) != r.EndLine {
					t.Fatal("mapped line and byte endpoints disagree", r, text, err)
				}
			}
		})
	}
}

func TestFoldMappingRetainsCollapsedStateThroughHistory(t *testing.T) {
	v := foldingFixture(t)
	m := v.document.model
	if err := m.ReplaceAtomic(textedit.Range{}, "guard\n"); err != nil {
		t.Fatal(err)
	}
	v.document.engine.Advance(time.Time{})
	v.Fold(2)
	original := slices.Clone(v.folds)
	if err := m.ReplaceAtomic(textedit.Range{}, "first\nsecond\n"); err != nil {
		t.Fatal(err)
	}
	if len(v.folds) != 2 || v.folds[0].StartLine != 3 || v.folds[1].StartLine != 4 || !v.folds[1].Collapsed {
		t.Fatal("observer lost mapped fold state", v.folds)
	}
	if !m.Undo() || !slices.Equal(v.folds, original) {
		t.Fatal("undo did not restore fold endpoints/state", v.folds, original)
	}
	if !m.Redo() || len(v.folds) != 2 || v.folds[1].StartLine != 4 || !v.folds[1].Collapsed {
		t.Fatal("redo did not remap collapsed fold", v.folds)
	}
}

func BenchmarkFoldMapping(b *testing.B) {
	const count = 100000
	for _, incremental := range []bool{false, true} {
		name := "position queries"
		if incremental {
			name = "line deltas"
		}
		b.Run(name, func(b *testing.B) {
			v := NewSourceView()
			m := v.document.model
			m.SetText("prefix\n" + strings.Repeat("{\nvalue\n}\n", count))
			folds := make([]foldEntry, count)
			for i := range folds {
				folds[i] = foldEntry{FoldRegion: FoldRegion{StartLine: 1 + i*3, EndLine: 3 + i*3}, start: 7 + i*10, end: 16 + i*10}
			}
			if err := m.ReplaceAtomic(textedit.Range{}, "new\n"); err != nil {
				b.Fatal(err)
			}
			c := textedit.Change{Revision: m.Revision(), Edits: []textedit.Edit{{Text: "new\n"}}}
			var splices []highlight.Splice
			if incremental {
				splices = []highlight.Splice{{Removed: 1, Added: 2}}
			}
			v.folds = make([]foldEntry, count)
			b.ResetTimer()
			for range b.N {
				copy(v.folds, folds)
				v.foldRevision = c.Revision - 1
				v.mapFolds(c, splices)
			}
		})
	}
}
