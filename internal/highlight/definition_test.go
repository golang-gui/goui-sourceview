package highlight

import (
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

type customScanner struct{ kind language.Kind }

func (customScanner) InitialState() language.State { return language.EmptyState{} }
func (s customScanner) Scan(in language.Input, state language.State) (language.Result, error) {
	r := language.Result{Consumed: len(in.Text), State: state}
	if len(in.Text) > 0 {
		r.Tokens = []language.Token{{Start: in.Offset, End: in.Offset + len(in.Text), Kind: s.kind, Class: language.ClassDelimiter}}
	}
	return r, nil
}
func TestSameIDDefinitionInvalidation(t *testing.T) {
	makeLanguage := func(kind language.Kind) *language.Language {
		l, err := language.New(language.Definition{Info: language.Info{ID: "custom", Name: "Custom"}, Tokenizer: customScanner{kind}})
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	m := textedit.NewModel("abc\nxyz")
	e := NewWithLanguage(m, makeLanguage("first"))
	drain(t, e)
	before := e.Line(0).ID
	e.SetDefinition(makeLanguage("second"))
	if _, _, valid := e.Tokens(0); valid {
		t.Fatal("old tokens exposed")
	}
	drain(t, e)
	tokens, _, ok := e.Tokens(0)
	if !ok || len(tokens) != 1 || tokens[0].Kind != "second" || tokens[0].Class != language.ClassDelimiter || e.Line(0).ID != before {
		t.Fatalf("new definition: %v", tokens)
	}
}

type badScanner struct{ customScanner }

func (s badScanner) Scan(in language.Input, state language.State) (language.Result, error) {
	return language.Result{Consumed: len(in.Text) + 1, State: state}, nil
}
func TestInvalidProviderStopsAnalysis(t *testing.T) {
	l, _ := language.New(language.Definition{Info: language.Info{ID: "bad", Name: "Bad"}, Tokenizer: badScanner{}})
	e := NewWithLanguage(textedit.NewModel("text"), l)
	for e.Pending() {
		e.Advance(time.Now().Add(time.Second))
	}
	if e.Err() == nil {
		t.Fatal("invalid scan accepted")
	}
	if _, _, ok := e.Tokens(0); ok {
		t.Fatal("invalid scan published")
	}
}

func TestNoTokenizerEditsOnlyUpdateLineMetadata(t *testing.T) {
	l, _ := language.New(language.Definition{Info: language.Info{ID: "uncolored", Name: "No colors"}})
	for _, definition := range []*language.Language{l, func() *language.Language { plain, _ := language.Builtin(language.Plain); return plain }()} {
		m := textedit.NewModel("one\ntwo")
		e := NewWithLanguage(m, definition)
		drain(t, e)
		handle := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
		if err := m.Replace(textedit.Range{Start: 0, End: 3}, "a\nb"); err != nil {
			t.Fatal(err)
		}
		if e.Pending() {
			t.Fatal("scheduled nonexistent tokenizer")
		}
		e.Advance(time.Time{})
		checkIndex(t, e, m)
		if tokens, _, ok := e.Tokens(0); !ok || len(tokens) != 0 {
			t.Fatal(tokens, ok)
		}
		handle.Disconnect()
	}
}
