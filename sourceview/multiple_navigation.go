package sourceview

import (
	"errors"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/events"
)

var errSelectionLayoutFailed = errors.New("sourceview: selection layout failed")

var errSelectionGeometry = errors.New("sourceview: selection geometry pending")

// cursorGeometry prepares only the cursor being planned. Results of earlier
// cursors are ordinary values, so a set larger than the page cache converges.
func (v *SourceView) cursorGeometry(c cursorState) (*paragraph, *textcore.Geometry, TextRange, error) {
	m := v.document.model
	row := m.LineAt(c.selection.Caret)
	r, _ := m.LineRange(row)
	p := v.paragraph(row)
	if p == nil {
		if _, failed := v.layoutFailures[v.paragraphKey(row)]; failed {
			return nil, nil, r, errSelectionLayoutFailed
		}
		if v.context == nil {
			return nil, nil, r, nil
		}
		return nil, nil, r, errSelectionGeometry
	}
	g := p.GeometryAt(c.selection.Caret - r.Start)
	if g == nil {
		return nil, nil, r, errSelectionGeometry
	}
	return p, g, r, nil
}
func (v *SourceView) navigateCursor(c cursorState, key events.Key, extend bool) (cursorState, error) {
	p, g, r, err := v.cursorGeometry(c)
	if err != nil {
		return c, err
	}
	state := v.operationState()
	row := v.document.model.LineAt(c.selection.Caret)
	if v.lineCollapsed(row) && (key == events.KeyArrowDown || key == events.KeyPageDown || key == events.KeyEnd) {
		v.expandLinePreview(row)
		if !state.valid(v) {
			return c, ErrOperationChanged
		}
	}
	old := c.selection
	at := old.Caret
	pos := textcore.Position{Offset: at - r.Start, Upstream: c.upstream}
	vertical := false
	if !extend && old.Anchor != old.Caret && (key == events.KeyArrowLeft || key == events.KeyArrowRight) {
		if key == events.KeyArrowLeft {
			at = old.Range().Start
		} else {
			at = old.Range().End
		}
	} else if g == nil {
		switch key {
		case events.KeyArrowLeft:
			at = v.adjacent(at, false)
		case events.KeyArrowRight:
			at = v.adjacent(at, true)
		case events.KeyHome:
			at = r.Start
		case events.KeyEnd:
			at = r.End
		}
	} else {
		switch key {
		case events.KeyArrowLeft, events.KeyArrowRight:
			right := key == events.KeyArrowRight
			if next, ok := g.Visual(pos, right); ok {
				pos = next
				at = r.Start + pos.Offset
			} else {
				at = v.adjacent(at, right)
				pos.Upstream = !right
			}
		case events.KeyHome, events.KeyEnd:
			pos = v.smartEdge(p, pos, key)
			at = r.Start + pos.Offset
		case events.KeyArrowUp, events.KeyArrowDown, events.KeyPageUp, events.KeyPageDown:
			vertical = true
			rect, ok := v.paragraphCaret(v.document.model.LineAt(at), p, pos, false)
			if !ok {
				return c, errSelectionGeometry
			}
			if !c.preferredSet {
				c.preferredX = float64(rect.X) - float64(v.gutter) - float64(padding) + float64(v.offset.X)
			}
			delta := max(v.lineHeight, rect.Height)
			if key == events.KeyPageUp || key == events.KeyPageDown {
				delta = max(v.lineHeight, v.viewport.Height-v.lineHeight)
			}
			if key == events.KeyArrowUp || key == events.KeyPageUp {
				delta = -delta
			}
			point := geometry.Point{X: float32(float64(v.gutter+padding) + c.preferredX - float64(v.offset.X)), Y: rect.Y + rect.Height/2 + delta}
			// Hit may prepare the destination in bounded steps. Require its resulting
			// logical row to have measured geometry before accepting the cursor result.
			next := v.hit(point)
			row := v.document.model.LineAt(next.Offset)
			target := v.paragraph(row)
			if target == nil || target.GeometryAt(next.Offset-v.displayRange(row).Start) == nil {
				return c, errSelectionGeometry
			}
			at, pos = next.Offset, next
		}
	}
	if key == events.KeyArrowLeft || key == events.KeyArrowRight {
		at = v.foldNavigationTarget(at, key == events.KeyArrowRight)
	}
	c.selection.Caret = at
	if !extend {
		c.selection.Anchor = at
	}
	c.upstream = pos.Upstream
	c.preferredSet = vertical
	return c, nil
}
func (v *SourceView) multipleNavigate(key events.Key, extend bool) {
	state := v.operationState()
	out := v.cursors()
	for i, c := range out {
		next, err := v.navigateCursor(c, key, extend)
		if err != nil {
			return
		}
		if !state.valid(v) {
			return
		}
		out[i] = next
	}
	v.document.model.BreakUndoGroup()
	v.setCursorStatesKeepingCompletion(normalizeCursors(out), true, nil)
}

type multipleInput struct {
	cursors   []cursorState
	plans     []editor.Plan
	pairs     []plannedPair
	after     []cursorState
	next      int
	layout    uint64
	generated int
	merge     textcore.BatchMerge
}

// advanceMultipleInput shares the caller's deadline across all cursor work;
// it never restarts a 2ms preparation budget for every cursor in one turn.
func (v *SourceView) advanceMultipleInput(q *inputBatch, a deferredInput, deadline time.Time) (bool, error) {
	if q.multiple == nil {
		q.multiple = &multipleInput{cursors: v.cursors(), layout: v.layoutEpoch, merge: a.merge}
	}
	work := q.multiple
	if work.layout != v.layoutEpoch {
		work.next = 0
		work.generated = 0
		work.merge = a.merge
		work.plans = nil
		work.pairs = nil
		work.after = nil
		work.layout = v.layoutEpoch
	}
	for count := 0; work.next < len(work.cursors) && count < 16; count++ {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return false, nil
		}
		c := work.cursors[work.next]
		// All measured edits snap their own endpoints before computing a plan.
		if a.geometry && v.projection == nil {
			snap := func(at int, forward bool) (int, error) {
				_, g, r, err := v.cursorGeometry(cursorState{selection: TextSelection{Anchor: at, Caret: at}})
				if err != nil {
					return at, err
				}
				if g == nil {
					return at, nil
				}
				return r.Start + g.Snap(at-r.Start, forward), nil
			}
			s := c.selection
			forward := s.Anchor <= s.Caret
			var err error
			c.selection.Caret, err = snap(s.Caret, forward)
			if err != nil {
				return false, err
			}
			if s.Anchor == s.Caret {
				c.selection.Anchor = c.selection.Caret
			} else {
				c.selection.Anchor, err = snap(s.Anchor, !forward)
				if err != nil {
					return false, err
				}
			}
		}

		if !q.valid(v) {
			return false, ErrOperationChanged
		}
		if a.navigate != nil {
			next, err := a.navigate(c)
			if err != nil {
				return false, err
			}
			work.after = append(work.after, next)
		} else {
			var plan editor.Plan
			var pair plannedPair
			var err error
			if a.typed != nil {
				pair, err = v.typingPlan(c, *a.typed)
				plan = pair.plan
			} else {
				plan, err = a.plan(c)
			}
			if err != nil {
				return false, err
			}
			for _, e := range plan.Edits {
				if len(e.Text) > (64<<20)-work.generated {
					return false, ErrOperationLimit
				}
				work.generated += len(e.Text)
			}
			if a.deletion && work.merge != textcore.MergeNone {
				forward := a.merge == textcore.MergeForwardDelete
				at := v.adjacent(c.selection.Caret, forward)
				want := TextRange{Start: min(at, c.selection.Caret), End: max(at, c.selection.Caret)}
				if len(plan.Edits) != 1 || plan.Edits[0].Range != want || plan.Edits[0].Text != "" {
					work.merge = textcore.MergeNone
				}
			}
			work.plans = append(work.plans, plan)
			work.pairs = append(work.pairs, pair)
		}
		if !q.valid(v) {
			return false, ErrOperationChanged
		}
		work.next++
	}
	if work.next < len(work.cursors) {
		return false, nil
	}
	q.multiple = nil
	if a.navigate != nil {
		v.document.model.BreakUndoGroup()
		v.setCursorStatesKeepingCompletion(normalizeCursors(work.after), true, nil)
		return true, nil
	}
	if a.typed != nil {
		merge := work.merge
		if a.typed.Composed || v.compositionSeen || v.projection != nil {
			merge = textcore.MergeNone
		}
		if v.projection != nil {
			v.projection = nil
			v.invalidateLayout()
		}
		v.compositionSeen = false
		return true, v.applyPairPlans(work.pairs, merge)
	}
	if a.deletion && v.unfoldMultipleDeletion(work.cursors, work.plans) {
		return true, nil
	}
	edits, after, err := combinePlans(work.plans)
	if err != nil {
		return false, err
	}
	return true, v.applyMultipleInput(edits, after, work.merge)
}

func replacementPlan(c cursorState, text string) editor.Plan {
	text = textcore.Normalize(text)
	r := c.selection.Range()
	at := r.Start + len(text)
	return editor.Plan{Before: c.selection, After: TextSelection{Anchor: at, Caret: at}, Edits: []TextEdit{{Range: r, Text: text}}}
}
func (v *SourceView) configureMultipleKey(a *deferredInput, event events.KeyEvent) {
	if command, bound := v.keyCommand(event); bound {
		switch command {
		case CommandAddCursorAbove, CommandAddCursorBelow, CommandSelectNextOccurrence, CommandSkipNextOccurrence, CommandSelectAllOccurrences, CommandSplitSelectionLines, CommandColumnSelectLeft, CommandColumnSelectRight, CommandColumnSelectUp, CommandColumnSelectDown:
			a.selectionCommand = command
			a.geometry = false
			return
		case CommandMoveWordLeft, CommandMoveWordRight, CommandSelectWordLeft, CommandSelectWordRight, CommandMoveWordPartLeft, CommandMoveWordPartRight, CommandSelectWordPartLeft, CommandSelectWordPartRight:
			forward := command == CommandMoveWordRight || command == CommandSelectWordRight || command == CommandMoveWordPartRight || command == CommandSelectWordPartRight
			extend := command == CommandSelectWordLeft || command == CommandSelectWordRight || command == CommandSelectWordPartLeft || command == CommandSelectWordPartRight
			part := command == CommandMoveWordPartLeft || command == CommandMoveWordPartRight || command == CommandSelectWordPartLeft || command == CommandSelectWordPartRight
			a.navigate = func(c cursorState) (cursorState, error) { return v.navigateWordCursor(c, forward, extend, part) }
			return
		case CommandDeleteWordLeft, CommandDeleteWordRight, CommandDeleteWordPartLeft, CommandDeleteWordPartRight:
			forward := command == CommandDeleteWordRight || command == CommandDeleteWordPartRight
			word := 1
			if command == CommandDeleteWordPartLeft || command == CommandDeleteWordPartRight {
				word = 2
			}
			a.deletion = true
			a.plan = func(c cursorState) (editor.Plan, error) { return v.deleteCursorPlan(c, forward, word) }
			return
		case CommandMoveLineStart, CommandMoveLineEnd, CommandSelectLineStart, CommandSelectLineEnd:
			key := events.KeyHome
			if command == CommandMoveLineEnd || command == CommandSelectLineEnd {
				key = events.KeyEnd
			}
			extend := command == CommandSelectLineStart || command == CommandSelectLineEnd
			a.navigate = func(c cursorState) (cursorState, error) { return v.navigateCursor(c, key, extend) }
			return
		case CommandMoveDocumentStart, CommandMoveDocumentEnd, CommandSelectDocumentStart, CommandSelectDocumentEnd:
			at := 0
			if command == CommandMoveDocumentEnd || command == CommandSelectDocumentEnd {
				at = v.document.Len()
			}
			extend := command == CommandSelectDocumentStart || command == CommandSelectDocumentEnd
			a.navigate = func(c cursorState) (cursorState, error) {
				c.selection.Caret = at
				if !extend {
					c.selection.Anchor = at
				}
				c.preferredSet = false
				c.upstream = false
				return c, nil
			}
			return
		}
	}

	if event.Modifiers&^events.ModifierShift != 0 {
		return
	}
	shift := event.Modifiers&events.ModifierShift != 0
	switch event.Key {
	case events.KeyArrowLeft, events.KeyArrowRight, events.KeyArrowUp, events.KeyArrowDown, events.KeyPageUp, events.KeyPageDown, events.KeyHome, events.KeyEnd:
		a.navigate = func(c cursorState) (cursorState, error) { return v.navigateCursor(c, event.Key, shift) }
	case events.KeyEnter:
		a.plan = v.multipleNewlinePlan
	case events.KeyTab:
		if v.acceptsTab && !v.escapeTab {
			a.plan = func(c cursorState) (editor.Plan, error) { return v.tabCursorPlan(c, shift) }
		}
	case events.KeyBackspace, events.KeyDelete:
		a.deletion = true
		a.merge = textcore.MergeBackwardDelete
		if event.Key == events.KeyDelete {
			a.merge = textcore.MergeForwardDelete
		}
		a.plan = func(c cursorState) (editor.Plan, error) {
			return v.deleteCursorPlan(c, event.Key == events.KeyDelete, 0)
		}
	}
}

func (v *SourceView) navigateWordCursor(c cursorState, forward, extend, part bool) (cursorState, error) {
	s := c.selection
	at := s.Caret
	if part && !extend && s.Anchor != s.Caret {
		at = s.Range().Start
		if forward {
			at = s.Range().End
		}
	} else if part {
		at = editor.WordPartBoundary(v.document.model, at, forward, false, v.definitionAt(at).IsWord)
	} else {
		at = editor.WordBoundaryWith(v.document.model, at, forward, false, v.definitionAt(at).IsWord)
	}
	at = v.foldNavigationTarget(at, forward)
	_, g, r, err := v.cursorGeometry(cursorState{selection: TextSelection{Anchor: at, Caret: at}})
	if err != nil {
		return c, err
	}
	if g != nil {
		at = r.Start + g.Snap(at-r.Start, forward)
	}
	c.selection.Caret = at
	if !extend {
		c.selection.Anchor = at
	}
	c.upstream = false
	c.preferredSet = false
	return c, nil
}
