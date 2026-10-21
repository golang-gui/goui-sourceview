package sourceview

import "time"

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
