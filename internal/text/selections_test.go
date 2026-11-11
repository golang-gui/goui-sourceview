package text

import (
	"math/rand"
	"reflect"
	"slices"
	"testing"
)

func TestNormalizeSelectionSets(t *testing.T) {
	cases := []struct{ in, want []SelectionState }{
		{[]SelectionState{{Selection: Selection{5, 2}, Upstream: true}, {Selection: Selection{0, 4}}, {Selection: Selection{5, 5}}}, []SelectionState{{Selection: Selection{5, 0}, Upstream: true}}},
		{[]SelectionState{{Selection: Selection{4, 6}}, {Selection: Selection{0, 2}}, {Selection: Selection{2, 4}}}, []SelectionState{{Selection: Selection{4, 6}}, {Selection: Selection{0, 2}}, {Selection: Selection{2, 4}}}},
		{[]SelectionState{{Selection: Selection{3, 3}}, {Selection: Selection{3, 3}}, {Selection: Selection{9, 9}}}, []SelectionState{{Selection: Selection{3, 3}}, {Selection: Selection{9, 9}}}},
	}
	for _, c := range cases {
		before := slices.Clone(c.in)
		got := NormalizeSelections(c.in)
		if !reflect.DeepEqual(got, c.want) || !reflect.DeepEqual(c.in, before) {
			t.Fatal(got, c.want, c.in)
		}
	}
}
func TestMapOffsetsAgainstSequentialReference(t *testing.T) {
	rng := rand.New(rand.NewSource(901))
	for run := 0; run < 100; run++ {
		length := 1000
		offsets := make([]int, 500)
		for i := range offsets {
			offsets[i] = rng.Intn(length + 1)
		}
		want := slices.Clone(offsets)
		var edits []Edit
		for range 100 {
			a := rng.Intn(length + 1)
			b := a + rng.Intn(length-a+1)
			n := rng.Intn(50)
			e := Edit{Range: Range{a, b}, Text: string(make([]byte, n))}
			edits = append(edits, e)
			for i, p := range want {
				if p >= a {
					if p <= b {
						want[i] = a + n
					} else {
						want[i] += n - (b - a)
					}
				}
			}
			length += n - (b - a)
		}
		if got := MapOffsets(offsets, edits); !slices.Equal(got, want) {
			t.Fatalf("run %d: endpoint mapping differs", run)
		}
	}
}
func TestBatchHistoryCompleteSelections(t *testing.T) {
	m := NewModel("ab\ncd")
	before := []SelectionState{{Selection: Selection{4, 3}, Upstream: true}, {Selection: Selection{0, 1}}}
	after := []SelectionState{{Selection: Selection{4, 4}}, {Selection: Selection{1, 1}, Upstream: true}}
	edits := []Edit{{Range: Range{0, 1}, Text: "中"}, {Range: Range{3, 4}, Text: "!"}}
	after[0].Selection = Selection{6, 6}
	after[1].Selection = Selection{3, 3}
	originalBefore, originalAfter := slices.Clone(before), slices.Clone(after)
	if err := m.ApplyBatch(edits, BatchOptions{BeforeSelections: before, AfterSelections: after}); err != nil {
		t.Fatal(err)
	}
	before[0].Caret = 0
	after[0].Caret = 0
	result := m.UndoWithSelection()
	if !result.Applied || m.Text() != "ab\ncd" || !slices.Equal(result.Selections, originalBefore) {
		t.Fatal(result, m.Text())
	}
	result.Selections[0].Caret = 0
	result = m.RedoWithSelection()
	if !result.Applied || m.Text() != "中b\n!d" || !slices.Equal(result.Selections, originalAfter) {
		t.Fatal(result, m.Text())
	}
	rev := m.Revision()
	bad := []SelectionState{{Selection: Selection{1, 1}}}
	if err := m.ApplyBatch([]Edit{{Range: Range{}, Text: "中"}}, BatchOptions{AfterSelections: bad}); err == nil || m.Revision() != rev {
		t.Fatal("invalid result committed", err)
	}
}
