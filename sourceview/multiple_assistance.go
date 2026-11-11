package sourceview

import (
	"slices"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/gui"
)

func (v *SourceView) typingPlan(c cursorState, commit gui.IMCommit) (plannedPair, error) {
	if !commit.Composed && !v.compositionSeen && v.projection == nil {
		if p, ok, err := v.linkedTagPlan(c, c.selection.Range(), commit.Text); ok || err != nil {
			return plannedPair{plan: p}, err
		}
		if p, ok, err := v.closeTagPlan(c, c.selection.Range(), commit.Text); ok || err != nil {
			return plannedPair{plan: p}, err
		}
		if p, ok, err := v.pairPlan(c, commit.Text); ok || err != nil {
			return p, err
		}
	}
	return plannedPair{plan: replacementPlan(c, commit.Text)}, nil
}

func (v *SourceView) applyPairPlans(results []plannedPair, merge textcore.BatchMerge) error {
	plans := make([]editor.Plan, len(results))
	for i, p := range results {
		plans[i] = p.plan
		if p.created != nil || p.consumed != nil {
			merge = textcore.MergeNone
		}
	}
	edits, after, err := combinePlans(plans)
	if err != nil {
		return err
	}
	// Retire only the generated closers actually traversed, after a successful
	// transaction. Other edits may shift their ordinary owned value coordinates.
	consumed := make(map[autoPair]bool)
	var positions []int
	var old []autoPair
	for _, r := range results {
		if r.consumed != nil {
			old = append(old, *r.consumed)
			positions = append(positions, r.consumed.open, r.consumed.close)
		}
	}
	changes := slices.Clone(edits)
	slices.Reverse(changes)
	positions = textcore.MapOffsets(positions, changes)
	for i, p := range old {
		p.open, p.close = positions[2*i], positions[2*i+1]
		consumed[p] = true
	}
	state := v.operationState()
	if len(edits) == 0 {
		v.document.model.BreakUndoGroup()
	}
	if err = v.applyMultipleInput(edits, after, merge); err != nil {
		return err
	}
	revision := state.revision
	if len(edits) > 0 {
		revision++
	}
	if v.Destroyed() || v.document != state.document || v.mountEpoch != state.mount || v.document.languageEpoch != state.language || v.editOptionsEpoch != state.settings || v.document.Revision() != revision || v.selectionEpoch != state.selection+1 {
		return nil
	}
	v.pairs = slices.DeleteFunc(v.pairs, func(p autoPair) bool { return consumed[p] })
	for i, r := range results {
		if r.created != nil {
			sel := after[i].selection
			v.pairs = append(v.pairs, autoPair{open: min(sel.Anchor, sel.Caret) - len(r.created.Open), close: max(sel.Anchor, sel.Caret), left: r.created.Open, right: r.created.Close})
		}
	}
	const pairLimit = 4 * maxSelections
	if len(v.pairs) > pairLimit {
		v.pairs = slices.Clone(v.pairs[len(v.pairs)-pairLimit:])
	}
	return nil
}

func (v *SourceView) deleteCursorPlan(c cursorState, forward bool, word int) (editor.Plan, error) {
	s := c.selection
	r := s.Range()
	if r.Start == r.End {
		if !forward {
			if word == 0 {
				if p, ok := v.deleteIndentPlan(s); ok {
					return p, nil
				}
			}
			if word != 2 {
				for _, p := range v.pairs {
					if p.open+len(p.left) == s.Caret && p.close == s.Caret && v.pairValid(p) {
						return editor.Plan{Before: s, After: TextSelection{Anchor: p.open, Caret: p.open}, Edits: []TextEdit{{Range: TextRange{Start: p.open, End: p.close + len(p.right)}}}}, nil
					}
				}
			}
		}
		at := s.Caret
		switch word {
		case 1:
			at = editor.WordBoundaryWith(v.document.model, at, forward, true, v.definitionAt(at).IsWord)
		case 2:
			at = editor.WordPartBoundary(v.document.model, at, forward, true, v.definitionAt(at).IsWord)
		default:
			at = v.adjacent(at, forward)
		}
		if word != 0 {
			at = v.snapOffset(at, forward)
		}
		if forward {
			r.End = at
		} else {
			r.Start = at
		}
	}
	if p, ok, err := v.linkedTagPlan(c, r, ""); ok || err != nil {
		return p, err
	}
	return editor.Plan{Before: s, After: TextSelection{Anchor: r.Start, Caret: r.Start}, Edits: []TextEdit{{Range: r}}}, nil
}

func (v *SourceView) unfoldMultipleDeletion(cursors []cursorState, plans []editor.Plan) bool {
	state := v.operationState()
	changed := false
	for i, c := range cursors {
		if c.selection.Anchor == c.selection.Caret {
			for _, e := range plans[i].Edits {
				if v.expandFoldRange(e.Range) {
					changed = true
				}
			}
		}
	}
	if changed && state.valid(v) && !v.Destroyed() {
		v.document.model.BreakUndoGroup()
		v.foldingSignal.Emit()
	}
	return changed
}
func (v *SourceView) multipleDeleteKind(forward bool, word int) error {
	if err := v.editable(); err != nil {
		return err
	}
	state := v.operationState()
	cursors := v.cursors()
	plans := make([]editor.Plan, len(cursors))
	for i, c := range cursors {
		p, err := v.deleteCursorPlan(c, forward, word)
		if err != nil {
			return err
		}
		if !state.editingValid(v) {
			return ErrOperationChanged
		}
		plans[i] = p
	}
	if v.unfoldMultipleDeletion(cursors, plans) {
		return nil
	}
	edits, after, err := combinePlans(plans)
	if err != nil {
		return err
	}
	return v.applyMultiple(edits, after)
}

func (v *SourceView) tabCursorPlan(c cursorState, outdent bool) (editor.Plan, error) {
	if outdent || c.selection.Anchor != c.selection.Caret {
		return editor.Indent(v.document.model, c.selection, editor.IndentOptions(v.indent), outdent)
	}
	text := "\t"
	if v.indent.InsertSpaces {
		line, _ := v.document.LineRange(v.document.model.LineAt(c.selection.Caret))
		prefix, _ := v.document.Slice(TextRange{Start: line.Start, End: c.selection.Caret})
		column := 0
		for _, r := range prefix {
			if r == '\t' {
				column += v.indent.TabWidth - column%v.indent.TabWidth
			} else {
				column++
			}
		}
		text = strings.Repeat(" ", v.indent.TabWidth-column%v.indent.TabWidth)
	}
	return replacementPlan(c, text), nil
}

// Input plans can span turns and call language policies. Option changes cancel
// that frozen operation instead of committing a mixture of old and new rules.
func (v *SourceView) editOptionsChanged() {
	v.editOptionsEpoch++
	v.columnSelection = nil
	v.pendingInput = nil
	v.cancelEdit()
	v.cancelPaste()
	v.document.model.BreakUndoGroup()
}
