package editor

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/golang-gui/goui/gui/textedit"
)

func TestChunkCountAgainstLiteralStringReference(t *testing.T) {
	rng := rand.New(rand.NewSource(20261017))
	alphabet := []string{"a", "b", "中", "_", "\n", " "}
	for sample := 0; sample < 300; sample++ {
		var text, query strings.Builder
		for n := rng.Intn(90); n > 0; n-- {
			text.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		for n := 1 + rng.Intn(5); n > 0; n-- {
			query.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		m := textedit.NewModel(text.String())
		s := Compile(Query{Text: query.String()})
		for _, budget := range []int{4, 7, 17} {
			at, total, after := 0, 0, 0
			for at < m.Len() {
				var n int
				at, n, after = s.CountChunk(m, at, budget, after)
				total += n
			}
			if want := strings.Count(text.String(), query.String()); total != want {
				t.Fatalf("sample %d %q / %q budget %d: %d want %d", sample, text.String(), query.String(), budget, total, want)
			}
		}
	}
}

func TestChunkedSearchCounts(t *testing.T) {
	for _, tt := range []struct {
		text  string
		query Query
		want  int
	}{
		{"aaaaa", Query{Text: "aa"}, 2},
		{"a 中中\na 中中", Query{Text: "中\na"}, 1},
		{"a𐐀中\na𐐨中", Query{Text: "𐐀中", IgnoreCase: true}, 2},
		{"cat catapult cat\ncat", Query{Text: "cat", WholeWord: true}, 3},
		{"a-a a- -a -", Query{Text: "-", WholeWord: true}, 1},
		{"unchanged", Query{}, 0},
		{strings.Repeat("x", 65535) + "中x\n中x", Query{Text: "中x"}, 2},
		{strings.Repeat("a", 9000), Query{Text: strings.Repeat("a", 4300)}, 2},
	} {
		m := textedit.NewModel(tt.text)
		s := Compile(tt.query)
		for _, budget := range []int{4, 7, 4096, 65536} {
			at, total, matchedEnd := 0, 0, 0
			for at < m.Len() {
				next, n, after := s.CountChunk(m, at, budget, matchedEnd)
				matchedEnd = after
				if next <= at || !m.IsRuneBoundary(next) {
					t.Fatal("count cursor did not advance", next)
				}
				at = next
				total += n
			}
			if total != tt.want {
				t.Fatalf("%q budget %d: got %d want %d", tt.query.Text, budget, total, tt.want)
			}
		}
	}
}

func TestFindUsesCanonicalNonOverlappingMatches(t *testing.T) {
	m := textedit.NewModel("aaaaa")
	s := Compile(Query{Text: "aa"})
	for _, tt := range []struct {
		at   int
		back bool
		want textedit.Range
	}{
		{0, false, textedit.Range{Start: 0, End: 2}}, {1, false, textedit.Range{Start: 2, End: 4}},
		{2, false, textedit.Range{Start: 2, End: 4}}, {4, false, textedit.Range{Start: 0, End: 2}},
		{4, true, textedit.Range{Start: 2, End: 4}}, {3, true, textedit.Range{Start: 0, End: 2}},
	} {
		if got, ok := s.Find(m, tt.at, tt.back); !ok || got != tt.want {
			t.Fatal(tt, got, ok)
		}
	}
	if got := s.InRange(m, textedit.Range{Start: 1, End: 5}, 10); len(got) != 2 || got[0] != (textedit.Range{Start: 0, End: 2}) || got[1] != (textedit.Range{Start: 2, End: 4}) {
		t.Fatal("single-line visible matches", got)
	}
}
