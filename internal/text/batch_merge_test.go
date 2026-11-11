package text

import (
	"slices"
	"testing"
)

func TestBatchContinuousInputHistoryAndBreaks(t *testing.T) {
	for _, kind := range []BatchMerge{MergeTyping, MergeBackwardDelete, MergeForwardDelete} {
		t.Run(map[BatchMerge]string{MergeTyping: "insert", MergeBackwardDelete: "backspace", MergeForwardDelete: "delete"}[kind], func(t *testing.T) {
			m := NewModel("abcdefghij\nabcdefghij")
			original := m.Text()
			source := &EditSource{}
			before := []SelectionState{{Selection: Selection{5, 5}}, {Selection: Selection{16, 16}, Upstream: true}}
			current := slices.Clone(before)
			for step := 0; step < 3; step++ {
				edits := make([]Edit, len(current))
				after := slices.Clone(current)
				delta := 0
				for i, s := range current {
					at := s.Caret
					e := Edit{Range: Range{at, at}, Text: "中"}
					if kind == MergeBackwardDelete {
						e = Edit{Range: Range{at - 1, at}}
					} else if kind == MergeForwardDelete {
						e = Edit{Range: Range{at, at + 1}}
					}
					edits[i] = e
					end := e.Range.Start + delta + len(e.Text)
					after[i].Selection = Selection{end, end}
					delta += len(e.Text) - (e.Range.End - e.Range.Start)
				}
				if err := m.ApplyBatch(edits, BatchOptions{Source: source, BeforeSelections: current, AfterSelections: after, Merge: kind}); err != nil {
					t.Fatal(err)
				}
				current = after
			}
			result := m.Text()
			if len(m.undoSteps) != 1 || len(m.undoSteps[0].beforeSelections) != 2 || !slices.Equal(m.undoSteps[0].afterSelections, current) {
				t.Fatal("group or snapshots", len(m.undoSteps))
			}
			undo := m.UndoWithSelection()
			if m.Text() != original || !slices.Equal(undo.Selections, before) || m.CanUndo() {
				t.Fatal("undo", m.Text(), undo)
			}
			redo := m.RedoWithSelection()
			if m.Text() != result || !slices.Equal(redo.Selections, current) {
				t.Fatal("redo", m.Text(), redo)
			}
			m.BreakUndoGroup()
			// A changed source and a redo branch must never attach to a closed group.
			m.Undo()
			if err := m.ApplyBatch([]Edit{{Range: Range{0, 0}, Text: "x"}}, BatchOptions{}); err != nil {
				t.Fatal(err)
			}
			if m.CanRedo() {
				t.Fatal("redo branch retained")
			}
		})
	}
}
func TestBatchMergeRequiresIdentitySourceAndInputShape(t *testing.T) {
	for _, reason := range []string{"source", "selection", "newline", "atomic", "direction"} {
		t.Run(reason, func(t *testing.T) {
			m := NewModel("ab cd")
			a, b := &EditSource{}, &EditSource{}
			before := []SelectionState{{Selection: Selection{1, 1}}, {Selection: Selection{4, 4}}}
			after := []SelectionState{{Selection: Selection{2, 2}}, {Selection: Selection{6, 6}}}
			m.ApplyBatch([]Edit{{Range: Range{1, 1}, Text: "x"}, {Range: Range{4, 4}, Text: "x"}}, BatchOptions{Source: a, BeforeSelections: before, AfterSelections: after, Merge: MergeTyping})
			source, kind, text := a, MergeTyping, "y"
			start := slices.Clone(after)
			switch reason {
			case "source":
				source = b
			case "selection":
				start[0].Upstream = true
			case "newline":
				text = "\n"
			case "atomic":
				kind = MergeNone
			case "direction":
				kind = MergeForwardDelete
			}
			end := []SelectionState{{Selection: Selection{3, 3}}, {Selection: Selection{8, 8}}}
			if err := m.ApplyBatch([]Edit{{Range: Range{2, 2}, Text: text}, {Range: Range{6, 6}, Text: text}}, BatchOptions{Source: source, BeforeSelections: start, AfterSelections: end, Merge: kind}); err != nil {
				t.Fatal(err)
			}
			if len(m.undoSteps) != 2 {
				t.Fatal("merged despite", reason)
			}
		})
	}
}
