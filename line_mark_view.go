package sourceview

import (
	"fmt"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/graphics"
)

type LineMarkAction uint8

const (
	LineMarkToggle LineMarkAction = iota
	LineMarkToggleEnabled
	LineMarkContextMenu
)

// LineMarkEvent is a request, not an automatic mutation. Line is the visible
// zero-based model line; Marks also includes marks hidden by its collapsed fold.
// Position is widget-local DIP, suitable for an application-owned popover.
type LineMarkEvent struct {
	Document *Document
	Revision uint64
	Line     int
	Marks    []LineMarkInfo
	Action   LineMarkAction
	Position geometry.Point
}

func (v *SourceView) ShowLineMarks() bool { return v.showLineMarks }
func (v *SourceView) SetShowLineMarks(show bool) {
	if v.showLineMarks != show {
		v.showLineMarks = show
		v.invalidateLayout()
	}
}

// ConnectLineMarkAction handles marker clicks, Shift-clicks, context requests
// and explicitly bound CommandToggleLineMark. The application owns breakpoints.
func (v *SourceView) ConnectLineMarkAction(fn func(LineMarkEvent)) signal.Handle {
	return v.lineMarkSignal.Connect(fn)
}

func (v *SourceView) markRow(line int) int {
	for _, hidden := range v.hiddenLines {
		if line < hidden.start {
			break
		}
		if line < hidden.end {
			return hidden.start - 1
		}
	}
	return line
}

func (v *SourceView) marksOnRow(row int) []LineMarkInfo {
	var marks []LineMarkInfo
	for _, mark := range v.document.LineMarks() {
		if mark.Valid && v.markRow(mark.Line) == row {
			marks = append(marks, mark)
		}
	}
	return marks
}

func (v *SourceView) lineMarkRect(row int) geometry.Rectangle {
	return geometry.Rect(0, padding+v.paragraphTop(row)-v.offset.Y, v.markColumn, v.lineHeight)
}

func (v *SourceView) lineMarkHit(point geometry.Point) (int, bool) {
	if !v.showLineMarks || v.projection != nil || point.X < 0 || point.X >= v.markColumn || point.Y < 0 || point.Y >= v.viewport.Height {
		return 0, false
	}
	row := v.paragraphAt(point.Y + v.offset.Y - padding)
	r := v.lineMarkRect(row)
	return row, point.Y >= r.Y && point.Y < r.Y+r.Height
}

func (v *SourceView) requestLineMark(row int, action LineMarkAction, point geometry.Point) {
	if !v.mounted || v.Destroyed() || !gui.IsEnabled(v) || row < 0 || row >= v.document.model.LineCount() {
		return
	}
	v.lineMarkSignal.Emit(LineMarkEvent{v.document, v.document.model.Revision(), row, v.marksOnRow(row), action, point})
}

// SetExecutionLine sets a view-local, zero-based execution anchor. It neither
// moves the selection nor scrolls; use RevealLine separately. -1 clears it.
func (v *SourceView) SetExecutionLine(line int) error {
	if line == -1 {
		v.ClearExecutionLine()
		return nil
	}
	if line < 0 || line >= v.document.model.LineCount() {
		return fmt.Errorf("sourceview: invalid execution line %d", line)
	}
	r, _ := v.document.model.LineRange(line)
	v.execution = &trackedLineMark{offset: r.Start, revision: v.document.model.Revision(), valid: true}
	v.RequestPaint()
	return nil
}

func (v *SourceView) ClearExecutionLine() { v.execution = nil; v.RequestPaint() }

// ExecutionLine returns -1,false if absent, invalidated, or awaiting model changes.
func (v *SourceView) ExecutionLine() (int, bool) {
	if e := v.execution; e != nil && e.valid && e.revision == v.document.model.Revision() {
		return v.document.model.LineAt(e.offset), true
	}
	return -1, false
}

// RevealLine expands containing folds and requests visibility using the current
// height index, without moving the caret or synchronously measuring the document.
func (v *SourceView) RevealLine(line int) error {
	if !v.mounted {
		return ErrNotMounted
	}
	if line < 0 || line >= v.document.model.LineCount() {
		return fmt.Errorf("sourceview: invalid reveal line %d", line)
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return ErrOperationChanged
	}
	r, _ := v.document.model.LineRange(line)
	if v.unfoldSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start}) {
		v.foldingSignal.Emit()
		if !state.valid(v) || v.Destroyed() {
			return ErrOperationChanged
		}
	}
	v.reveal = nil
	v.pendingAnchor = false
	v.anchor.valid = false
	v.scrollSignal.Emit(geometry.Rect(v.offset.X, padding+v.paragraphTop(line), v.viewport.Width, v.lineHeight))
	return nil
}

func lineMarkLabel(mark LineMarkInfo) string {
	label := mark.Label
	if label == "" {
		switch mark.Kind {
		case MarkConditionalBreakpoint:
			label = resource.ConditionalBreakpoint()
		case MarkLogpoint:
			label = resource.Logpoint()
		case MarkBookmark:
			label = resource.Bookmark()
		default:
			label = resource.Breakpoint()
		}
	}
	switch mark.State {
	case MarkDisabled:
		label += " · " + resource.MarkDisabled()
	case MarkPending:
		label += " · " + resource.MarkPending()
	case MarkRejected:
		label += " · " + resource.MarkRejected()
	}
	return fmt.Sprintf("%s (%d)", label, mark.Line+1)
}

func (v *SourceView) lineMarkLabel(row int) string {
	var labels []string
	for _, m := range v.marksOnRow(row) {
		labels = append(labels, lineMarkLabel(m))
		if len(labels) == 16 {
			break
		}
	}
	if len(labels) == 0 {
		return resource.ToggleBreakpoint()
	}
	return strings.Join(labels, "\n")
}

// Tooltip resolves marker help without replacing an application tooltip.
func (v *SourceView) Tooltip() string {
	if v.pointer != nil && v.pointer.Hover() {
		if row, ok := v.lineMarkHit(v.pointerPoint); ok {
			return v.lineMarkLabel(row)
		}
	}
	return v.WidgetBase.Tooltip()
}

func (v *SourceView) paintLineMarks(p gui.Painter) {
	if !v.showLineMarks || v.projection != nil {
		return
	}
	rows := make(map[int][]LineMarkInfo)
	for _, mark := range v.document.LineMarks() {
		if mark.Valid {
			row := v.markRow(mark.Line)
			rows[row] = append(rows[row], mark)
		}
	}
	for row, marks := range rows {
		r := v.lineMarkRect(row)
		if r.Y+r.Height <= 0 || r.Y >= v.viewport.Height {
			continue
		}
		m := marks[0]
		part, fallback := "line-mark", rgb(0xd83b36)
		if m.Kind == MarkBookmark {
			part, fallback = "bookmark", rgb(0x3579c8)
		}
		if m.State == MarkDisabled || m.State == MarkPending {
			part, fallback = "line-mark-muted", v.colors.GutterForeground
		}
		c := graphics.ColorOf(v.partColor(part, fallback))
		x, y := r.Width*.38, r.Y+r.Height/2
		size := min(5.5, r.Height*.27)
		switch m.Kind {
		case MarkLogpoint:
			path := graphics.MoveTo(x, y-size).LineTo(x+size, y).LineTo(x, y+size).LineTo(x-size, y).Close()
			if m.State == MarkEnabled {
				p.FillPath(path, c)
			} else {
				p.DrawPath(path, 1.3, c)
			}
		case MarkBookmark:
			path := graphics.MoveTo(x-size*.7, y-size).LineTo(x+size*.7, y-size).LineTo(x+size*.7, y+size).LineTo(x, y+size*.5).LineTo(x-size*.7, y+size).Close()
			if m.State == MarkEnabled {
				p.FillPath(path, c)
			} else {
				p.DrawPath(path, 1.3, c)
			}
		default:
			if m.State == MarkEnabled {
				p.FillEllipse(geometry.Point{X: x, Y: y}, size, size, c)
			} else {
				p.DrawEllipse(geometry.Point{X: x, Y: y}, size, size, 1.3, c)
			}
			if m.Kind == MarkConditionalBreakpoint {
				p.FillEllipse(geometry.Point{X: x, Y: y}, 1.5, 1.5, graphics.ColorOf(v.colors.GutterBackground))
			}
		}
		if m.State == MarkRejected {
			p.DrawLine(geometry.Point{X: x - size, Y: y + size}, geometry.Point{X: x + size, Y: y - size}, 1.3, c)
		}
		if m.State == MarkPending {
			p.FillEllipse(geometry.Point{X: x, Y: y}, 1, 1, c)
		}
		if len(marks) > 1 {
			p.FillEllipse(geometry.Point{X: x + size + 2, Y: y - size}, 1.5, 1.5, c)
		}
	}
	if line, ok := v.ExecutionLine(); ok {
		r := v.lineMarkRect(v.markRow(line))
		x, y := r.Width*.78, r.Y+r.Height/2
		p.FillPath(graphics.MoveTo(x-2, y-4).LineTo(x+3, y).LineTo(x-2, y+4).Close(), graphics.ColorOf(v.partColor("execution-arrow", rgb(0xc39400))))
	}
}

func (v *SourceView) snapshotLineMarks(info *gui.WidgetInfo, first, last int) {
	meta := info.Attributes["goui.sourceview"].(map[string]any)
	meta["lineMarks"] = v.showLineMarks
	line, _ := v.ExecutionLine()
	meta["executionLine"] = line
	if !v.showLineMarks || v.projection != nil {
		return
	}
	for row, count := first, 0; row <= last && count < 256; row, count = v.nextDisplayRow(row), count+1 {
		r := v.lineMarkRect(row)
		if r.Y+r.Height <= 0 || r.Y >= v.viewport.Height {
			continue
		}
		child := gui.WidgetInfo{ID: fmt.Sprintf("%s.mark.%d", info.ID, row), Role: gui.RoleButton, Name: v.lineMarkLabel(row), Bounds: r.Translate(info.Bounds.Pos), Visible: true, Enabled: gui.IsEnabled(v), Actions: []gui.Action{gui.ActionClick}}
		child.SetAttribute("goui.sourceview.lineMark", map[string]any{"line": row, "marks": v.marksOnRow(row)})
		info.Children = append(info.Children, child)
	}
}
