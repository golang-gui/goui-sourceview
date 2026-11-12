package sourceview

import (
	"fmt"
	"slices"
	"time"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
)

// SelectionModifiers configures exact pointer gestures. Zero disables a
// gesture; plain/Shift-only gestures are reserved for ordinary selection.
// Defaults are Alt for adding selections and Alt+Shift for column selection.
type SelectionModifiers struct{ Add, Column gui.KeyModifiers }

func (v *SourceView) SelectionModifiers() SelectionModifiers { return v.selectionModifiers }
func (v *SourceView) SetSelectionModifiers(value SelectionModifiers) error {
	a, ok := value.Add.Resolve()
	if !ok {
		return fmt.Errorf("sourceview: invalid add-selection modifiers")
	}
	b, ok := value.Column.Resolve()
	if !ok {
		return fmt.Errorf("sourceview: invalid column-selection modifiers")
	}
	if a != 0 && a&^events.ModifierShift == 0 || b != 0 && b&^events.ModifierShift == 0 || a != 0 && a == b {
		return fmt.Errorf("sourceview: conflicting selection modifiers")
	}
	v.selectionModifiers = value
	return nil
}
func defaultSelectionModifiers() SelectionModifiers {
	return SelectionModifiers{Add: gui.ModAlt, Column: gui.ModAlt | gui.ModShift}
}
func (v *SourceView) selectionGesture(mods events.Modifiers) int {
	a, _ := v.selectionModifiers.Add.Resolve()
	b, _ := v.selectionModifiers.Column.Resolve()
	if a != 0 && mods == a {
		return 1
	}
	if b != 0 && mods == b {
		return 2
	}
	return 0
}

type columnSelection struct {
	state                      operationState
	layout                     uint64
	anchor, target             textcore.Position
	left, right                float64
	before                     []cursorState
	mouse, pending, hitPending bool
	point                      geometry.Point
	pointY                     float64
	start, end, next           int
	targetStart, targetIndex   int
	results                    []cursorState
	initialized                bool
	gesture                    uint64
}

func (v *SourceView) beginColumn(point geometry.Point, pos textcore.Position) {
	v.pendingInput = nil
	v.columnSelection = &columnSelection{state: v.operationState(), layout: v.layoutEpoch, anchor: pos, target: pos, left: float64(point.X-v.gutter-padding) + float64(v.offset.X), right: float64(point.X-v.gutter-padding) + float64(v.offset.X), before: v.cursors(), mouse: true, pending: true, point: point, pointY: float64(point.Y) + float64(v.offset.Y), gesture: v.selectionGestureID}
	v.requestLayoutWork()
	v.RequestPaint()
}
func (v *SourceView) updateColumn(point geometry.Point) {
	c := v.columnSelection
	if c == nil {
		return
	}
	right := float64(point.X-v.gutter-padding) + float64(v.offset.X)
	y := float64(point.Y) + float64(v.offset.Y)
	if c.point == point && c.right == right && c.pointY == y {
		return
	}
	c.point = point
	c.pointY = y
	c.hitPending = true
	c.pending = true
	c.initialized = false
	c.results = nil
	c.right = right
	v.requestLayoutWork()
	v.RequestPaint()
}
func (v *SourceView) columnFrame(at int) (*paragraph, *textcore.Geometry, float64, TextRange, error) {
	row := v.document.model.LineAt(at)
	r := v.displayRange(row)
	p := v.paragraph(row)
	if p == nil {
		if _, failed := v.layoutFailures[v.paragraphKey(row)]; failed {
			return nil, nil, 0, r, errSelectionLayoutFailed
		}
		return nil, nil, 0, r, errSelectionGeometry
	}
	if p.Paged != nil {
		frame := p.Paged.GeometryAt(at - r.Start)
		if frame == nil {
			return nil, nil, 0, r, errSelectionGeometry
		}
		return p, &frame.Geometry, frame.X, r, nil
	}
	return p, &p.Geometry, 0, r, nil
}
func (v *SourceView) columnLineStart(pos textcore.Position) (int, error) {
	_, g, _, r, err := v.columnFrame(pos.Offset)
	if err != nil {
		return 0, err
	}
	if v.wrap == WrapNone {
		return r.Start, nil
	}
	return r.Start + g.LineMetrics(g.LineIndex(textcore.Position{Offset: pos.Offset - r.Start, Upstream: pos.Upstream})).Start, nil
}
func (v *SourceView) advanceColumn(c *columnSelection, deadline time.Time) (bool, error) {
	if v.columnSelection != c || !c.state.valid(v) || c.layout != v.layoutEpoch || v.Destroyed() {
		return false, ErrOperationChanged
	}
	if c.hitPending {
		pos, _, measured := v.hitCaret(c.point)
		if !measured {
			return false, errSelectionGeometry
		}
		c.target = pos
		c.hitPending = false
	}
	if !c.initialized {
		start, err := v.columnLineStart(c.anchor)
		if err != nil {
			return false, err
		}
		end, err := v.columnLineStart(c.target)
		if err != nil {
			return false, err
		}
		c.start, c.end = min(start, end), max(start, end)
		c.targetStart = end
		c.next = c.start
		c.results = nil
		c.initialized = true
	}
	for count := 0; c.next <= c.end && count < 32; count++ {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return false, nil
		}
		if len(c.results) >= maxSelections {
			return false, ErrOperationLimit
		}
		p, g, x, r, err := v.columnFrame(c.next)
		if err != nil {
			return false, err
		}
		if v.wrap == WrapNone && p.Paged != nil {
			frame, ready, err := p.Paged.ColumnGeometry(c.left, c.right)
			if err == view.ErrColumnWindow {
				return false, ErrOperationLimit
			}
			if err != nil {
				return false, err
			}
			if !ready {
				return false, errSelectionGeometry
			}
			g, x = &frame.Geometry, frame.X
		}
		index := g.LineIndex(textcore.Position{Offset: c.next - r.Start})
		line := g.LineMetrics(index)
		selected, err := view.ColumnRange(g, index, float32(c.left-x), float32(c.right-x))
		if err != nil {
			return false, err
		}
		selected.Anchor += r.Start
		selected.Caret += r.Start
		cursor := cursorState{selection: selected.Selection, upstream: selected.Upstream}
		c.results = append(c.results, cursor)
		lineStart := r.Start + line.Start
		if v.wrap == WrapNone {
			lineStart = r.Start
		}
		if lineStart == c.targetStart {
			c.target = textcore.Position{Offset: selected.Caret, Upstream: selected.Upstream}
			c.targetIndex = len(c.results) - 1
		}
		next := r.Start + line.Start + line.Length
		if v.wrap == WrapNone || next >= r.End || v.lineCollapsed(v.document.model.LineAt(c.next)) {
			row := v.neighboringRow(v.document.model.LineAt(c.next), 1)
			if row < 0 || row >= v.document.model.LineCount() {
				c.next = v.document.Len() + 1
				break
			}
			next = v.displayRange(row).Start
		}
		if next <= c.next {
			return false, fmt.Errorf("sourceview: column layout did not advance")
		}
		c.next = next
		if !c.state.valid(v) || c.layout != v.layoutEpoch {
			return false, ErrOperationChanged
		}
	}
	if c.next <= c.end {
		return false, nil
	}
	// The active end is primary regardless of upward/downward dragging.
	target := c.targetIndex
	result := make([]cursorState, 0, len(c.results))
	result = append(result, c.results[target])
	result = append(result, c.results[:target]...)
	result = append(result, c.results[target+1:]...)
	before := v.operationState()
	v.setCursorStates(normalizeCursors(result), false, nil, c.gesture, true)
	if v.Destroyed() || v.document != before.document || v.mountEpoch != before.mount || v.document.Revision() != before.revision || v.selectionEpoch != before.selection+1 {
		return false, ErrOperationChanged
	}
	c.state = v.operationState()
	c.pending = false
	c.initialized = false
	c.results = nil
	v.columnSelection = c
	return true, nil
}
func (v *SourceView) columnStep(deadline time.Time) {
	c := v.columnSelection
	if c == nil || !c.pending || v.pendingInput != nil {
		return
	}
	done, err := v.advanceColumn(c, deadline)
	if err != nil && err != errSelectionGeometry {
		v.columnSelection = nil
		if err == view.ErrColumnDirection || err == ErrOperationLimit {
			v.setCursorStatesKeepingCompletion(slices.Clone(c.before), false, nil)
		}
		if err != ErrOperationChanged && err != errSelectionLayoutFailed {
			v.report(err)
		}
		return
	}
	if !done {
		v.RequestPaint()
	}
}
