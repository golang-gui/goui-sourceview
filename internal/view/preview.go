package view

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
)

// PreviewPainter confines paragraph page clips to a borrowed preview rectangle.
// SetClipRect replaces the caller's explicit clip in the public Painter contract.
// The paragraph's original geometry and native resources remain unchanged.
type PreviewPainter struct {
	gui.Painter
	Bounds geometry.Rectangle
}

func (p *PreviewPainter) SetClipRect(rect geometry.Rectangle) {
	p.Painter.SetClipRect(rect.Intersect(p.Bounds))
}
