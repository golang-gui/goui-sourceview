package view

import (
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/platform/typography"
)

// ParagraphResources owns a cached paragraph's native resources and the sole
// measured geometry used by paint, caret, hit testing and preview consumers.
// The root editor controls cache membership, revisions, eviction and pinning.
type ParagraphResources struct {
	Layout, Number       typography.TextLayout
	Paged                *ParagraphLayout
	Geometry             textcore.Geometry
	GeometryX, GeometryY float64
}

func (p *ParagraphResources) GeometryAt(offset int) *textcore.Geometry {
	if p.Paged != nil {
		if g := p.Paged.GeometryAt(offset); g != nil {
			return &g.Geometry
		}
	}
	return &p.Geometry
}

// PreviewGeometry borrows the first measured line without preparing missing
// pages. Paint and snapshots must not trigger native layout to show a preview.
func (p *ParagraphResources) PreviewGeometry(lineHeight float32) (*textcore.Geometry, float32) {
	if p == nil {
		return nil, lineHeight
	}
	g := &p.Geometry
	if p.Paged != nil {
		frame := p.Paged.CachedGeometryAt(0)
		if frame == nil {
			return nil, lineHeight
		}
		g = &frame.Geometry
	}
	if g.LineCount() == 0 {
		return nil, lineHeight
	}
	return g, max(1, g.LineMetrics(0).Height)
}

// Destroy preserves native release order; the cache owner detaches entries
// before calling it, so reentrant callbacks cannot observe an evicted entry.
func (p *ParagraphResources) Destroy() {
	if p.Layout != nil {
		p.Layout.Destroy()
	}
	if p.Paged != nil {
		p.Paged.Destroy()
	}
	if p.Number != nil {
		p.Number.Destroy()
	}
}
