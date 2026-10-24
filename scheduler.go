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
		until := minTime(deadline, time.Now().Add(2*time.Millisecond))
		v.layoutStep(until)
		if v.mounted && !v.Destroyed() {
			v.whitespaceStep(until)
		}
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

func (q *frameQueue) requestAnalysis() {
	if q.analysisTimer.Active() {
		return
	}
	if err := q.analysisTimer.Start(8 * time.Millisecond); err != nil {
		// A stopped application cannot start another timer. Keep the existing
		// frame path available for a host whose timer is unavailable.
		q.analysisHandle.Disconnect()
		q.analysisTimer = nil
		_ = q.root.RequestPaint()
	}
}

func (d *Document) analysisPending() bool {
	if d.engine != nil && d.engine.Pending() {
		return true
	}
	if gui.App != nil && !d.foldReady && d.definition.Folding() != nil {
		for v := range d.views {
			if v.foldingEnabled {
				return true
			}
		}
	}
	return false
}

// Pure analysis uses ordinary GUI event-loop turns, without forcing a window
// repaint for every offscreen chunk. Native viewport layout stays on frames.
func (q *frameQueue) advanceAnalysis(deadline time.Time) bool {
	views := slices.Clone(q.views)
	if len(views) == 0 {
		return false
	}
	start := q.analysisNext % len(views)
	q.analysisNext = (start + 1) % len(views)
	seen := make(map[*Document]bool)
	pending := false
	for i := range views {
		v := views[(start+i)%len(views)]
		if !v.mounted || v.Destroyed() || v.Root() != q.root || !visible(v) {
			continue
		}
		d := v.document
		if seen[d] || d.engine == nil || d.driver != q.root {
			continue
		}
		seen[d] = true
		d.lastFrame = time.Now()
		d.foldingStep()
		e := d.engine
		ready := e.Ready()
		if e.Pending() && (deadline.IsZero() || time.Now().Before(deadline)) {
			e.Advance(deadline)
			for view := range d.views {
				if view.mounted && view.document == d && visible(view) && (ready != e.Ready() || view.analysisPaintNeeded()) {
					view.RequestPaint()
				}
			}
			if err := e.Err(); err != nil {
				v.report(err)
			}
		}
		pending = pending || d.analysisPending()
	}
	return pending
}

func (v *SourceView) analysisPaintNeeded() bool {
	if v.viewport.Height <= 0 {
		return false
	}
	first, last := v.visibleRows()
	for row := first; row <= last; row = v.nextDisplayRow(row) {
		p := v.cachedParagraph(row)
		if p == nil {
			return true
		}
		if tokenRow, ok := v.highlightRow(row); ok {
			_, version, valid := v.document.engine.Tokens(tokenRow)
			if !p.styled || p.style.tokenVersion != version || p.style.tokenValid != valid {
				return true
			}
		}
	}
	return false
}

func (v *SourceView) requestLayoutWork() {
	if v.layoutPending {
		return
	}
	v.layoutPending = true
	if root := v.Root(); root != nil {
		_ = root.RequestPaint()
	}
}

// Prepare the latest viewport on the GUI thread between event dispatches. A
// native paragraph call is still indivisible: the deadline bounds the batch,
// not a single paragraph. No independent substring shaping is used here.
func (v *SourceView) layoutStep(deadline time.Time) {
	if !v.layoutPending || !v.mounted || v.context == nil || !visible(v) || v.viewport.Height <= 0 {
		return
	}
	v.layoutPending = false
	state, epoch := v.operationState(), v.layoutEpoch
	valid := func() bool {
		return state.valid(v) && v.mounted && visible(v) && !v.Destroyed() && epoch == v.layoutEpoch
	}
	prepare := func(row int) bool {
		if _, failed := v.layoutFailures[v.paragraphKey(row)]; failed {
			return valid()
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			v.requestLayoutWork()
			return false
		}
		v.paragraph(row)
		return valid()
	}
	defer func() {
		if valid() {
			v.RequestLayout()
			v.RequestPaint()
		}
	}()
	// First refine the text position retained across a width change. Other
	// viewport work uses the latest demand rather than a queue of old widths.
	if v.pendingAnchor && v.anchor.valid && v.projection == nil && v.anchorRevision == v.document.model.Revision() {
		if !prepare(v.document.model.LineAt(v.clampOffset(v.anchor.offset))) {
			return
		}
	}
	if request := v.reveal; request != nil && request.valid(v) {
		row := v.displayLine(v.displayCaret())
		if !prepare(row) {
			return
		}
		first := v.paragraphAt(max(0, v.paragraphTop(row)-v.viewport.Height))
		last := min(v.displayCount()-1, v.neighboringRow(row, 1))
		for row := first; row <= last; row = v.nextDisplayRow(row) {
			if !prepare(row) {
				return
			}
		}
	}
	for range 2 {
		first, last := v.visibleRows()
		for row := first; row <= last; row = v.nextDisplayRow(row) {
			if !prepare(row) {
				return
			}
		}
		nextFirst, nextLast := v.visibleRows()
		if nextFirst >= first && nextLast <= last {
			return
		}
	}
	v.requestLayoutWork()
}
