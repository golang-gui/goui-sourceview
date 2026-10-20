package sourceview

import (
	"image/color"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

func TestPaintRangeCullsWrappedLines(t *testing.T) {
	const count = 1000
	lines := make([]typography.TextLine, count)
	clusters := make([]typography.TextCluster, count)
	for i := range lines {
		lines[i] = typography.TextLine{Start: i, Length: 1, Y: float32(i * 20), Height: 20}
		clusters[i] = typography.TextCluster{Start: i, Length: 1, Width: 10, LineIndex: i}
	}
	item := &paragraph{geometry: textedit.NewGeometry(lines, clusters, count, 20)}
	v := NewSourceView()
	v.viewport = geometry.Size{Width: 200, Height: 38}
	p := &bracketPainter{}
	// A huge selection includes the whole logical paragraph, but only two
	// visual lines intersect the viewport. Keep full rectangle heights for
	// underlines and let the Painter perform the final pixel clipping.
	v.paintRange(p, item, textedit.Range{End: count}, geometry.Point{Y: -10001},
		textedit.Range{End: count}, color.NRGBA{A: 255})
	if len(p.fills) != 2 {
		t.Fatalf("painted %d selection rectangles, want 2", len(p.fills))
	}
	for i, fill := range p.fills {
		want := geometry.Rect(0, float32(i*20-1), 10, 20)
		if fill.rect != want {
			t.Fatalf("rectangle %d = %v, want %v", i, fill.rect, want)
		}
	}
}
