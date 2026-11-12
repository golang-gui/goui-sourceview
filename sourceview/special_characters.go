package sourceview

import (
	"time"

	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
)

// ShowSpecialCharacters reports whether invisible/control-character markers
// are enabled. It is independent of whitespace and logical line-ending markers.
func (v *SourceView) ShowSpecialCharacters() bool { return v.showSpecialCharacters }

// SetShowSpecialCharacters changes only decorative, measured-cluster markers
// and their Unicode tooltip. Text, shaping, cursor stops and history are unchanged.
// The default is false. Active preedit temporarily hides the markers.
func (v *SourceView) SetShowSpecialCharacters(show bool) {
	if v.Destroyed() || show == v.showSpecialCharacters {
		return
	}
	v.showSpecialCharacters = show
	for _, p := range v.layouts {
		p.special = nil
	}
	v.requestLayoutWork()
	v.RequestPaint()
}

func (v *SourceView) specialLayout(row int, p *paragraph, create bool) *view.SpecialLayout {
	if !v.showSpecialCharacters || v.projection != nil || p == nil {
		return nil
	}
	origin := v.paragraphOrigin(row, p)
	clip := geometry.Rect(v.gutter-origin.X, -origin.Y, max(0, v.textRight()-v.gutter), v.viewport.Height)
	if v.lineCollapsed(row) {
		clip = clip.Intersect(v.linePreviewRect(row, p).Translate(geometry.Point{X: -origin.X, Y: -origin.Y}))
	}
	version := uint64(0)
	if p.Paged != nil {
		version = p.Paged.Version()
	}
	s := p.special
	if s != nil && s.Text == p.text && s.TextStart == p.textStart && s.Version == version && s.Revision == v.document.Revision() && s.Clip == clip {
		return s
	}
	if !create || p.Paged != nil && p.text == "" {
		return nil
	}
	s = view.NewSpecialLayout(p.text, p.textStart, p.Geometry, clip)
	s.Version, s.Revision = version, v.document.Revision()
	p.special = s
	return s
}

func (v *SourceView) specialCharacterStep(deadline time.Time) {
	if !v.showSpecialCharacters || v.projection != nil || !v.mounted || !visible(v) {
		return
	}
	first, last := v.visibleRows()
	for row := first; row <= last; row = v.nextDisplayRow(row) {
		p := v.cachedParagraph(row)
		if p == nil {
			continue
		}
		s := v.specialLayout(row, p, true)
		if s == nil {
			v.requestLayoutWork()
			continue
		}
		if s.Pending() {
			s.Advance(deadline)
			v.RequestPaint()
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return
		}
	}
}

func (v *SourceView) paintSpecialCharacters(draw gui.Painter, row int, p *paragraph, origin geometry.Point) {
	if !v.showSpecialCharacters || v.projection != nil {
		return
	}
	s := v.specialLayout(row, p, !v.frameLayout)
	if s == nil {
		v.requestLayoutWork()
		return
	}
	if !v.frameLayout {
		for s.Pending() {
			s.Advance(time.Time{})
		}
	} else if s.Pending() {
		v.RequestPaint()
	}
	c := v.partColor("special-character", view.RGB(0xb55d00))
	s.Visit(func(m view.SpecialMarker) { view.PaintSpecialCharacter(draw, m, origin, c) })
}

func (v *SourceView) specialCharacterTooltip() string {
	if !v.showSpecialCharacters || v.projection != nil || !v.mounted {
		return ""
	}
	first, last := v.visibleRows()
	for row := first; row <= last; row = v.nextDisplayRow(row) {
		p := v.cachedParagraph(row)
		s := v.specialLayout(row, p, false)
		if s == nil {
			continue
		}
		origin := v.paragraphOrigin(row, p)
		if text := s.Tooltip(geometry.Point{X: v.pointerPoint.X - origin.X, Y: v.pointerPoint.Y - origin.Y}); text != "" {
			return text
		}
	}
	return ""
}
