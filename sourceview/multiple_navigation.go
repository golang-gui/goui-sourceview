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
				c.preferredX = rect.X - v.gutter - padding + v.offset.X
			}
			delta := max(v.lineHeight, rect.Height)
			if key == events.KeyPageUp || key == events.KeyPageDown {
				delta = max(v.lineHeight, v.viewport.Height-v.lineHeight)
			}
			if key == events.KeyArrowUp || key == events.KeyPageUp {
				delta = -delta
			}
			point := geometry.Point{X: v.gutter + padding + c.preferredX - v.offset.X, Y: rect.Y + rect.Height/2 + delta}
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
	cursors []cursorState
	plans   []editor.Plan
	after   []cursorState
	next    int
	layout  uint64
}

// advanceMultipleInput shares the caller's deadline across all cursor work;
// it never restarts a 2ms preparation budget for every cursor in one turn.
func (v *SourceView) advanceMultipleInput(q *inputBatch, a deferredInput, deadline time.Time) (bool, error) {
	if q.multiple == nil {
		q.multiple = &multipleInput{cursors: v.cursors(), layout: v.layoutEpoch}
	}
	work := q.multiple
	if work.layout != v.layoutEpoch {
		work.next = 0
		work.plans = nil
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
			_, g, r, err := v.cursorGeometry(c)
			if err != nil {
				return false, err
			}
			if g != nil {
				forward := c.selection.Anchor <= c.selection.Caret
				c.selection.Caret = r.Start + g.Snap(c.selection.Caret-r.Start, forward)
				if c.selection.Anchor == work.cursors[work.next].selection.Caret {
					c.selection.Anchor = c.selection.Caret
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
			plan, err := a.plan(c)
			if err != nil {
				return false, err
			}
			work.plans = append(work.plans, plan)
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
	edits, after, err := combinePlans(work.plans)
	if err != nil {
		return false, err
	}
	return true, v.applyMultiple(edits, after)
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
			a.plan = func(c cursorState) (editor.Plan, error) {
				if shift {
					return editor.Indent(v.document.model, c.selection, editor.IndentOptions(v.indent), true)
				}
				return replacementPlan(c, v.indentUnit()), nil
			}
		}
	case events.KeyBackspace, events.KeyDelete:
		a.plan = func(c cursorState) (editor.Plan, error) {
			r := c.selection.Range()
			if r.Start == r.End {
				if event.Key == events.KeyDelete {
					r.End = v.adjacent(r.End, true)
				} else {
					r.Start = v.adjacent(r.Start, false)
				}
			}
			s := TextSelection{Anchor: r.Start, Caret: r.Start}
			return editor.Plan{Before: c.selection, After: s, Edits: []TextEdit{{Range: r}}}, nil
		}
	}
}
