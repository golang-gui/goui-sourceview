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
	state, bindingsEpoch := v.operationState(), v.keyBindingsEpoch
	command, bound := v.keyCommand(event)
	if v.completionKey(event) {
		event.PreventDefault()
		ctx.StopPropagation()
		return
	}
	// Closing transient UI invokes external callbacks. They may replace the
	// document, bindings or focus; the same key must not edit replacement state.
	if v.Destroyed() || !v.mounted || !state.valid(v) || v.keyBindingsEpoch != bindingsEpoch || !v.Focused() || !gui.IsEnabled(v) {
		event.PreventDefault()
		ctx.StopPropagation()
		return
	}
	if bound && v.runBoundCommand(command) {
		event.PreventDefault()
		ctx.StopPropagation()
		return
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
				return
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
	if handled {
		event.PreventDefault()
		ctx.StopPropagation()
	}
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
