package sourceview

import (
	"math/rand"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
)

func TestVisibleLineMapAgainstEnumeration(t *testing.T) {
	rng := rand.New(rand.NewSource(7331))
	for range 2000 {
		n := 1 + rng.Intn(250)
		var hidden []lineInterval
		var visible []int
		for row := 0; row < n; {
			if row != 0 && rng.Intn(4) == 0 {
				end := min(n, row+1+rng.Intn(20))
				hidden = append(hidden, lineInterval{row, end})
				row = end
			} else {
				visible = append(visible, row)
				row++
			}
		}
		var m visibleLineMap
		m.reset(n, hidden)
		if m.count != len(visible) || m.line(m.count) != n {
			t.Fatalf("counts: %+v want %v", m, visible)
		}
		for i, line := range visible {
			if got := m.line(i); got != line {
				t.Fatalf("line(%d)=%d want %d", i, got, line)
			}
		}
		previous := 0
		for row := range n {
			wantVisible := previous+1 < len(visible) && visible[previous+1] == row || visible[previous] == row
			if previous+1 < len(visible) && visible[previous+1] == row {
				previous++
			}
			index, ok := m.index(row)
			if index != previous || ok != wantVisible {
				t.Fatalf("index(%d)=%d,%v want %d,%v", row, index, ok, previous, wantVisible)
			}
		}
	}
}

func TestHiddenParagraphsUseVisibleHeightsAndSourceCoordinates(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("head\nhidden\nhidden\nhidden\ntail")
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	tail := v.paragraph(4)
	v.hiddenLines = []lineInterval{{1, 4}}
	v.resetHeights()
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	if v.heights.Count() != 2 || v.heights.Total() != 40 || v.paragraph(2) != nil || v.paragraph(4) != tail {
		t.Fatalf("visibility affected paragraph identity or heights: %d %g", v.heights.Count(), v.heights.Total())
	}
	pos := v.hit(geometry.Point{X: v.gutter + padding, Y: padding + 21})
	r, _ := v.Document().Model().LineRange(4)
	if pos.Offset != r.Start || v.paragraphTop(4) != 20 || v.neighboringRow(0, 1) != 4 || v.neighboringRow(4, -1) != 0 {
		t.Fatalf("source/display mapping: hit=%v range=%v", pos, r)
	}
}
