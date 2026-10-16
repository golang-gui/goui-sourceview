package sourceview

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
)

// This bounded popup owns at most eight native rows. Keyboard input stays in
// SourceView; pointer activation uses the popup's normal click controller.
type completionList struct {
	gui.WidgetBase
	view      *SourceView
	session   *completionSession
	first     int
	rowHeight float32
	width     float32
	rows      []typography.TextLayout
	epoch     uint64
	err       error
}

func newCompletionList(v *SourceView, s *completionSession) *completionList {
	l := &completionList{view: v, session: s, rowHeight: max(24, v.lineHeight+6)}
	l.ConnectUnmount(l.release)
	click := gui.NewClickEventController()
	click.ConnectClicked(func(ctx gui.EventContext) {
		if p, ok := ctx.Position(); ok {
			index := l.first + int((p.Y-4)/l.rowHeight)
			if p.Y >= 4 && index >= l.first && index < min(len(s.items), l.first+8) {
				v.report(v.acceptCompletion(s, index))
				ctx.StopPropagation()
			}
		}
	})
	l.AddEventController(click)
	motion := gui.NewMotionEventController()
	motion.ConnectMotion(func(info gui.MotionInfo) {
		index := l.first + int((info.Position.Y-4)/l.rowHeight)
		if info.Position.Y >= 4 && index >= l.first && index < min(l.first+8, len(s.items)) && s.index != index {
			s.index = index
			l.RequestPaint()
		}
	})
	l.AddEventController(motion)
	wheel := gui.NewWheelEventController()
	wheel.ConnectScroll(func(ctx gui.EventContext, e events.WheelEvent) {
		if e.DeltaY == 0 {
			return
		}
		delta := 1
		if e.DeltaY < 0 {
			delta = -1
		}
		s.index = max(0, min(len(s.items)-1, s.index+delta))
		l.refresh()
		ctx.StopPropagation()
	})
	l.AddEventController(wheel)
	return l
}

func (l *completionList) release() {
	l.epoch++
	rows := l.rows
	l.rows = nil
	for _, row := range rows {
		row.Destroy()
	}
}
func (l *completionList) refresh() {
	first := l.first
	if l.session.index < first {
		first = l.session.index
	}
	if l.session.index >= first+8 {
		first = l.session.index - 7
	}
	if first != l.first {
		l.first = first
		l.release()
		l.RequestLayout()
	}
	l.RequestPaint()
}
func (l *completionList) ensureRows() {
	if l.rows != nil || l.err != nil {
		return
	}
	l.width = 240
	context, epoch := l.view.context, l.epoch
	if context == nil || l.view.completion != l.session {
		return
	}
	for i := l.first; i < min(l.first+8, len(l.session.items)); i++ {
		item := l.session.items[i]
		text := item.Label
		if item.Detail != "" {
			text += "    " + item.Detail
		}
		native, err := context.NewTextLayout(text, typography.TextFormat{Font: l.view.font, TextColor: l.view.colors.Foreground}, 540, 1<<16)
		if l.epoch != epoch || l.view.completion != l.session || l.view.context != context {
			if native != nil {
				native.Destroy()
			}
			return
		}
		if err != nil {
			l.err = err
			l.release()
			if l.session.shown {
				l.view.closeCompletion()
				l.view.report(err)
			}
			return
		}
		l.rows = append(l.rows, native)
		w, _ := native.MeasureSize()
		if l.epoch != epoch || l.view.completion != l.session {
			return
		}
		l.width = max(l.width, min(560, w+20))
	}
}
func (l *completionList) Measure(c layout.Constraint) layout.Measurement {
	l.ensureRows()
	return layout.Measured(c.Clamp(geometry.Size{Width: l.width, Height: 8 + float32(min(8, len(l.session.items)))*l.rowHeight}))
}
func (l *completionList) Paint(p gui.Painter) {
	l.ensureRows()
	if l.view.completion != l.session {
		return
	}
	epoch := l.epoch
	colors := l.view.colors
	p.FillRect(geometry.Rect(0, 0, l.Rect().Width, l.Rect().Height), graphics.ColorOf(colors.Background))
	for i, row := range l.rows {
		y := 4 + float32(i)*l.rowHeight
		foreground := colors.Foreground
		if l.first+i == l.session.index {
			p.FillRect(geometry.Rect(4, y, l.Rect().Width-8, l.rowHeight), graphics.ColorOf(colors.SelectionBackground))
			foreground = colors.SelectionForeground
		}
		row.SetTextColor(0, len(row.Text()), foreground)
		if l.epoch != epoch || l.view.completion != l.session {
			return
		}
		_, h := row.MeasureSize()
		p.DrawTextLayout(geometry.Point{X: 10, Y: y + max(0, (l.rowHeight-h)/2)}, row)
	}
}
func (l *completionList) Snapshot() gui.WidgetInfo {
	info := l.WidgetBase.Snapshot()
	info.Role = gui.Role("completion-list")
	info.ItemCount = len(l.session.items)
	for i := l.first; i < min(l.first+8, len(l.session.items)); i++ {
		item := l.session.items[i]
		info.Children = append(info.Children, gui.WidgetInfo{Role: gui.Role("completion-item"), Text: item.Label, Visible: true, Enabled: true, Selected: i == l.session.index, Actions: []gui.Action{gui.ActionClick}, Bounds: geometry.Rect(info.Bounds.X+4, info.Bounds.Y+4+float32(i-l.first)*l.rowHeight, max(0, info.Bounds.Width-8), l.rowHeight)})
	}
	return info
}
