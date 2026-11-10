package view

import (
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
)

type previewClipPainter struct {
	gui.Painter
	clip geometry.Rectangle
}

func (p *previewClipPainter) SetClipRect(r geometry.Rectangle) { p.clip = r }

func TestPreviewClipsEveryPage(t *testing.T) {
	base := &previewClipPainter{}
	p := &PreviewPainter{Painter: base, Bounds: geometry.Rect(10, 20, 100, 18)}
	for _, tc := range []struct{ page, want geometry.Rectangle }{
		{geometry.Rect(0, 0, 200, 200), geometry.Rect(10, 20, 100, 18)},
		{geometry.Rect(30, 25, 200, 100), geometry.Rect(30, 25, 80, 13)},
		{geometry.Rect(15, 22, 10, 8), geometry.Rect(15, 22, 10, 8)},
	} {
		p.SetClipRect(tc.page)
		if base.clip != tc.want {
			t.Fatalf("page %v escaped preview: %v, want %v", tc.page, base.clip, tc.want)
		}
	}
}
