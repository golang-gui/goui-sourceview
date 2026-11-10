package folding

import (
	"math/rand"
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
)

func TestVisibleLineMapAgainstEnumeration(t *testing.T) {
	rng := rand.New(rand.NewSource(7331))
	for range 2000 {
		n := 1 + rng.Intn(250)
		var hidden []Interval
		var visible []int
		for row := 0; row < n; {
			if row != 0 && rng.Intn(4) == 0 {
				end := min(n, row+1+rng.Intn(20))
				hidden = append(hidden, Interval{row, end})
				row = end
			} else {
				visible = append(visible, row)
				row++
			}
		}
		var m VisibleLines
		m.Reset(n, hidden)
		if m.Count() != len(visible) || m.Line(m.Count()) != n {
			t.Fatalf("counts: %+v want %v", m, visible)
		}
		for i, line := range visible {
			if got := m.Line(i); got != line {
				t.Fatalf("line(%d)=%d want %d", i, got, line)
			}
		}
		previous := 0
		for row := range n {
			wantVisible := previous+1 < len(visible) && visible[previous+1] == row || visible[previous] == row
			if previous+1 < len(visible) && visible[previous+1] == row {
				previous++
			}
			index, ok := m.Index(row)
			if index != previous || ok != wantVisible {
				t.Fatalf("index(%d)=%d,%v want %d,%v", row, index, ok, previous, wantVisible)
			}
		}
	}
}

func TestFoldValidation(t *testing.T) {
	for _, bad := range [][]language.FoldRange{{{StartLine: -1, EndLine: 2}}, {{StartLine: 0, EndLine: 4}}, {{StartLine: 2, EndLine: 2}}, {{StartLine: 0, EndLine: 2}, {StartLine: 1, EndLine: 3}}} {
		if _, err := Normalize(bad, 4); err == nil {
			t.Fatal("accepted invalid ranges", bad)
		}
	}
	got, err := Normalize([]language.FoldRange{{StartLine: 1, EndLine: 2}, {StartLine: 0, EndLine: 3}, {StartLine: 0, EndLine: 2}}, 4)
	if err != nil || len(got) != 2 || got[0] != (language.FoldRange{StartLine: 0, EndLine: 3}) {
		t.Fatal(got, err)
	}
}
