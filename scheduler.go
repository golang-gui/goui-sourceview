package sourceview

import (
	"time"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
)

type frameQueue struct {
	root   gui.Root
	handle signal.Handle
	views  map[*SourceView]struct{}
}

var queues = make(map[gui.Root]*frameQueue)

func attachFrame(v *SourceView) {
	root := v.Root()
	q := queues[root]
	if q == nil {
		q = &frameQueue{root: root, views: make(map[*SourceView]struct{})}
		queues[root] = q
		q.handle = root.ConnectFrame(q.frame)
	}
	q.views[v] = struct{}{}
}
func detachFrame(v *SourceView) {
	q := queues[v.Root()]
	if q == nil {
		return
	}
	delete(q.views, v)
	if len(q.views) == 0 {
		q.handle.Disconnect()
		delete(queues, v.Root())
	}
}
func (q *frameQueue) frame(_ time.Time) {
	deadline := time.Now().Add(4 * time.Millisecond)
	seen := make(map[*Document]bool)
	for v := range q.views {
		if !v.mounted || !visible(v) {
			continue
		}
		if activeFocus(v) && v.blink != nil && !v.blink.Active() {
			v.resetBlink()
		}
		if v.dragging && (v.dragPoint.Y < 0 || v.dragPoint.Y > v.viewport.Height || !v.dragGutter && (v.dragPoint.X < v.gutter || v.dragPoint.X > v.viewport.Width)) {
			v.dragSelection(v.dragPoint)
			v.RequestPaint()
			if !v.mounted {
				continue
			}
		}
		d := v.document
		if seen[d] || d.engine == nil {
			continue
		}
		if d.driver != q.root && time.Since(d.lastFrame) > 100*time.Millisecond {
			d.driver = q.root
		}
		if d.driver != q.root {
			if d.engine.Pending() {
				_ = q.root.RequestPaint()
			}
			continue
		}
		d.lastFrame = time.Now()
		seen[d] = true
		if d.engine.Pending() && time.Now().Before(deadline) {
			until := minTime(deadline, time.Now().Add(2*time.Millisecond))
			d.engine.Advance(until)
			for view := range d.views {
				view.RequestPaint()
			}
			if d.engine.Err() != nil {
				v.report(d.engine.Err())
			}
		}
		if d.engine != nil && d.engine.Pending() {
			d.requestWork()
		}
	}
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

type settingsGroup struct {
	handle signal.Handle
	views  map[*SourceView]struct{}
}

var settingsGroups = make(map[gui.Application]*settingsGroup)

func attachSettings(v *SourceView) {
	app := gui.App
	group := settingsGroups[app]
	if group == nil {
		group = &settingsGroup{views: make(map[*SourceView]struct{})}
		settingsGroups[app] = group
		group.handle = app.Settings().ConnectChanged(func() {
			for view := range group.views {
				if view.explicitScheme == nil {
					view.updateScheme()
				}
			}
		})
	}
	group.views[v] = struct{}{}
}
func detachSettings(v *SourceView) {
	group := settingsGroups[gui.App]
	if group == nil {
		return
	}
	delete(group.views, v)
	if len(group.views) == 0 {
		group.handle.Disconnect()
		delete(settingsGroups, gui.App)
	}
}
func (v *SourceView) colorScheme() *scheme.Scheme {
	s := v.explicitScheme
	if s == nil {
		s = scheme.Light()
		if v.mounted && gui.App.Settings().ColorScheme() == gui.ColorSchemeDark {
			s = scheme.Dark()
		}
	}
	return s
}

func (v *SourceView) updateScheme() {
	s := v.colorScheme()
	if v.appliedScheme == s {
		return
	}
	v.closeCompletion()
	v.appliedScheme = s
	v.colors = s.Definition()
	v.RequestPaint()
}
