package editor

import (
	"strings"
	"testing"

	"github.com/golang-gui/goui/gui/textedit"
)

func TestIndentHistory(t *testing.T) {
	m := textedit.NewModel("a\nb")
	s := textedit.Selection{Anchor: 3, Caret: 0}
	p, err := Indent(m, s, IndentOptions{TabWidth: 4, InsertSpaces: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Apply(m, &textedit.EditSource{}); err != nil {
		t.Fatal(err)
	}
	if m.Text() != "    a\n    b" || p.After != (textedit.Selection{Anchor: 11, Caret: 4}) {
		t.Fatal(m.Text(), p.After)
	}
	r := m.UndoWithSelection()
	if m.Text() != "a\nb" || *r.Selection != s {
		t.Fatal(m.Text(), r)
	}
	r = m.RedoWithSelection()
	if *r.Selection != p.After {
		t.Fatal(r)
	}
	m.SetText("a\nb")
	p, _ = Indent(m, textedit.Selection{Anchor: 0, Caret: 2}, IndentOptions{InsertSpaces: true}, false)
	p.Apply(m, nil)
	if m.Text() != "    a\nb" {
		t.Fatal(m.Text())
	}
}
func TestOutdentPreservesContent(t *testing.T) {
	m := textedit.NewModel("  a\n\tb\n c")
	p, err := Indent(m, textedit.Selection{Anchor: 0, Caret: m.Len()}, IndentOptions{TabWidth: 4}, true)
	if err != nil {
		t.Fatal(err)
	}
	p.Apply(m, nil)
	if m.Text() != "a\nb\nc" {
		t.Fatal(m.Text())
	}
}
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
