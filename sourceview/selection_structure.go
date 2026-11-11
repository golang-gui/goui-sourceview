package sourceview

import (
	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/gui"
	"slices"
)

// MoveWordPart moves across camel-case, acronym, digit and underscore boundaries.
// Without extension, a nonempty selection collapses towards the requested side.
func (v *SourceView) MoveWordPart(forward, extend bool) {
	if len(v.additional) > 0 {
		v.moveMultipleWord(forward, extend, true)
		return
	}
	if v.Destroyed() || !gui.IsEnabled(v) {
		return
	}
	s := v.Selection()
	at := s.Caret
	if !extend && s.Anchor != s.Caret {
		at = s.Range().Start
		if forward {
			at = s.Range().End
		}
	} else {
		at = editor.WordPartBoundary(v.document.model, at, forward, false, v.definitionAt(at).IsWord)
	}
	v.moveTo(v.foldNavigationTarget(at, forward), false, extend)
}

// DeleteWordPart deletes one component or the selection as one undo item.
func (v *SourceView) DeleteWordPart(forward bool) error {
	if len(v.additional) > 0 {
		return v.multipleDeleteKind(forward, 2)
	}
	if err := v.editable(); err != nil {
		return err
	}
	if v.unfoldDeleteBoundary(forward) {
		return nil
	}
	s := v.Selection()
	r := s.Range()
	if r.Start == r.End {
		at := editor.WordPartBoundary(v.document.model, s.Caret, forward, true, v.definitionAt(s.Caret).IsWord)
		if forward {
			r.End = v.snapOffset(at, true)
		} else {
			r.Start = v.snapOffset(at, false)
		}
	}
	if r.Start == r.End {
		return nil
	}
	if s.Anchor == s.Caret && v.expandFoldRange(r) {
		v.foldingSignal.Emit()
		return nil
	}
	return v.applyPlan(editor.Plan{Before: s, After: TextSelection{Anchor: r.Start, Caret: r.Start}, Edits: []TextEdit{{Range: r}}})
}

type selectionTrail struct {
	state operationState
	steps [][]cursorState
}

// ExpandSelection selects the smallest confirmed enclosing word, literal,
// bracket/tag region, logical line or document. Analysis never changes a later selection.
func (v *SourceView) ExpandSelection() bool {
	if v.Destroyed() || !gui.IsEnabled(v) || v.projection != nil {
		return false
	}
	state := v.operationState()
	if !v.selectionTrail.state.valid(v) {
		v.selectionTrail.steps = nil
	}
	before := v.cursors()
	after := slices.Clone(before)
	changed := false
	for i, c := range before {
		s := c.selection
		r := s.Range()
		m := v.document.model
		best := TextRange{End: m.Len()}
		found := best != r
		add := func(q TextRange) {
			if q.Start <= r.Start && q.End >= r.End && q != r && (!found || q.End-q.Start < best.End-best.Start) {
				best = q
				found = true
			}
		}
		add(editor.WordRange(m, r.Start, v.definitionAt(s.Caret).IsWord))
		a, _ := m.LineRange(m.LineAt(r.Start))
		b, _ := m.LineRange(m.LineAt(r.End))
		add(TextRange{Start: a.Start, End: b.End})
		add(TextRange{Start: a.Start, End: min(m.Len(), b.End+1)})
		v.structureCandidates(r, add)
		if !state.valid(v) || v.Destroyed() {
			return false
		}
		if found {
			next := TextSelection{Anchor: best.Start, Caret: best.End}
			if s.Anchor > s.Caret {
				next.Anchor, next.Caret = next.Caret, next.Anchor
			}
			after[i] = cursorState{selection: next}
			changed = true
		}
	}
	if !changed {
		return false
	}
	steps := slices.Clone(v.selectionTrail.steps)
	if len(steps) == 64 {
		steps = steps[1:]
	}
	steps = append(steps, before)
	v.document.model.BreakUndoGroup()
	v.setCursorStatesKeepingCompletion(normalizeCursors(after), true, nil)
	if v.Destroyed() || v.document != state.document || v.mountEpoch != state.mount || v.document.Revision() != state.revision || v.selectionEpoch != state.selection+1 {
		return false
	}
	v.selectionTrail = selectionTrail{v.operationState(), steps}
	return true
}

// ShrinkSelection restores the preceding complete set, including direction,
// identity and affinity even if expansion merged intersecting ranges.
func (v *SourceView) ShrinkSelection() bool {
	trail := v.selectionTrail
	if v.Destroyed() || !gui.IsEnabled(v) || v.projection != nil || !trail.state.valid(v) || len(trail.steps) == 0 {
		v.selectionTrail.steps = nil
		return false
	}
	v.document.model.BreakUndoGroup()
	v.setCursorStatesKeepingCompletion(slices.Clone(trail.steps[len(trail.steps)-1]), true, nil)
	if v.Destroyed() || v.document != trail.state.document || v.mountEpoch != trail.state.mount || v.document.Revision() != trail.state.revision || v.selectionEpoch != trail.state.selection+1 {
		return false
	}
	v.selectionTrail = selectionTrail{v.operationState(), trail.steps[:len(trail.steps)-1]}
	return true
}

func (v *SourceView) structureCandidates(r TextRange, add func(TextRange)) {
	editor.StructureCandidates(v.document.model, r, v.definitionAt(r.Start).Editing(), func() editor.TokenReader {
		if e := v.document.engine; e != nil {
			return e.Tokens
		}
		return nil
	}, v.lexicalContext, add)
}
