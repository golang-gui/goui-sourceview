package sourceview

import (
	"slices"
	"time"

	"github.com/golang-gui/goui/gui"
)

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
