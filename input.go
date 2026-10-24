package sourceview

import (
	"math"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

func (v *SourceView) installInput() {
	v.pointer = gui.NewMotionEventController()
	v.pointer.ConnectMotion(func(info gui.MotionInfo) { v.pointerPoint = info.Position })
	v.AddEventController(v.pointer)
	companions := gui.NewKeyEventController()
	companions.SetPhase(gui.PhaseCapture)
	companions.ConnectKeyDown(func(ctx gui.EventContext, event events.KeyEvent) {
		if !v.mounted || !gui.IsEnabled(v) || !visible(v) {
			return
		}
		command, bound := v.keyCommand(event)
		if bound && (command == CommandFind || command == CommandReplace || command == CommandGoTo) {
			v.pendingInput = nil
			v.runBoundCommand(command)
			event.PreventDefault()
			ctx.StopPropagation()
		}
	})
	v.AddEventController(companions)
	key := gui.NewKeyEventController()
	key.SetPhase(gui.PhaseBubble)
	key.ConnectKeyDown(v.keyDown)
	v.AddEventController(key)
	drag := gui.NewDragEventController()
	drag.SetPhase(gui.PhaseTarget)
	drag.SetThreshold(0)
	drag.ConnectBegin(func(point geometry.Point, mods events.Modifiers) {
		if !v.mounted || !gui.IsEnabled(v) {
			return
		}
		state := v.operationState()
		v.cancelPreedit()
		if !state.valid(v) || !v.mounted {
			return
		}
		v.document.model.BreakUndoGroup()
		if root, ok := v.Root().(gui.EventTarget); ok {
			root.SetFocusedWidget(v)
		}
		if !v.mounted {
			return
		}
		if line, ok := v.foldHit(point); ok {
			v.dragging = false
			v.ToggleFold(line)
			return
		}
		pos := v.hit(point)
		s := v.Selection()
		v.dragGutter = point.X < v.gutter
		v.dragPoint = point
		v.dragging = true
		v.dragUnit = 0
		if mods&events.ModifierShift != 0 {
			s.Caret = pos.Offset
		} else {
			s = textedit.Selection{Anchor: pos.Offset, Caret: pos.Offset}
		}
		now := time.Now()
		twice := now.Sub(v.lastClick) < 400*time.Millisecond && math.Abs(float64(point.X-v.lastClickPoint.X)) < 4 && math.Abs(float64(point.Y-v.lastClickPoint.Y)) < 4
		if twice && mods&events.ModifierShift == 0 {
			v.clickCount = v.clickCount%3 + 1
		} else {
			v.clickCount = 1
		}
		v.lastClick = now
		v.lastClickPoint = point
		if v.dragGutter {
			r := v.lineSelection(pos.Offset)
			v.dragUnit, v.dragRange = 2, r
			if mods&events.ModifierShift == 0 {
				s.Anchor = r.Start
			} else {
				v.dragRange = textedit.Range{Start: s.Anchor, End: s.Anchor}
			}
			s.Caret = r.End
		} else if v.clickCount > 1 {
			r := v.wordSelection(point)
			v.dragUnit = 1
			if v.clickCount == 3 {
				v.dragUnit = 2
				r = v.lineSelection(pos.Offset)
			}
			v.dragRange = r
			s = textedit.Selection{Anchor: r.Start, Caret: r.End}
		}
		v.setSelection(s, pos.Upstream, false)
	})
	drag.ConnectUpdate(func(point geometry.Point, _ events.Modifiers) {
		if v.dragging {
			v.dragPoint = point
			v.dragSelection(point)
		}
	})
	drag.ConnectEnd(func(point geometry.Point, _ events.Modifiers) {
		if v.dragging {
			v.dragSelection(point)
		}
		v.dragging = false
	})
	drag.ConnectCancel(func() { v.dragging = false })
	v.AddEventController(drag)
}

// Cursor resolves code, gutter and fold affordances without shaping text.
// A captured selection keeps the cursor chosen by its starting region.
func (v *SourceView) Cursor() gui.Cursor {
	if v.dragging {
		if v.dragGutter {
			return gui.CursorDefault
		}
		return v.WidgetBase.Cursor()
	}
	if v.findBar != nil && v.findBar.Visible() && v.findBar.pointer.ContainsHover() ||
		v.goToBar != nil && v.goToBar.Visible() && v.goToBar.pointer.ContainsHover() {
		return nil
	}
	if v.pointer != nil && v.pointer.Hover() {
		return v.cursorAt(v.pointerPoint)
	}
	return v.WidgetBase.Cursor()
}

func (v *SourceView) cursorAt(point geometry.Point) gui.Cursor {
	if _, ok := v.foldHit(point); ok {
		return gui.CursorPointing
	}
	if point.X < v.gutter {
		return gui.CursorDefault
	}
	return v.WidgetBase.Cursor()
}

func (v *SourceView) dragSelection(point geometry.Point) {
	if !v.mounted || !gui.IsEnabled(v) {
		v.dragging = false
		return
	}
	if point.Y < 0 || point.Y > v.viewport.Height || !v.dragGutter && (point.X < v.gutter || point.X > v.viewport.Width) {
		target := geometry.Rect(max(0, point.X+v.offset.X-v.gutter), max(0, point.Y+v.offset.Y), v.gutter+1, v.lineHeight)
		v.scrollSignal.Emit(target)
		if !v.mounted {
			return
		}
	}
	pos := v.hit(point)
	s := v.Selection()
	s.Caret = pos.Offset
	if v.dragUnit != 0 {
		r := v.wordSelection(point)
		if v.dragUnit == 2 {
			r = v.lineSelection(pos.Offset)
		}
		if r.Start < v.dragRange.Start {
			s.Anchor, s.Caret = v.dragRange.End, r.Start
		} else {
			s.Anchor, s.Caret = v.dragRange.Start, max(v.dragRange.End, r.End)
		}
	}
	v.setSelection(s, pos.Upstream, false)
}

func (v *SourceView) lineSelection(at int) textedit.Range {
	m := v.document.model
	row := m.LineAt(at)
	r, _ := m.LineRange(row)
	if row+1 < m.LineCount() {
		r.End++
	}
	return r
}

// Select the cluster under the pointer, including its trailing half. Nearest
// caret hit testing alone would select the following punctuation/word there.
func (v *SourceView) wordSelection(point geometry.Point) textedit.Range {
	pos := v.hit(point)
	row := v.document.model.LineAt(pos.Offset)
	r, _ := v.document.model.LineRange(row)
	if p := v.paragraph(row); p != nil && pos.Offset > r.Start {
		previous := p.geometry.Adjacent(pos.Offset-r.Start, false)
		origin := v.paragraphOrigin(row, p)
		local := geometry.Point{X: point.X - origin.X, Y: point.Y - origin.Y}
		for _, rect := range p.geometry.Selection(textedit.Range{Start: previous, End: pos.Offset - r.Start}) {
			if containsPoint(rect, local) {
				pos.Offset = r.Start + previous
				break
			}
		}
	}
	selected := editor.WordRange(v.document.model, pos.Offset, v.definitionAt(v.Selection().Caret).IsWord)
	if p := v.paragraph(row); p != nil && selected.End > selected.Start {
		selected.End = max(selected.End, r.Start+p.geometry.Adjacent(selected.Start-r.Start, true))
	}
	return selected
}

func containsPoint(r geometry.Rectangle, p geometry.Point) bool {
	return p.X >= r.X && p.X < r.X+r.Width && p.Y >= r.Y && p.Y < r.Y+r.Height
}
func (v *SourceView) keyDown(ctx gui.EventContext, event events.KeyEvent) {
	if !v.mounted || !v.Focused() || !gui.IsEnabled(v) {
		return
	}
	if v.inputKey(event) {
		event.PreventDefault()
		ctx.StopPropagation()
	}
}

func (v *SourceView) inputKey(event events.KeyEvent) bool {
	if event.Key == events.KeyEscape && event.Modifiers == 0 {
		v.pendingInput = nil
	}
	event.Handled = nil // The native dispatch response must never escape this event.
	command, bound := v.keyCommand(event)
	known, geometry, mutates := bound, true, false
	if bound {
		switch command {
		case CommandMoveDocumentStart, CommandMoveDocumentEnd, CommandSelectDocumentStart, CommandSelectDocumentEnd,
			CommandSelectAll, CommandFind, CommandReplace, CommandGoTo, CommandUndo, CommandRedo:
			geometry = false
		}
		switch command {
		case CommandUndo, CommandRedo, CommandCut, CommandPaste, CommandDeleteSelection, CommandMoveLinesUp, CommandMoveLinesDown,
			CommandDuplicateLinesUp, CommandDuplicateLinesDown, CommandDeleteLines, CommandToggleLineComment, CommandToggleBlockComment,
			CommandIndent, CommandOutdent, CommandInsertLineAbove, CommandInsertLineBelow, CommandDeleteWordLeft, CommandDeleteWordRight:
			mutates = true
		}
	} else if event.Modifiers&^events.ModifierShift == 0 {
		switch event.Key {
		case events.KeyArrowLeft, events.KeyArrowRight, events.KeyArrowUp, events.KeyArrowDown, events.KeyPageUp, events.KeyPageDown:
			known = true
		case events.KeyHome, events.KeyEnd:
			known, geometry = true, v.wrap != WrapNone
		case events.KeyBackspace, events.KeyDelete, events.KeyEnter:
			known, mutates = true, true
		case events.KeyTab:
			known, mutates = v.acceptsTab && !v.escapeTab, true
		}
	}
	if known && v.delayInput(deferredInput{apply: func() { v.applyKey(event) }, geometry: geometry, mutates: mutates}) {
		return true
	}
	return v.applyKey(event)
}

func (v *SourceView) applyKey(event events.KeyEvent) bool {
	state, bindingsEpoch := v.operationState(), v.keyBindingsEpoch
	command, bound := v.keyCommand(event)
	if v.completionKey(event) {
		return true
	}
	// Closing transient UI invokes external callbacks. They may replace the
	// document, bindings or focus; the same key must not edit replacement state.
	if v.Destroyed() || !v.mounted || !state.valid(v) || v.keyBindingsEpoch != bindingsEpoch || !v.Focused() || !gui.IsEnabled(v) {
		return true
	}
	if bound && v.runBoundCommand(command) {
		return true
	}
	mods := event.Modifiers
	shift := mods&events.ModifierShift != 0
	base := mods &^ events.ModifierShift
	handled := false
	run := func(f func() error) {
		handled = true
		if v.editable() == nil {
			v.report(f())
		}
	}
	if base == 0 {
		switch event.Key {
		case events.KeyEscape:
			v.cancelPreedit()
			v.escapeTab = true
			handled = true
		case events.KeyTab:
			if v.escapeTab {
				v.escapeTab = false
				return false
			}
			if v.acceptsTab {
				run(func() error { return v.tab(shift) })
			}
		case events.KeyEnter:
			run(v.newline)
		case events.KeyBackspace, events.KeyDelete:
			wasCompleting := v.completion != nil
			run(func() error {
				if v.unfoldDeleteBoundary(event.Key == events.KeyDelete) {
					return nil
				}
				if event.Key == events.KeyBackspace {
					if handled, err := v.deleteIndent(); handled {
						return err
					}
					if handled, err := v.deletePair(); handled {
						return err
					}
				}
				r := v.Selection().Range()
				if r.Start == r.End {
					if event.Key == events.KeyBackspace {
						r.Start = v.adjacent(r.Start, false)
					} else {
						r.End = v.adjacent(r.End, true)
					}
				}
				if handled, err := v.linkedTag(r, ""); handled {
					return err
				}
				return v.replace(r, "", false)
			})
			if wasCompleting && v.editable() == nil {
				v.report(v.complete(false))
			}
		case events.KeyArrowLeft, events.KeyArrowRight, events.KeyArrowUp, events.KeyArrowDown, events.KeyHome, events.KeyEnd, events.KeyPageUp, events.KeyPageDown:
			v.navigate(event.Key, shift)
			handled = true
		}
	}
	return handled
}

func (v *SourceView) moveTo(at int, upstream, extend bool) {
	v.moveToKeepingCompletion(at, upstream, extend, nil)
}

func (v *SourceView) moveToKeepingCompletion(at int, upstream, extend bool, keep *completionSession) {
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return
	}
	v.document.model.BreakUndoGroup()
	s := v.Selection()
	s.Caret = at
	if !extend {
		s.Anchor = at
	}
	v.setSelectionKeepingCompletion(s, upstream, true, keep)
}
func (v *SourceView) adjacent(at int, forward bool) int {
	m := v.document.model
	row := m.LineAt(at)
	r, _ := m.LineRange(row)
	if forward && at >= r.End {
		return min(m.Len(), r.End+1)
	}
	if !forward && at <= r.Start {
		return max(0, r.Start-1)
	}
	if p := v.paragraph(row); p != nil {
		return r.Start + p.geometryAt(at-r.Start).Adjacent(at-r.Start, forward)
	}
	if forward {
		at++
		for at < m.Len() && !m.IsRuneBoundary(at) {
			at++
		}
	} else {
		at--
		for at > 0 && !m.IsRuneBoundary(at) {
			at--
		}
	}
	return max(0, min(at, m.Len()))
}
func (v *SourceView) navigate(key events.Key, extend bool) {
	v.navigateKeepingCompletion(key, extend, nil)
}

func (v *SourceView) navigateKeepingCompletion(key events.Key, extend bool, keep *completionSession) {
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return
	}
	s := v.Selection()
	caret := s.Caret
	m := v.document.model
	row := m.LineAt(caret)
	r, _ := m.LineRange(row)
	p := v.paragraph(row)
	pos := textedit.Position{Offset: caret - r.Start, Upstream: v.upstream}
	if !extend && s.Anchor != s.Caret && (key == events.KeyArrowLeft || key == events.KeyArrowRight) {
		at := s.Range().Start
		if key == events.KeyArrowRight {
			at = s.Range().End
		}
		v.moveToKeepingCompletion(at, false, false, keep)
		return
	}
	if p == nil {
		switch key {
		case events.KeyArrowLeft:
			caret = v.adjacent(caret, false)
		case events.KeyArrowRight:
			caret = v.adjacent(caret, true)
		case events.KeyHome:
			caret = r.Start
		case events.KeyEnd:
			caret = r.End
		}
		v.moveToKeepingCompletion(caret, false, extend, keep)
		return
	}
	g := p.geometryAt(pos.Offset)
	vertical := false
	x := v.preferredX
	switch key {
	case events.KeyArrowLeft, events.KeyArrowRight:
		right := key == events.KeyArrowRight
		if next, ok := g.Visual(pos, right); ok {
			pos = next
			caret = r.Start + pos.Offset
		} else {
			caret = v.adjacent(caret, right)
			pos.Upstream = !right
		}
	case events.KeyHome, events.KeyEnd:
		pos = v.smartEdge(p, pos, key)
		caret = r.Start + pos.Offset
	case events.KeyArrowUp, events.KeyArrowDown, events.KeyPageUp, events.KeyPageDown:
		vertical = true
		if !v.preferredSet {
			x = g.Caret(pos).X
		}
		if p.paged != nil {
			rect, ok := v.paragraphCaret(row, p, pos, false)
			if !ok {
				break
			}
			delta := max(v.lineHeight, rect.Height)
			if key == events.KeyPageUp || key == events.KeyPageDown {
				delta = max(v.lineHeight, v.viewport.Height-v.lineHeight)
			}
			if key == events.KeyArrowUp || key == events.KeyPageUp {
				delta = -delta
			}
			at := v.hit(geometry.Point{X: rect.X, Y: rect.Y + rect.Height/2 + delta})
			caret, pos = at.Offset, at
			break
		}
		index := g.LineIndex(pos)
		down := key == events.KeyArrowDown || key == events.KeyPageDown
		delta := -1
		if down {
			delta = 1
		}
		if key == events.KeyPageUp || key == events.KeyPageDown {
			target := v.paragraphTop(row) + g.Caret(pos).Y + float32(delta)*max(v.lineHeight, v.viewport.Height-v.lineHeight)
			row = v.paragraphAt(max(0, target))
			r, _ = m.LineRange(row)
			p = v.paragraph(row)
			if p != nil {
				g = &p.geometry
			}
			if p != nil {
				pos = g.Hit(geometry.Point{X: x, Y: target - v.paragraphTop(row)})
				caret = r.Start + pos.Offset
			}
		} else {
			index += delta
			if index < 0 || index >= g.LineCount() {
				next := v.neighboringRow(row, delta)
				if next >= 0 && next < m.LineCount() {
					row = next
					r, _ = m.LineRange(row)
					p = v.paragraph(row)
					if p != nil {
						g = &p.geometry
					}
					if p != nil {
						index = 0
						if !down {
							index = g.LineCount() - 1
						}
					}
				} else {
					index = max(0, min(index, g.LineCount()-1))
				}
			}
			if p != nil {
				pos = g.HitLine(index, x)
				caret = r.Start + pos.Offset
			}
		}
	}
	if key == events.KeyArrowLeft || key == events.KeyArrowRight {
		caret = v.foldNavigationTarget(caret, key == events.KeyArrowRight)
	}
	v.moveToKeepingCompletion(caret, pos.Upstream, extend, keep)
	if vertical {
		v.preferredSet = true
		v.preferredX = x
	}
}

// Pending input belongs to one selection, document and binding generation.
// It retains operations after normal input dispatch, never an EventContext or
// native Handled pointer. Existing frame work prepares their measured stops.
type deferredInput struct {
	apply             func()
	geometry, mutates bool
}
type inputBatch struct {
	state    operationState
	bindings uint64
	readOnly bool
	actions  []deferredInput
}

func (q *inputBatch) valid(v *SourceView) bool {
	return q.state.valid(v) && q.bindings == v.keyBindingsEpoch && q.readOnly == v.readOnly && v.mounted && v.Focused() && gui.IsEnabled(v) && visible(v) && !v.Destroyed()
}

func (v *SourceView) inputGeometryReady() bool {
	if v.context == nil {
		return true
	}
	selection := v.Selection()
	for i, at := range []int{selection.Caret, selection.Anchor} {
		if i == 1 && at == selection.Caret {
			continue
		}
		row := v.document.model.LineAt(at)
		r, _ := v.document.model.LineRange(row)
		if r.End-r.Start <= 64*1024 {
			continue
		}
		p := v.paragraph(row)
		if p == nil {
			return false
		}
		if p.paged != nil && p.paged.GeometryAt(at-r.Start) == nil {
			return false
		}
	}
	return true
}

// An explicit model offset may have been clamped while its shaping window
// was unavailable. Normalize it before the next measured editing operation.
func (v *SourceView) inputSelection() textedit.Selection {
	s := v.Selection()
	if v.projection != nil {
		return s
	}
	snap := func(at int, forward bool) int {
		r, _ := v.document.model.LineRange(v.document.model.LineAt(at))
		if r.End-r.Start <= 64*1024 {
			return at
		}
		return v.snapOffset(at, forward)
	}
	if s.Anchor == s.Caret {
		s.Caret = snap(s.Caret, true)
		s.Anchor = s.Caret
	} else {
		forward := s.Caret > s.Anchor
		s.Anchor = snap(s.Anchor, !forward)
		s.Caret = snap(s.Caret, forward)
	}
	return s
}

func (v *SourceView) delayInput(action deferredInput) bool {
	if v.pendingInput != nil && !v.pendingInput.valid(v) {
		v.pendingInput = nil
	}
	if v.pendingInput == nil {
		if !action.geometry {
			return false
		}
		state := v.operationState()
		ready := v.inputGeometryReady()
		if !state.valid(v) || !v.mounted || !v.Focused() || v.Destroyed() {
			return true
		}
		if ready {
			corrected := v.inputSelection()
			if !state.valid(v) || v.Destroyed() {
				return true
			}
			if corrected == v.Selection() {
				return false
			}
		}
		v.pendingInput = &inputBatch{state: state, bindings: v.keyBindingsEpoch, readOnly: v.readOnly}
	}
	v.pendingInput.actions = append(v.pendingInput.actions, action)
	v.requestLayoutWork()
	v.RequestPaint()
	return true
}

func (v *SourceView) inputStep(deadline time.Time) {
	for work := 0; work < 8 && v.pendingInput != nil; work++ {
		q := v.pendingInput
		if !q.valid(v) {
			v.pendingInput = nil
			return
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			v.RequestPaint()
			return
		}
		action := q.actions[0]
		if action.geometry && !v.inputGeometryReady() {
			row := v.document.model.LineAt(v.Selection().Caret)
			if _, failed := v.layoutFailures[v.paragraphKey(row)]; failed {
				v.pendingInput = nil
				return
			}
			v.RequestPaint()
			return
		}
		if v.pendingInput != q || !q.valid(v) {
			return
		}
		if action.geometry {
			before := v.operationState()
			corrected := v.inputSelection()
			if v.pendingInput != q || !before.valid(v) {
				return
			}
			if corrected != v.Selection() {
				v.setSelection(corrected, v.upstream, true)
				if v.pendingInput != q {
					return
				}
				if !q.advance(v, before, false) {
					v.pendingInput = nil
					return
				}
			}
		}
		q.actions[0] = deferredInput{}
		q.actions = q.actions[1:]
		before := v.operationState()
		action.apply()
		if v.pendingInput != q {
			return
		}
		if !q.advance(v, before, action.mutates) {
			v.pendingInput = nil
			return
		}
		if len(q.actions) == 0 {
			v.pendingInput = nil
			return
		}
	}
	if v.pendingInput != nil {
		v.RequestPaint()
	}
}

func (q *inputBatch) advance(v *SourceView, before operationState, mutates bool) bool {
	after := v.operationState()
	allowedRevision := before.revision
	if mutates {
		allowedRevision++
	}
	if after.document != before.document || after.mount != before.mount || after.language != before.language ||
		after.revision > allowedRevision || after.selection > before.selection+1 {
		return false
	}
	q.state = after
	return q.valid(v)
}
