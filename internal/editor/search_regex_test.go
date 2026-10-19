package editor

import (
	"github.com/golang-gui/goui/gui/textedit"
	"reflect"
	"regexp"
	"testing"
)

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
