package text

import "sort"

// SelectionState is view-independent history data. Upstream disambiguates a
// caret shared by two wrapped display lines; no layout or Widget is retained.
type SelectionState struct {
	Selection
	Upstream bool
}

// NormalizeSelections merges ambiguous editing ranges. Input zero remains the
// primary selection; other results are in document order. A touching empty
// cursor merges, while two nonempty touching selections remain independent.
// The earliest input in a merged group owns its direction and affinity.
func NormalizeSelections(input []SelectionState) []SelectionState {
	if len(input) == 0 {
		return nil
	}
	type entry struct {
		state SelectionState
		owner int
	}
	work := make([]entry, len(input))
	for i, s := range input {
		work[i] = entry{s, i}
	}
	sort.SliceStable(work, func(i, j int) bool {
		a, b := work[i].state.Range(), work[j].state.Range()
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.End < b.End
	})
	merged := work[:0]
	for _, next := range work {
		if len(merged) == 0 {
			merged = append(merged, next)
			continue
		}
		last := &merged[len(merged)-1]
		a, b := last.state.Range(), next.state.Range()
		if b.Start < a.End || b.Start == a.End && (a.Start == a.End || b.Start == b.End) {
			winner := *last
			if next.owner < winner.owner {
				winner = next
			}
			r := Range{min(a.Start, b.Start), max(a.End, b.End)}
			if winner.state.Anchor <= winner.state.Caret {
				winner.state.Selection = Selection{r.Start, r.End}
			} else {
				winner.state.Selection = Selection{r.End, r.Start}
			}
			*last = winner
		} else {
			merged = append(merged, next)
		}
	}
	primary := 0
	for i, e := range merged {
		if e.owner == 0 {
			primary = i
			break
		}
	}
	out := make([]SelectionState, 0, len(merged))
	out = append(out, merged[primary].state)
	for i, e := range merged {
		if i != primary {
			out = append(out, e.state)
		}
	}
	return out
}

// HistorySelections previews only the next history item's value metadata, so
// its initiating view can publish the final set during the change notification.
func (m *Model) HistorySelections(undo bool) []SelectionState {
	steps := m.redoSteps
	if undo {
		steps = m.undoSteps
	}
	if len(steps) == 0 {
		return nil
	}
	step := steps[len(steps)-1]
	states := step.afterSelections
	if undo {
		states = step.beforeSelections
	}
	if len(states) > 0 {
		return append([]SelectionState(nil), states...)
	}
	if step.selection != nil {
		s := step.selection.after
		if undo {
			s = step.selection.before
		}
		return []SelectionState{{Selection: s}}
	}
	return nil
}
