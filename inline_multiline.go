package sourceview

import (
	"image/color"
	"strings"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

// The source paragraph retains its native text/geometry. A view-local extent
// after it reserves space for the remaining ghost lines. Model row identities
// and all following paragraphs are unchanged; only their measured tops move.
type multilineInlineLayout struct {
	head, tail                 typography.TextLayout
	headGeometry, tailGeometry textedit.Geometry
	headText, tailText         string
	bodyWidth, headWidth       float32
	wrap                       WrapMode
	tabWidth                   float32
	extra, width               float32
	tailHeight, headExtra      float32
}

func (m *multilineInlineLayout) destroy() {
	if m.head != nil {
		m.head.Destroy()
		m.head = nil
	}
	if m.tail != nil {
		m.tail.Destroy()
		m.tail = nil
	}
}

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
	g, origin := &p.geometry, v.paragraphOrigin(s.row, p)
	if p.paged != nil {
		frame := p.paged.CachedGeometryAt(pos.Offset)
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
	if m := s.multi; m != nil && m.bodyWidth == bodyWidth && m.headWidth == headWidth && m.wrap == v.wrap && m.tabWidth == tabWidth && s.font == v.font && s.color == c {
		v.setParagraphHeight(s.row, p.height)
		return nil
	}
	m := &multilineInlineLayout{bodyWidth: bodyWidth, headWidth: headWidth, wrap: v.wrap, tabWidth: tabWidth}
	font, epoch := v.font, v.layoutEpoch
	valid := func() bool {
		return v.inlineSuggestion == s && v.inlineSuggestionCurrent() && !v.Destroyed() &&
			v.layoutEpoch == epoch && v.font == font && v.wrap == m.wrap && float32(v.indent.TabWidth)*v.spaceWidth == tabWidth &&
			v.partColor("inline-suggestion", v.colors.GutterForeground) == c && v.textRight()-v.gutter-2*padding == bodyWidth
	}
	makeLayout := func(text string, width float32) (typography.TextLayout, textedit.Geometry, error) {
		l, err := v.context.NewTextLayout(text, typography.TextFormat{Font: font, WrapMode: m.wrap, TextColor: c, TabWidth: tabWidth}, max(1, width), 1<<20)
		if err != nil {
			return nil, textedit.Geometry{}, err
		}
		if !valid() {
			l.Destroy()
			return nil, textedit.Geometry{}, nil
		}
		lines, clusters := l.MeasureMetrics()
		if !valid() {
			l.Destroy()
			return nil, textedit.Geometry{}, nil
		}
		return l, textedit.NewGeometry(lines, clusters, len(text), v.lineHeight), nil
	}
	var err error
	overflow := ""
	if first != "" && headWidth >= 1 {
		m.headText = first
		m.head, m.headGeometry, err = makeLayout(first, headWidth)
		if err != nil {
			m.destroy()
			if valid() {
				v.ClearInlineSuggestion()
			}
			return err
		}
		if !valid() {
			m.destroy()
			return nil
		}
		if m.headGeometry.LineCount() > 1 {
			at := m.headGeometry.LineMetrics(1).Start
			overflow = first[at:]
		}
	} else if first != "" {
		overflow = first
	}
	m.tailText = rest
	if overflow != "" {
		m.tailText = overflow + "\n" + rest
	}
	m.tail, m.tailGeometry, err = makeLayout(m.tailText, bodyWidth)
	if err != nil {
		m.destroy()
		if valid() {
			v.ClearInlineSuggestion()
		}
		return err
	}
	if !valid() {
		m.destroy()
		return nil
	}
	tailWidth, tailHeight := m.tail.MeasureSize()
	extraHead := float32(0)
	if m.head != nil {
		h := m.headGeometry.LineMetrics(0)
		extraHead = max(0, (line.Baseline-line.Y)-(h.Baseline-h.Y)+h.Height-line.Height)
	}
	m.tailHeight, m.headExtra = tailHeight, extraHead
	m.extra = extraHead + tailHeight
	m.width = bodyWidth
	if v.wrap == WrapNone {
		m.width = tailWidth
		if m.head != nil {
			h := m.headGeometry.LineMetrics(0)
			m.width = max(m.width, ceilCoordinate(p.geometryX+float64(caret.X)+1+float64(h.Width)))
		}
	}
	if !valid() {
		m.destroy()
		return nil
	}
	old := s.multi
	s.multi = m
	s.font, s.color = font, c
	s.baseHeight = p.height
	if old != nil {
		old.destroy()
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
		if i, ok := v.visibleLines.index(s.row); ok && i < v.heights.Count() {
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
	return s.multi.extra
}
func (v *SourceView) inlineExtraWidth() float32 {
	s := v.inlineSuggestion
	if s != nil && s.multi != nil && v.inlineSuggestionCurrent() && v.wrap == WrapNone {
		return s.multi.width
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
	if !ok || m.bodyWidth != v.textRight()-v.gutter-2*padding || m.wrap != v.wrap {
		return nil
	}
	viewport := geometry.Rect(v.gutter, 0, max(0, v.textRight()-v.gutter), v.viewport.Height)
	var segments []inlineSegment
	if m.head != nil {
		h := m.headGeometry.LineMetrics(0)
		at := geometry.Point{X: origin.X + caret.X + 1 - h.X, Y: origin.Y + line.Baseline - h.Baseline}
		top := max(origin.Y+line.Y, at.Y+h.Y)
		r := geometry.Rect(origin.X+caret.X+1, top, h.Width, max(0, at.Y+h.Y+h.Height-top)).Intersect(viewport)
		if r.Width > 0 && r.Height > 0 {
			segments = append(segments, inlineSegment{m.head, at, r, m.headText})
		}
	}
	// Tail coordinates start at column zero; every newline/soft wrap remains a
	// view extent within the anchor paragraph and therefore has no line number.
	extraHead, height := m.headExtra, m.tailHeight
	at := v.geometryOrigin(s.row, 0, float64(p.height+extraHead))
	width := m.bodyWidth
	if v.wrap == WrapNone {
		width = m.width
	}
	r := geometry.Rect(at.X, at.Y, width, height).Intersect(viewport)
	if r.Width > 0 && r.Height > 0 {
		segments = append(segments, inlineSegment{m.tail, at, r, m.tailText})
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
	if point.Y < top || point.Y >= top+s.multi.extra {
		return textedit.Position{}, geometry.Rectangle{}, false
	}
	r, _ := v.document.model.LineRange(s.row)
	caret, ok := v.paragraphCaret(s.row, p, textedit.Position{Offset: r.End - r.Start, Upstream: true}, true)
	return textedit.Position{Offset: s.request.Position, Upstream: true}, caret, ok
}
