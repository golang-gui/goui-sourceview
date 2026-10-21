package sourceview

import (
	"slices"
	"time"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
)

type frameQueue struct {
	root           gui.Root
	handle         signal.Handle
	views          []*SourceView
	next           int
	analysisNext   int
	analysisTimer  *gui.Timer
	analysisHandle signal.Handle
}

var queues = make(map[gui.Root]*frameQueue)

func attachFrame(v *SourceView) {
	root := v.Root()
	q := queues[root]
	if q == nil {
		q = &frameQueue{root: root}
		queues[root] = q
		q.handle = root.ConnectFrame(q.frame)
		q.analysisTimer = gui.App.NewTimer()
		q.analysisHandle = q.analysisTimer.ConnectTimeout(func() {
			if !q.advanceAnalysis(time.Now().Add(2 * time.Millisecond)) {
				q.analysisTimer.Stop()
			}
		})
	}
	if !slices.Contains(q.views, v) {
		q.views = append(q.views, v)
	}
	v.frameLayout = true
}
func detachFrame(v *SourceView) {
	v.frameLayout, v.layoutPending = false, false
	q := queues[v.Root()]
	if q == nil {
		return
	}
	if i := slices.Index(q.views, v); i >= 0 {
		q.views = slices.Delete(q.views, i, i+1)
	}
	if len(q.views) == 0 {
		if q.analysisTimer != nil {
			q.analysisTimer.Stop()
			q.analysisHandle.Disconnect()
		}
		q.handle.Disconnect()
		delete(queues, v.Root())
	}
}
func (q *frameQueue) frame(_ time.Time) {
	deadline := time.Now().Add(4 * time.Millisecond)
	seen := make(map[*Document]bool)
	views := slices.Clone(q.views)
	if len(views) == 0 {
		return
	}
	start := q.next % len(views)
	q.next = (start + 1) % len(views)
	for i := range views {
		v := views[(start+i)%len(views)]
		if !v.mounted || v.Root() != q.root || !visible(v) {
			continue
		}
		v.layoutStep(minTime(deadline, time.Now().Add(2*time.Millisecond)))
		if !v.mounted || v.Root() != q.root || v.Destroyed() {
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
		if q.analysisTimer != nil {
			if d.analysisPending() {
				q.requestAnalysis()
			}
			continue
		}
		d.foldingStep()
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

func (v *SourceView) currentScheme() (*scheme.Scheme, error) {
	if v.explicitScheme != nil {
		return v.explicitScheme, nil
	}
	return v.styledScheme()
}
func (v *SourceView) updateScheme() {
	s, err := v.currentScheme()
	if err != nil {
		v.report(err)
		return
	}
	if v.appliedScheme == s {
		return
	}
	v.closeCompletion()
	if v.Destroyed() {
		return
	}
	// Popup teardown can synchronously replace the document, explicit scheme
	// or local stylesheet. Resolve the current appearance after that callback.
	s, err = v.currentScheme()
	if err != nil {
		v.report(err)
		return
	}
	v.appliedScheme = s
	v.colors = s.Definition()
	v.RequestPaint()
}
