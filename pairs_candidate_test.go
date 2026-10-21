package sourceview

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestNonBracketCaretDoesNotWaitForSearchWindowLexing(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("plain text\n" + strings.Repeat("<item>value</item>\n", 10000))
	if err := v.Document().SetLanguage(language.XML); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	if v.Document().engine.Ready() {
		t.Fatal("fixture unexpectedly finished indexing")
	}
	if len(v.matchingBrackets()) != 0 || !v.brackets.valid {
		t.Fatal("ordinary caret demanded unresolved search-window tokens")
	}
	if v.Document().engine.ScannedLines() != 0 {
		t.Fatal("matching ordinary text forced synchronous analysis")
	}
}

func TestBracketCandidateHandlesSequenceEdgesAndUTF8(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("字««x»»z")
	pairs := []language.Pair{{Open: "««", Close: "»»"}}
	for _, tc := range []struct {
		at   int
		want bool
	}{{0, false}, {3, true}, {5, true}, {7, true}, {8, true}, {10, true}, {12, true}, {13, false}} {
		if got := len(v.bracketCandidates(tc.at, pairs)) > 0; got != tc.want {
			t.Fatal("candidate mismatch", tc, got)
		}
	}
	// Preserve actual longest-sequence matching inside a custom delimiter.
	l, err := language.New(language.Definition{Info: language.Info{ID: "custom", Name: "Custom"}, Editing: language.EditingRules{Brackets: pairs}})
	if err != nil {
		t.Fatal(err)
	}
	v.Document().SetLanguageDefinition(l)
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 5, Caret: 5})
	r := v.matchingBrackets()
	if len(r) != 2 || r[0] != (textedit.Range{Start: 3, End: 7}) || r[1] != (textedit.Range{Start: 8, End: 12}) {
		t.Fatal("candidate check changed custom matching", r)
	}
}

type slowMarkupTokenizer struct{ language.Tokenizer }

func (s slowMarkupTokenizer) Scan(in language.Input, state language.State) (language.Result, error) {
	// Make a single scan exceed the fixture's soft deadline. This establishes
	// an exact partially analyzed state without depending on scanner speed.
	time.Sleep(2 * time.Millisecond)
	return s.Tokenizer.Scan(in, state)
}

func TestNonStructuralCandidateOnlyNeedsItsOwnLexicalLine(t *testing.T) {
	v := NewSourceView()
	xml, _ := language.Builtin(language.XML)
	d := xml.Definition()
	d.Tokenizer = slowMarkupTokenizer{d.Tokenizer}
	l, err := language.New(d)
	if err != nil {
		t.Fatal(err)
	}
	v.Document().SetLanguageDefinition(l)
	v.Document().Model().SetText("<?xml version=\"1.0\"?>\n" + strings.Repeat("<item>value</item>\n", 100))
	observe(t, v)
	e := v.Document().engine
	for {
		e.Advance(time.Now().Add(time.Millisecond))
		if _, _, ready := e.Tokens(0); ready {
			break
		}
	}
	if _, _, ready := e.Tokens(1); ready {
		t.Fatal("fixture analyzed more than the declaration")
	}
	if len(v.matchingBrackets()) != 0 || !v.brackets.valid {
		t.Fatal("non-structural '<' demanded tokens from unrelated search-window lines")
	}
}
