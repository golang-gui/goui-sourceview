package sourceview

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

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
