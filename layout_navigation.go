package sourceview

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

type viewportAnchor struct {
	offset   int
	inset, x float32
	valid    bool
}

func (v *SourceView) snapOffset(offset int, forward bool) int {
	row := v.document.model.LineAt(offset)
	r, _ := v.document.model.LineRange(row)
	if p := v.paragraph(row); p != nil {
		return r.Start + p.geometry.Snap(offset-r.Start, forward)
	}
	return v.clampOffset(offset)
}

func (v *SourceView) caretRect() (geometry.Rectangle, bool) {
	caret := v.displayCaret()
	row := v.displayLine(caret)
	r := v.displayRange(row)
	p := v.paragraph(row)
	if p == nil {
		return geometry.Rectangle{}, false
	}
	rect := p.geometry.Caret(textedit.Position{Offset: caret - r.Start, Upstream: v.upstream})
	return rect.Translate(geometry.Point{X: v.gutter + padding - v.offset.X, Y: padding + v.paragraphTop(row) - v.offset.Y}), true
}

// Paint and IME updates must not shape an offscreen caret during scrolling.
// Explicit menu, completion and navigation requests use caretRect instead.
func (v *SourceView) cachedCaretRect() (geometry.Rectangle, bool) {
	caret := v.displayCaret()
	row := v.displayLine(caret)
	p := v.cachedParagraph(row)
	if p == nil {
		return geometry.Rectangle{}, false
	}
	r := v.displayRange(row)
	rect := p.geometry.Caret(textedit.Position{Offset: caret - r.Start, Upstream: v.upstream})
	return rect.Translate(geometry.Point{X: v.gutter + padding - v.offset.X, Y: padding + v.paragraphTop(row) - v.offset.Y}), true
}

func (v *SourceView) updateIMCaret() {
	if r, ok := v.cachedCaretRect(); ok {
		v.im.SetCaretRect(r)
	}
}

func (v *SourceView) RevealSelection() {
	if v.Destroyed() {
		return
	}
	request := v.operationState()
	v.reveal = &request
	v.pendingAnchor = false
	v.anchor.valid = false
	v.resolveReveal(false)
	v.RequestLayout()
}

// Keep the logical request until the host has delivered an offset at which the
// caret is visible with measured paragraphs. A one-shot pixel rectangle becomes
// stale when previously unseen rows acquire their actual heights.
func (v *SourceView) resolveReveal(measured bool) {
	request := v.reveal
	if request == nil {
		return
	}
	if !request.valid(v) || v.Destroyed() {
		v.reveal = nil
		return
	}
	row := v.displayLine(v.displayCaret())
	if v.viewportParagraph(row) == nil {
		return
	}
	rect, ok := v.cachedCaretRect()
	if !ok || v.reveal != request || !request.valid(v) || v.Destroyed() {
		return
	}
	// ScrollView receives content coordinates. Include the fixed gutter in the
	// requested width so a caret at its leading edge remains visible.
	rect.X += v.offset.X - v.gutter
	rect.Y += v.offset.Y
	rect.Width += v.gutter + padding
	if v.viewport.Width <= 0 || v.viewport.Height <= 0 {
		return // Retain the request until the first usable allocation.
	}
	visible := func(start, length, offset, viewport float32) bool {
		// ScrollView rounds offsets to whole DIP and prefers the leading edge
		// of a target larger than the viewport.
		if length > viewport {
			return offset <= start && start-offset < 1
		}
		return start >= offset && start+length <= offset+viewport
	}
	if measured && visible(rect.X, rect.Width, v.offset.X, v.viewport.Width) && visible(rect.Y, rect.Height, v.offset.Y, v.viewport.Height) {
		v.reveal = nil
		v.captureAnchor()
		return
	}
	// Validate the destination before scrolling. Measuring just the caret's
	// paragraph leaves the preceding visible rows estimated, so their later
	// growth can push the caret below the viewport again. Actual paragraph
	// heights are at least lineHeight; an estimated viewport before the target
	// therefore covers the rows needed when revealing its bottom edge.
	first := v.paragraphAt(max(0, v.paragraphTop(row)-v.viewport.Height))
	epoch := v.layoutEpoch
	for at, end := first, v.neighboringRow(row, 1); at <= min(end, v.displayCount()-1); at = v.nextDisplayRow(at) {
		p := v.viewportParagraph(at)
		if v.reveal != request || !request.valid(v) || epoch != v.layoutEpoch || v.Destroyed() || p == nil {
			return
		}
	}
	rect, ok = v.cachedCaretRect()
	if !ok || v.reveal != request || !request.valid(v) || v.Destroyed() {
		return
	}
	rect.X += v.offset.X - v.gutter
	rect.Y += v.offset.Y
	rect.Width += v.gutter + padding
	v.scrollSignal.Emit(rect)
}

func (v *SourceView) hit(point geometry.Point) textedit.Position {
	row := v.paragraphAt(max(0, point.Y+v.offset.Y-padding))
	r := v.displayRange(row)
	pos := textedit.Position{Offset: r.Start}
	if p := v.paragraph(row); p != nil {
		pos = p.geometry.Hit(geometry.Point{X: point.X - v.gutter - padding + v.offset.X, Y: point.Y + v.offset.Y - padding - v.paragraphTop(row)})
		pos.Offset += r.Start
	}
	if v.projection != nil {
		pos.Offset = v.projection.ModelOffset(pos.Offset, pos.Upstream)
	}
	return pos
}

// captureAnchor records committed byte coordinates, independent of pixel estimates.
func (v *SourceView) captureAnchor() {
	if v.projection != nil || !v.mounted || v.observed != v.document.model.Revision() {
		return
	}
	row := v.paragraphAt(max(0, v.offset.Y-padding))
	r := v.displayRange(row)
	p := v.viewportParagraph(row)
	if p == nil {
		return
	}
	y := max(0, v.offset.Y-padding-v.paragraphTop(row))
	pos := p.geometry.Hit(geometry.Point{Y: y})
	index := p.geometry.LineIndex(pos)
	edge := p.geometry.LineEdge(index, false)
	v.anchor = viewportAnchor{offset: r.Start + edge.Offset, inset: v.offset.Y - padding - v.paragraphTop(row) - p.geometry.LineMetrics(index).Y, x: v.offset.X, valid: true}
	v.anchorRevision = v.document.model.Revision()
}
