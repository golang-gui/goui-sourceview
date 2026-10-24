package sourceview

import (
	"math"
	"sort"
	"strconv"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/typography"
)

const padding float32 = 6

func (v *SourceView) invalidateLayout() {
	if v.mounted && v.anchor.valid && v.anchorRevision == v.document.model.Revision() {
		v.pendingAnchor = true
	}

	v.dropLayouts()
	v.resetHeights()
	v.contentWidth = 0
	v.RequestLayout()
	v.RequestPaint()
}

func (v *SourceView) ensureMetrics() {
	if v.context == nil || v.metricsReady {
		return
	}
	t, err := v.context.NewTextLayout("0000000000", typography.TextFormat{Font: v.font}, 1<<20, 1<<20)
	if err != nil {
		v.report(err)
		return
	}
	w, h := t.MeasureSize()
	t.Destroy()
	v.digitWidth = max(1, w/10)
	v.spaceWidth = v.digitWidth
	spaces, err := v.context.NewTextLayout(" x", typography.TextFormat{Font: v.font}, 1<<20, 1<<20)
	if err == nil {
		_, clusters := spaces.MeasureMetrics()
		for _, c := range clusters {
			if c.Start == 0 && c.Length == 1 && c.Width > 0 {
				v.spaceWidth = c.Width
				break
			}
		}
		spaces.Destroy()
	}
	v.lineHeight = max(1, h)
	v.metricsReady = true
	v.resetHeights()
}

func (v *SourceView) updateGutter() {
	old := v.gutter
	v.gutter = 0
	if v.showLineNumbers {
		v.gutter = float32(max(2, len(strconv.Itoa(v.document.model.LineCount()))))*max(v.spaceWidth, v.digitWidth) + 2*padding
	}
	v.foldColumn = 0
	if v.foldingEnabled && v.document.definition.Folding() != nil {
		v.foldColumn = max(16, min(24, v.lineHeight))
		v.gutter += v.foldColumn
	}
	if old != v.gutter && v.wrap != WrapNone {
		v.layoutEpoch++
	}
}

func (v *SourceView) Measure(c layout.Constraint) layout.Measurement {
	if !v.Visible() {
		return layout.Measurement{}
	}
	v.ensureMetrics()
	v.updateGutter()
	size := geometry.Size{Width: 640, Height: 320}
	size.Width = max(size.Width, v.MinSize().Width)
	size.Height = max(size.Height, v.MinSize().Height)
	if v.MaxSize().Width > 0 {
		size.Width = min(size.Width, v.MaxSize().Width)
	}
	if v.MaxSize().Height > 0 {
		size.Height = min(size.Height, v.MaxSize().Height)
	}
	size = c.Clamp(size)
	v.estimateWrappedHeight(max(1, size.Width-v.gutter-2*padding))
	return layout.Measured(size)
}

// Estimate only the initial scrolling extent from a bounded native sample.
// This prevents measuring an entire long paragraph once without a scrollbar
// and again after the scrollbar appears. Sample geometry is never exposed to
// editing or painting: full-context paragraph metrics replace the estimate.
func (v *SourceView) estimateWrappedHeight(width float32) {
	if v.heightEstimated || v.context == nil || v.wrap == WrapNone || v.projection != nil || v.document.model.LineCount() != 1 ||
		v.document.model.Len() <= 4096 || v.heights.Total() != v.lineHeight {
		return
	}
	end := min(512, v.document.model.Len())
	for !v.document.model.IsRuneBoundary(end) {
		end--
	}
	text, err := v.document.model.Slice(textedit.Range{End: end})
	if err != nil {
		return
	}
	format := typography.TextFormat{Font: v.font, WrapMode: v.wrap, TextColor: v.colors.Foreground, TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}
	state, epoch := v.operationState(), v.layoutEpoch
	sample, err := v.context.NewTextLayout(text, format, width, 1<<24)
	if err != nil {
		return // An optional estimate must not report a second layout failure.
	}
	w, h := sample.MeasureSize()
	sample.Destroy()
	if !state.valid(v) || epoch != v.layoutEpoch || v.Destroyed() {
		return
	}
	ratio := float32(v.document.model.Len()) / float32(end)
	height := h * ratio
	if h <= v.lineHeight {
		height = float32(math.Ceil(float64(w*ratio/width))) * v.lineHeight
	}
	v.setParagraphHeight(0, max(v.lineHeight, height))
	v.heightEstimated = true
}

func (v *SourceView) Arrange(rect geometry.Rectangle) {
	v.WidgetBase.Arrange(rect)
	// ScrollView delivers its current offset separately through LayoutVisible.
	// The retained offset can be stale after reattachment or host clamping.
	v.layoutVisible(rect.Size, v.offset, false)
}

func (v *SourceView) ContentSize() geometry.Size {
	v.ensureHeights()
	width := ceilCoordinate(float64(v.contentWidth) + float64(v.gutter) + 2*float64(padding))
	if v.wrap != WrapNone {
		width = v.viewport.Width
	}
	return geometry.Size{Width: max(v.viewport.Width, width), Height: ceilCoordinate(float64(v.heights.Total()) + 2*float64(padding))}
}

func (v *SourceView) ensureHeights() {
	if v.visibleLines.total != v.displayCount() || v.heights.Count() != v.visibleLines.count {
		v.resetHeights()
	}
}

func (v *SourceView) LayoutVisible(viewport geometry.Size, offset geometry.Point) {
	v.layoutVisible(viewport, offset, true)
}

func (v *SourceView) layoutVisible(viewport geometry.Size, offset geometry.Point, hostOffset bool) {
	defer v.arrangeCompanions()
	if v.viewport != viewport || v.offset != offset {
		v.closeCompletion()
	}
	if v.viewport.Width != viewport.Width && v.wrap != WrapNone {
		v.layoutEpoch++
		// Keep prior heights as estimates until current-width paragraphs are
		// ready. Collapsing every long paragraph to one line would hide the
		// scrollbar, create an intermediate width, then immediately show it.
		v.contentWidth = 0
	}
	v.viewport = viewport
	v.offset = offset
	v.ensureMetrics()
	v.updateGutter()
	v.ensureHeights()
	if v.document.model.Revision() != v.observed && v.mounted {
		return
	}
	// Re-query after measuring: fallback fonts and wrapping can change which
	// paragraphs cover the viewport. Bound the work to two passes per call.
	old := v.ContentSize()
	state, epoch := v.operationState(), v.layoutEpoch
	measured := false
	for range 2 {
		first, last := v.visibleRows()
		complete := true
		for row := first; row <= last; row = v.nextDisplayRow(row) {
			if v.viewportParagraph(row) == nil {
				complete = false
			}
			if !state.valid(v) || epoch != v.layoutEpoch || v.Destroyed() {
				return
			}
		}
		nextFirst, nextLast := v.visibleRows()
		if nextFirst >= first && nextLast <= last {
			measured = complete
			break
		}
	}
	if v.ContentSize() != old {
		v.RequestLayout()
	}
	if v.reveal != nil {
		v.resolveReveal(measured && hostOffset)
		if !state.valid(v) || epoch != v.layoutEpoch || v.Destroyed() {
			return
		}
	} else if v.pendingAnchor && v.projection == nil && v.anchor.valid {
		at := v.clampOffset(v.anchor.offset)
		row := v.document.model.LineAt(at)
		r, _ := v.document.model.LineRange(row)
		if p := v.viewportParagraph(row); p != nil {
			g := &p.geometry
			baseY := p.geometryY
			if p.paged != nil {
				frame := p.paged.CachedGeometryAt(at - r.Start)
				if frame == nil {
					v.requestLayoutWork()
					return
				}
				g, baseY = &frame.Geometry, frame.Y
			}
			if g == nil {
				v.requestLayoutWork()
				return
			}
			v.pendingAnchor = false
			rect := g.Caret(textedit.Position{Offset: at - r.Start})
			y := max(0, min(float32(float64(padding)+v.paragraphTop64(row)+baseY+float64(rect.Y)+float64(v.anchor.inset)), max(0, v.ContentSize().Height-viewport.Height)))
			v.offset = geometry.Point{X: v.anchor.x, Y: y}
			v.scrollSignal.Emit(geometry.Rect(v.anchor.x, y, viewport.Width, viewport.Height))
		}
	} else {
		v.captureAnchor()
	}
	v.updateIMCaret()
	v.pruneLayouts(layoutKey{})
}

func (v *SourceView) visibleRows() (int, int) {
	v.ensureHeights()
	first := v.heights.At(max(0, v.offset.Y-padding))
	last := v.heights.At(max(0, v.offset.Y+v.viewport.Height-padding))
	return v.visibleLines.line(first), v.visibleLines.line(min(v.visibleLines.count-1, max(first, last)+1))
}

// lineInterval is half-open in projected paragraph coordinates. Paragraph zero
// is always visible. It is not a replacement range in the text model.
type lineInterval struct{ start, end int }

type visibleRun struct{ start, end, index int }

type visibleLineMap struct {
	total, count int
	runs         []visibleRun
}

func (m *visibleLineMap) reset(total int, hidden []lineInterval) {
	m.total, m.count, m.runs = max(1, total), 0, m.runs[:0]
	start := 0
	appendRun := func(end int) {
		if end > start {
			m.runs = append(m.runs, visibleRun{start, end, m.count})
			m.count += end - start
		}
	}
	for _, h := range hidden { // sorted, merged by the folding state
		h.start, h.end = max(1, h.start), min(m.total, h.end)
		if h.end <= start || h.start >= m.total || h.end <= h.start {
			continue
		}
		appendRun(h.start)
		start = max(start, h.end)
	}
	appendRun(m.total)
}

func (m *visibleLineMap) line(index int) int {
	if index >= m.count {
		return m.total
	}
	index = max(0, index)
	i := sort.Search(len(m.runs), func(i int) bool { r := m.runs[i]; return r.index+r.end-r.start > index })
	if i == len(m.runs) {
		return 0
	}
	r := m.runs[i]
	return r.start + index - r.index
}

// index maps a hidden line to its preceding visible paragraph and reports false.
func (m *visibleLineMap) index(line int) (int, bool) {
	if line >= m.total {
		return m.count, true
	}
	line = max(0, line)
	i := sort.Search(len(m.runs), func(i int) bool { return m.runs[i].end > line })
	if i == len(m.runs) {
		return max(0, m.count-1), false
	}
	r := m.runs[i]
	if line < r.start {
		return max(0, r.index-1), false
	}
	return r.index + line - r.start, true
}

func (v *SourceView) resetHeights() {
	v.heightEstimated = false
	hidden := v.hiddenLines
	if v.projection != nil {
		hidden = nil
		first, last := v.projection.FirstLine(), v.projection.LastLine()
		delta := v.projection.ParagraphCount() - (last - first + 1)
		for _, h := range v.hiddenLines {
			if h.end <= first {
				hidden = append(hidden, h)
			} else if h.start > last {
				hidden = append(hidden, lineInterval{h.start + delta, h.end + delta})
			}
		}
	}
	v.visibleLines.reset(v.displayCount(), hidden)
	v.heights.Reset(v.visibleLines.count, v.lineHeight)
}

func (v *SourceView) paragraphTop(row int) float32 { return float32(v.paragraphTop64(row)) }

func (v *SourceView) paragraphTop64(row int) float64 {
	v.ensureHeights()
	i, _ := v.visibleLines.index(row)
	return v.heights.Top64(i)
}

func (v *SourceView) paragraphAt(y float32) int {
	v.ensureHeights()
	return v.visibleLines.line(v.heights.At(y))
}

func (v *SourceView) setParagraphHeight(row int, height float32) {
	i, visible := v.visibleLines.index(row)
	if visible {
		v.heights.Set(i, height)
	}
}

func (v *SourceView) nextDisplayRow(row int) int {
	i, _ := v.visibleLines.index(row)
	return v.visibleLines.line(i + 1)
}

func (v *SourceView) neighboringRow(row, delta int) int {
	v.ensureHeights()
	i, _ := v.visibleLines.index(row)
	if i+delta < 0 {
		return -1
	}
	return v.visibleLines.line(i + delta)
}

type viewportAnchor struct {
	offset   int
	inset, x float32
	valid    bool
}

func (v *SourceView) snapOffset(offset int, forward bool) int {
	row := v.document.model.LineAt(offset)
	r, _ := v.document.model.LineRange(row)
	if p := v.paragraph(row); p != nil {
		if p.paged != nil {
			if g := p.paged.GeometryAt(offset - r.Start); g != nil {
				return r.Start + g.Snap(offset-r.Start, forward)
			}
			v.requestLayoutWork()
			return v.clampOffset(offset)
		}
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
	return v.paragraphCaret(row, p, textedit.Position{Offset: caret - r.Start, Upstream: v.upstream}, false)
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
	return v.paragraphCaret(row, p, textedit.Position{Offset: caret - r.Start, Upstream: v.upstream}, true)
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
	// Test the actual local caret before converting it to coarse host coordinates.
	if v.viewport.Width <= 0 || v.viewport.Height <= 0 {
		return // Retain the request until the first usable allocation.
	}
	visible := func(start, length, viewport float32) bool {
		// ScrollView rounds offsets to whole DIP and prefers the leading edge
		// of a target larger than the viewport.
		if length > viewport {
			return start >= 0 && start < 1
		}
		return start >= 0 && float64(start)+float64(length) <= float64(viewport)
	}
	if measured && visible(rect.X-v.gutter, rect.Width+v.gutter+padding, v.viewport.Width) && visible(rect.Y, rect.Height, v.viewport.Height) {
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
	v.scrollSignal.Emit(v.revealBounds(rect))
}

// Keep the caret's viewport-local precision until the final host request.
// The rectangle conservatively encloses the desired edges despite a coarse
// float32 document origin. Width/height retain the small residual distances.
func (v *SourceView) revealBounds(rect geometry.Rectangle) geometry.Rectangle {
	x := float64(v.offset.X) + float64(rect.X) - float64(v.gutter)
	y := float64(v.offset.Y) + float64(rect.Y)
	right := float64(v.offset.X) + float64(rect.X) + float64(rect.Width) + float64(padding)
	bottom := y + float64(rect.Height)
	left, top := floorCoordinate(x), floorCoordinate(y)
	return geometry.Rect(left, top, ceilCoordinate(right-float64(left)), ceilCoordinate(bottom-float64(top)))
}

func ceilCoordinate(value float64) float32 {
	n := float32(value)
	if float64(n) < value {
		n = math.Nextafter32(n, float32(math.Inf(1)))
	}
	return n
}
func floorCoordinate(value float64) float32 {
	n := float32(value)
	if float64(n) > value {
		n = math.Nextafter32(n, float32(math.Inf(-1)))
	}
	return n
}

func (v *SourceView) hit(point geometry.Point) textedit.Position {
	pos, _, _ := v.hitCaret(point)
	return pos
}

// Hit and its scroll target must use one measured snapshot and local origin.
func (v *SourceView) hitCaret(point geometry.Point) (textedit.Position, geometry.Rectangle, bool) {
	row := v.paragraphAt(max(0, point.Y+v.offset.Y-padding))
	r := v.displayRange(row)
	pos := textedit.Position{Offset: r.Start}
	var caret geometry.Rectangle
	measured := false
	if p := v.paragraph(row); p != nil {
		origin := v.paragraphOrigin(row, p)
		pos = p.geometry.Hit(geometry.Point{X: point.X - origin.X, Y: point.Y - origin.Y})
		caret = p.geometry.Caret(pos).Translate(origin)
		measured = true
		pos.Offset += r.Start
	}
	if v.projection != nil {
		pos.Offset = v.projection.ModelOffset(pos.Offset, pos.Upstream)
	}
	return pos, caret, measured
}

// captureAnchor records committed byte coordinates, independent of pixel estimates.
func (v *SourceView) captureAnchor() {
	if v.projection != nil || !v.mounted || v.observed != v.document.model.Revision() {
		return
	}
	row := v.paragraphAt(max(0, v.offset.Y-padding))
	r := v.displayRange(row)
	p := v.viewportParagraph(row)
	if p == nil || p.paged != nil && p.paged.VisibleRange().End == 0 {
		return
	}
	origin := v.paragraphOrigin(row, p)
	y := -origin.Y
	pos := p.geometry.Hit(geometry.Point{X: v.gutter + padding - origin.X, Y: y})
	index := p.geometry.LineIndex(pos)
	edge := p.geometry.LineEdge(index, false)
	v.anchor = viewportAnchor{offset: r.Start + edge.Offset, inset: -origin.Y - p.geometry.LineMetrics(index).Y, x: v.offset.X, valid: true}
	v.anchorRevision = v.document.model.Revision()
}
