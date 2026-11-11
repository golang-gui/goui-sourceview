package sourceview

import (
	"fmt"
	"slices"
	"sort"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
)

const maxSelections = 4096

type cursorState struct {
	selection    TextSelection
	upstream     bool
	preferredX   float32
	preferredSet bool
}

// selectionCommit publishes one initiating view's complete result while model
// change callbacks are running. External/reentrant changes cannot reuse it.
type selectionCommit struct {
	state   operationState
	after   []cursorState
	applied bool
}

// Selections returns an owned copy. The first item is the primary selection;
// additional selections are in document order. There is always at least one.
func (v *SourceView) Selections() []TextSelection {
	out := make([]TextSelection, 1, len(v.additional)+1)
	out[0] = v.Selection()
	for _, c := range v.additional {
		out = append(out, v.clampSelection(c.selection))
	}
	return out
}

// SetSelections validates and copies a nonempty UTF-8 selection set (at most
// 4096). Invalid input leaves the view unchanged. SetSelection instead clamps
// its one selection and explicitly removes additional selections.
func (v *SourceView) SetSelections(selections []TextSelection) error {
	if v.Destroyed() {
		return ErrDisabled
	}
	if len(selections) == 0 {
		return fmt.Errorf("sourceview: empty selection set")
	}
	if len(selections) > maxSelections {
		return ErrOperationLimit
	}
	states := make([]cursorState, len(selections))
	for i, s := range selections {
		r := s.Range()
		m := v.document.model
		if r.Start < 0 || r.End > m.Len() || !m.IsRuneBoundary(r.Start) || !m.IsRuneBoundary(r.End) {
			return fmt.Errorf("sourceview: invalid selection %d", i)
		}
		states[i].selection = s
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	v.pendingInput = nil
	v.document.model.BreakUndoGroup()
	v.setCursorStatesKeepingCompletion(normalizeCursors(states), true, nil)
	return nil
}

// ConnectSelections observes complete sets, with a separate owned copy for
// each invocation. Disconnect the handle when its owner is released.
func (v *SourceView) ConnectSelections(fn func([]TextSelection)) signal.Handle {
	return v.selectionsSignal.Connect(func(s []TextSelection) { fn(slices.Clone(s)) })
}

func (v *SourceView) cursors() []cursorState {
	out := make([]cursorState, 1, len(v.additional)+1)
	out[0] = cursorState{v.selection, v.upstream, v.preferredX, v.preferredSet}
	return append(out, v.additional...)
}
func (v *SourceView) historySelections() []textcore.SelectionState {
	return cursorHistory(v.cursors())
}
func cursorHistory(cursors []cursorState) []textcore.SelectionState {
	out := make([]textcore.SelectionState, len(cursors))
	for i, c := range cursors {
		out[i] = textcore.SelectionState{Selection: c.selection, Upstream: c.upstream}
	}
	return out
}
func cursorsFromHistory(states []textcore.SelectionState) []cursorState {
	out := make([]cursorState, len(states))
	for i, s := range states {
		out[i] = cursorState{selection: s.Selection, upstream: s.Upstream}
	}
	return out
}
func (v *SourceView) assignCursors(states []cursorState) {
	if len(states) == 0 {
		states = []cursorState{{}}
	}
	c := states[0]
	v.selection, v.upstream, v.preferredX, v.preferredSet = c.selection, c.upstream, c.preferredX, c.preferredSet
	v.additional = slices.Clone(states[1:])
}
func normalizeCursors(states []cursorState) []cursorState {
	normalized := textcore.NormalizeSelections(cursorHistory(states))
	owners := make(map[TextSelection]cursorState, len(states))
	for i := len(states) - 1; i >= 0; i-- {
		owners[states[i].selection] = states[i]
	}
	out := cursorsFromHistory(normalized)
	for i := range out {
		if c, ok := owners[out[i].selection]; ok {
			out[i].preferredX, out[i].preferredSet = c.preferredX, c.preferredSet
		}
	}
	return out
}
func (v *SourceView) emitSelections(before []TextSelection) {
	after := v.Selections()
	if slices.Equal(before, after) {
		return
	}
	state := v.operationState()
	v.selectionsSignal.Emit(after)
	if !state.valid(v) || v.Destroyed() {
		return
	}
	if len(before) == 0 || before[0] != after[0] {
		v.selectionSignal.Emit(after[0])
	}
}
func (v *SourceView) orderedSelections() []TextSelection {
	out := v.Selections()
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Range(), out[j].Range()
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.End < b.End
	})
	return out
}
func (v *SourceView) setCursorStatesKeepingCompletion(cursors []cursorState, reveal bool, keep *completionSession) {
	v.cancelViewRestore()
	v.ClearInlineSuggestion()
	defer v.searchState.Emit()
	state := v.operationState()
	for i := range cursors {
		cursors[i].selection = v.clampSelection(cursors[i].selection)
	}
	s := cursors[0].selection
	upstream := cursors[0].upstream
	foldingChanged := false
	if reveal {
		for _, c := range cursors {
			if v.unfoldSelection(c.selection) {
				foldingChanged = true
			}
			if !state.valid(v) {
				return
			}
		}
		if !state.valid(v) || v.Destroyed() {
			return
		}
	}
	if len(cursors) == 1 && v.context != nil && v.projection == nil {
		if s.Anchor == s.Caret {
			s.Caret = v.snapOffset(s.Caret, true)
			s.Anchor = s.Caret
		} else {
			forward := s.Caret >= s.Anchor
			s.Anchor = v.snapOffset(s.Anchor, !forward)
			s.Caret = v.snapOffset(s.Caret, forward)
		}
	}
	if !state.valid(v) {
		return
	}
	old := v.Selections()
	v.reveal = nil
	if reveal {
		v.pendingAnchor = false
		v.anchor.valid = false
	}
	cursors[0].selection, cursors[0].upstream = s, upstream
	v.assignCursors(cursors)
	v.selectionRevision = v.document.model.Revision()
	v.selectionEpoch++
	v.cancelPaste()
	v.cancelEdit()
	if keep != nil && v.completion == keep && !foldingChanged && s.Anchor == s.Caret && s.Caret >= keep.range_.Start && s.Caret <= keep.range_.End {
		keep.state = v.operationState()
		keep.pending = true // Old candidates cannot be committed from selection callbacks.
	} else {
		v.closeCompletion()
	}
	if v.document != state.document || v.mountEpoch != state.mount || v.document.model.Revision() != state.revision || v.selectionEpoch != state.selection+1 {
		return
	}
	v.currentMatch = nil
	v.resetBlink()
	v.RequestPaint()
	selectionEpoch := v.selectionEpoch
	v.updateMenu()
	if reveal {
		v.RevealSelection()
	}
	if v.selectionEpoch == selectionEpoch {
		v.emitSelections(old)
	}
	if foldingChanged && !v.Destroyed() && v.document == state.document && v.mountEpoch == state.mount {
		v.foldingSignal.Emit()
	}
}

func (v *SourceView) snapshotSelections() []gui.TextSelection {
	out := make([]gui.TextSelection, 0, len(v.additional)+1)
	for _, s := range v.Selections() {
		out = append(out, gui.TextSelection(s))
	}
	return out
}
func (v *SourceView) selectionRanges() []TextRange {
	if v.selectionRangesCache == nil || v.selectionRangesEpoch != v.selectionEpoch || v.selectionRangesRevision != v.document.Revision() {
		out := make([]TextRange, 0, len(v.additional)+1)
		for _, s := range v.Selections() {
			if s.Anchor != s.Caret {
				out = append(out, s.Range())
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
		v.selectionRangesCache = out
		v.selectionRangesEpoch = v.selectionEpoch
		v.selectionRangesRevision = v.document.Revision()
	}
	return v.selectionRangesCache
}
func selectionIntersections(ranges []TextRange, r TextRange) []TextRange {
	start := sort.Search(len(ranges), func(i int) bool { return ranges[i].End > r.Start })
	end := start
	for end < len(ranges) && ranges[end].Start <= r.End {
		end++
	}
	return ranges[start:end]
}
