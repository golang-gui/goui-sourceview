package sourceview

import (
	"math"
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
	width := v.gutter + 2*padding + v.contentWidth
	if v.wrap != WrapNone {
		width = v.viewport.Width
	}
	return geometry.Size{Width: max(v.viewport.Width, width), Height: 2*padding + v.heights.Total()}
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
			v.pendingAnchor = false
			rect := p.geometry.Caret(textedit.Position{Offset: at - r.Start})
			y := max(0, min(padding+v.paragraphTop(row)+rect.Y+v.anchor.inset, max(0, v.ContentSize().Height-viewport.Height)))
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
