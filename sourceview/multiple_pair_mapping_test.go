package sourceview

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
)

func TestPairMappingSequentialRuns(t *testing.T) {
	rng := rand.New(rand.NewSource(179))
	for round := 0; round < 300; round++ {
		v := NewSourceView()
		for i := 0; i < 30; i++ {
			at := rng.Intn(200)
			v.pairs = append(v.pairs, autoPair{open: at, close: at + 5, left: "((", right: "))"})
		}
		want := slices.Clone(v.pairs)
		var edits []TextEdit
		for i := 0; i < 20; i++ {
			a := rng.Intn(210)
			b := a + rng.Intn(4)
			edits = append(edits, TextEdit{Range: TextRange{Start: a, End: b}, Text: []string{"", "x", "abc"}[rng.Intn(3)]})
		}
		for _, e := range edits {
			kept := want[:0]
			for _, p := range want {
				if editor.TouchesPair(e.Range, p.open, len(p.left)) || editor.TouchesPair(e.Range, p.close, len(p.right)) {
					continue
				}
				p.open = textcore.MapOffset(p.open, e)
				p.close = textcore.MapOffset(p.close, e)
				kept = append(kept, p)
			}
			want = kept
		}
		v.mapPairs(TextChange{Edits: edits})
		if !slices.Equal(v.pairs, want) {
			t.Fatal(round, v.pairs, want)
		}
	}
}
