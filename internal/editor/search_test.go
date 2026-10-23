package editor

import (
	"math/rand"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/golang-gui/goui/gui/textedit"
)

func TestSearchUnicodeWholeWordAndChunks(t *testing.T) {
	m := textedit.NewModel("K K k_keep kk\né e\u0301")
	s := Compile(Query{Text: "k", IgnoreCase: true, WholeWord: true})
	a, ok := s.Find(m, 0, false)
	if !ok || a != (textedit.Range{Start: 0, End: 1}) {
		t.Fatal(a, ok)
	}
	a, ok = s.Find(m, 1, false)
	if !ok || a != (textedit.Range{Start: 2, End: 5}) {
		t.Fatal(a, ok)
	}
	a, ok = s.Find(m, 5, false)
	if !ok || a.Start != 0 {
		t.Fatal(a, ok)
	}
	p, err := s.ReplaceAll(m, textedit.Selection{}, "Z")
	if err != nil || len(p.Edits) != 2 {
		t.Fatal(p, err)
	}
	p.Apply(m, nil)
	if m.Text() != "Z Z k_keep kk\né e\u0301" {
		t.Fatal(m.Text())
	}
	m.SetText(strings.Repeat("x", 16383) + "é\nabc")
	s = Compile(Query{Text: "é\nab"})
	a, ok = s.Find(m, 0, false)
	if !ok || a.Start != 16383 || a.End != 16388 {
		t.Fatal(a, ok)
	}
}

func TestReplaceAllNonOverlappingAtomic(t *testing.T) {
	m := textedit.NewModel("aaaa")
	s := Compile(Query{Text: "aa"})
	p, err := s.ReplaceAll(m, textedit.Selection{Anchor: 4, Caret: 0}, "aaa")
	if err != nil || len(p.Edits) != 2 {
		t.Fatal(err, p)
	}
	p.Apply(m, nil)
	if m.Text() != "aaaaaa" {
		t.Fatal(m.Text())
	}
	m.Undo()
	if m.Text() != "aaaa" || m.CanUndo() {
		t.Fatal("not atomic")
	}
	if _, err := Compile(Query{Text: "a"}).ReplaceAll(textedit.NewModel(strings.Repeat("a", 100001)), textedit.Selection{}, "x"); err != ErrOperationLimit {
		t.Fatal(err)
	}
}

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

func TestRegexCanonicalNavigationCountAndCaptures(t *testing.T) {
	text := "a=12\n中=34\nnone"
	m := textedit.NewModel(text)
	query := `(?m)^(?P<key>\w+|中)=(\d+)$`
	s := Compile(Query{Text: query, Regex: true})
	if s.Err() != nil {
		t.Fatal(s.Err())
	}
	want := regexp.MustCompile(query).FindAllStringIndex(text, -1)
	at, total, after := 0, 0, 0
	for at < m.Len() {
		var n int
		at, n, after = s.CountChunk(m, at, 4, after)
		total += n
	}
	if total != len(want) {
		t.Fatal(total, want)
	}
	for _, w := range want {
		r, ok := s.Find(m, w[0], false)
		if !ok || r != (textedit.Range{Start: w[0], End: w[1]}) || !s.Matches(m, r) {
			t.Fatal(r, w)
		}
	}
	plan, err := s.ReplaceAll(m, textedit.Selection{}, `${key}:$2 $$`)
	if err != nil {
		t.Fatal(err)
	}
	if err = plan.Apply(m, nil); err != nil {
		t.Fatal(err)
	}
	if m.Text() != "a:12 $\n中:34 $\nnone" {
		t.Fatal(m.Text())
	}
	if !m.Undo() || m.Text() != text {
		t.Fatal("not atomic")
	}
}

func TestRegexZeroWidthUsesGoCanonicalSequence(t *testing.T) {
	for _, query := range []string{`(?m)^`, `a*`, `\b`, `$`} {
		m := textedit.NewModel("a\n中")
		s := Compile(Query{Text: query, Regex: true})
		reference := regexp.MustCompile(query).FindAllStringIndex(m.Text(), -1)
		got := s.InRange(m, textedit.Range{End: m.Len()}, 100)
		var want []textedit.Range
		for _, r := range reference {
			want = append(want, textedit.Range{Start: r[0], End: r[1]})
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal(query, got, want)
		}
		plan, err := s.ReplaceAll(m, textedit.Selection{}, "!")
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Edits) != len(want) {
			t.Fatal(query, plan.Edits, want)
		}
	}
}

func TestSearchScopeAndRevisionInvalidation(t *testing.T) {
	m := textedit.NewModel("a=1 a=2 a=3")
	s := Compile(Query{Text: `a=(\d)`, Regex: true})
	scope := textedit.Range{Start: 4, End: 7}
	s.SetRange(&scope)
	scope.Start = 0
	r, ok := s.Find(m, 0, false)
	if !ok || r != (textedit.Range{Start: 4, End: 7}) {
		t.Fatal(r, ok)
	}
	p, err := s.ReplaceAll(m, textedit.Selection{}, "<$1>")
	if err != nil || len(p.Edits) != 1 || p.Edits[0].Text != "<2>" {
		t.Fatal(p, err)
	}
	m.SetText("a=8 a=9 a=0")
	p, err = s.ReplaceAll(m, textedit.Selection{}, "<$1>")
	if err != nil || p.Edits[0].Text != "<9>" {
		t.Fatal("stale capture", p, err)
	}
	bad := Compile(Query{Text: "[", Regex: true})
	if bad.Err() == nil {
		t.Fatal("syntax accepted")
	}
	if _, err := bad.ReplaceAll(m, textedit.Selection{}, "x"); err == nil {
		t.Fatal("invalid expression mutated")
	}
}

func TestMultilinePaintUsesCanonicalNonOverlap(t *testing.T) {
	m := textedit.NewModel("a\na\na\na")
	s := Compile(Query{Text: "a\na"})
	want := []textedit.Range{{Start: 0, End: 3}, {Start: 4, End: 7}}
	if got := s.InRange(m, textedit.Range{End: m.Len()}, 20); !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	if got := s.InRange(m, textedit.Range{Start: 2, End: 3}, 20); !reflect.DeepEqual(got, want[:1]) {
		t.Fatal(got)
	}
}

func TestCachedSearchIndexesBothDirectionsAndVisibleSpans(t *testing.T) {
	m := textedit.NewModel("ab ab ab ")
	s := Compile(Query{Text: `ab|$`, Regex: true})
	for _, tc := range []struct {
		start int
		back  bool
		want  textedit.Range
	}{{4, true, textedit.Range{Start: 0, End: 2}}, {6, true, textedit.Range{Start: 3, End: 5}}, {3, false, textedit.Range{Start: 3, End: 5}}, {7, false, textedit.Range{Start: 9, End: 9}}, {10, false, textedit.Range{Start: 0, End: 2}}} {
		r, ok := s.Find(m, tc.start, tc.back)
		if !ok || r != tc.want {
			t.Fatal(tc, r)
		}
	}
	if got := s.InRange(m, textedit.Range{Start: 4, End: 7}, 10); !reflect.DeepEqual(got, []textedit.Range{{Start: 3, End: 5}, {Start: 6, End: 8}}) {
		t.Fatal(got)
	}
}
