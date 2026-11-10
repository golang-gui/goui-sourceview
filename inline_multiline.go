package sourceview

import (
	"image/color"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

// Cached source geometry, also when only the virtual continuation is on screen.
func (v *SourceView) inlineAnchor(s *inlineSuggestionState) (*paragraph, geometry.Point, typography.TextLine, geometry.Rectangle, bool) {
	if !v.inlineSuggestionCurrent() || v.lineCollapsed(s.row) {
		return nil, geometry.Point{}, typography.TextLine{}, geometry.Rectangle{}, false
	}
	p := v.cachedParagraph(s.row)
	if p == nil {
		return nil, geometry.Point{}, typography.TextLine{}, geometry.Rectangle{}, false
	}
	r, _ := v.document.model.LineRange(s.row)
	pos := textedit.Position{Offset: s.request.Position - r.Start, Upstream: true}
	g, origin := &p.Geometry, v.paragraphOrigin(s.row, p)
	if p.Paged != nil {
		frame := p.Paged.CachedGeometryAt(pos.Offset)
		if frame == nil {
			return nil, geometry.Point{}, typography.TextLine{}, geometry.Rectangle{}, false
		}
		g, origin = &frame.Geometry, v.geometryOrigin(s.row, frame.X, frame.Y)
	}
	if g.LineCount() == 0 {
		return nil, geometry.Point{}, typography.TextLine{}, geometry.Rectangle{}, false
	}
	line := g.LineMetrics(g.LineIndex(pos))
	return p, origin, line, g.Caret(pos), true
}

func (v *SourceView) prepareMultilineInline(s *inlineSuggestionState, c color.NRGBA) error {
	p, _, line, caret, ok := v.inlineAnchor(s)
	if !ok || v.viewport.Width <= 0 {
		return nil
	}
	first, rest, _ := strings.Cut(s.request.Text, "\n")
	if first != "" && caret.X < line.X+line.Width-.5 {
		return nil
	}
	bodyWidth := v.textRight() - v.gutter - 2*padding
	if bodyWidth <= 0 {
		return nil
	}
	// Width is measured in paragraph coordinates, independent of scrolling.
	headWidth := max(0, bodyWidth-caret.X-1)
	if v.wrap == WrapNone {
		headWidth = 1 << 20
	}
	tabWidth := float32(v.indent.TabWidth) * v.spaceWidth
	if m := s.multi; m != nil && m.BodyWidth == bodyWidth && m.HeadWidth == headWidth && m.Wrap == v.wrap && m.TabWidth == tabWidth && s.font == v.font && s.color == c {
		v.setParagraphHeight(s.row, p.height)
		return nil
	}
	m := &view.InlineLayout{BodyWidth: bodyWidth, HeadWidth: headWidth, Wrap: v.wrap, TabWidth: tabWidth}
	font, epoch := v.font, v.layoutEpoch
	valid := func() bool {
		return v.inlineSuggestion == s && v.inlineSuggestionCurrent() && !v.Destroyed() &&
			v.layoutEpoch == epoch && v.font == font && v.wrap == m.Wrap && float32(v.indent.TabWidth)*v.spaceWidth == tabWidth &&
			v.partColor("inline-suggestion", v.colors.GutterForeground) == c && v.textRight()-v.gutter-2*padding == bodyWidth
	}
	ready, err := m.Build(func() typography.Context { return v.context }, first, rest, view.InlineMetrics{
		Font: font, Color: c, LineHeight: func() float32 { return v.lineHeight }, SourceLine: line, GeometryX: func() float64 { return p.GeometryX }, CaretX: caret.X,
	}, valid)
	if err != nil {
		if valid() {
			v.ClearInlineSuggestion()
		}
		return err
	}
	if !ready {
		return nil
	}

	old := s.multi
	s.multi = m
	s.font, s.color = font, c
	s.baseHeight = p.height
	if old != nil {
		old.Destroy()
	}
	if !valid() {
		return nil
	}
	v.setParagraphHeight(s.row, p.height)
	v.beginViewportInitialization()
	v.RequestLayout()
	v.RequestPaint()
	return nil
}

func (v *SourceView) restoreInlineExtent(s *inlineSuggestionState) {
	if s.multi == nil {
		return
	}
	// changed() invokes this before splicing the old row/height map, even when
	// Model already has the new revision. Restore the old index before mapping.
	if s.state.document == v.document && s.state.mount == v.mountEpoch && s.baseHeight > 0 {
		if i, ok := v.visibleLines.Index(s.row); ok && i < v.heights.Count() {
			v.heights.Set(i, s.baseHeight)
		}
		if v.anchor.valid {
			v.pendingAnchor = true
			if v.document.model.LineAt(v.clampOffset(v.anchor.offset)) == s.row {
				v.anchor.inset = 0
			}
		}
	}
	v.beginViewportInitialization()
	v.RequestLayout()
}

func (v *SourceView) inlineExtraHeight(row int) float32 {
	s := v.inlineSuggestion
	if s == nil || s.multi == nil || row != s.row || !v.inlineSuggestionCurrent() || v.lineCollapsed(row) {
		return 0
	}
	return s.multi.Extra
}
func (v *SourceView) inlineExtraWidth() float32 {
	s := v.inlineSuggestion
	if s != nil && s.multi != nil && v.inlineSuggestionCurrent() && v.wrap == WrapNone {
		return s.multi.Width
	}
	return 0
}

type inlineSegment struct {
	layout typography.TextLayout
	origin geometry.Point
	bounds geometry.Rectangle
	text   string
}

func (v *SourceView) multilineSegments(s *inlineSuggestionState) []inlineSegment {
	if !v.inlineSuggestionCurrent() || s.multi == nil || s.font != v.font || s.color != v.partColor("inline-suggestion", v.colors.GutterForeground) {
		return nil
	}
	m := s.multi
	p, origin, line, caret, ok := v.inlineAnchor(s)
	if !ok || m.BodyWidth != v.textRight()-v.gutter-2*padding || m.Wrap != v.wrap {
		return nil
	}
	viewport := geometry.Rect(v.gutter, 0, max(0, v.textRight()-v.gutter), v.viewport.Height)
	var segments []inlineSegment
	if m.Head != nil {
		h := m.HeadGeometry.LineMetrics(0)
		at := geometry.Point{X: origin.X + caret.X + 1 - h.X, Y: origin.Y + line.Baseline - h.Baseline}
		top := max(origin.Y+line.Y, at.Y+h.Y)
		r := geometry.Rect(origin.X+caret.X+1, top, h.Width, max(0, at.Y+h.Y+h.Height-top)).Intersect(viewport)
		if r.Width > 0 && r.Height > 0 {
			segments = append(segments, inlineSegment{m.Head, at, r, m.HeadText})
		}
	}
	// Tail coordinates start at column zero; every newline/soft wrap remains a
	// view extent within the anchor paragraph and therefore has no line number.
	extraHead, height := m.HeadExtra, m.TailHeight
	at := v.geometryOrigin(s.row, 0, float64(p.height+extraHead))
	width := m.BodyWidth
	if v.wrap == WrapNone {
		width = m.Width
	}
	r := geometry.Rect(at.X, at.Y, width, height).Intersect(viewport)
	if r.Width > 0 && r.Height > 0 {
		segments = append(segments, inlineSegment{m.Tail, at, r, m.TailText})
	}
	return segments
}
func (v *SourceView) multilineInlinePreview(s *inlineSuggestionState) (inlinePreview, bool) {
	var bounds geometry.Rectangle
	visible := false
	for _, part := range v.multilineSegments(s) {
		if strings.TrimSpace(part.text) == "" {
			continue
		}
		if !visible {
			bounds = part.bounds
			visible = true
		} else {
			x, y := min(bounds.X, part.bounds.X), min(bounds.Y, part.bounds.Y)
			right, bottom := max(bounds.X+bounds.Width, part.bounds.X+part.bounds.Width), max(bounds.Y+bounds.Height, part.bounds.Y+part.bounds.Height)
			bounds = geometry.Rect(x, y, right-x, bottom-y)
		}
	}
	return inlinePreview{bounds: bounds, textClip: bounds}, visible
}
func (v *SourceView) paintMultilineInline(p gui.Painter, s *inlineSuggestionState) {
	for _, part := range v.multilineSegments(s) {
		p.Save()
		p.SetClipRect(part.bounds)
		p.DrawTextLayout(part.origin, part.layout)
		p.Restore()
	}
}

// Virtual-only space maps to the real insertion point. Following source rows
// still use the same measured row index; selection APIs never see ghost bytes.
func (v *SourceView) inlineVirtualHit(point geometry.Point) (textedit.Position, geometry.Rectangle, bool) {
	s := v.inlineSuggestion
	if s == nil || s.multi == nil || !v.inlineSuggestionCurrent() {
		return textedit.Position{}, geometry.Rectangle{}, false
	}
	p := v.cachedParagraph(s.row)
	if p == nil {
		return textedit.Position{}, geometry.Rectangle{}, false
	}
	top := padding + v.paragraphTop(s.row) + p.height - v.offset.Y
	if point.Y < top || point.Y >= top+s.multi.Extra {
		return textedit.Position{}, geometry.Rectangle{}, false
	}
	r, _ := v.document.model.LineRange(s.row)
	caret, ok := v.paragraphCaret(s.row, p, textedit.Position{Offset: r.End - r.Start, Upstream: true}, true)
	return textedit.Position{Offset: s.request.Position, Upstream: true}, caret, ok
}
