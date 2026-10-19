package sourceview

import (
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func extensionLanguage(t *testing.T, rules language.EditingRules) *language.Language {
	t.Helper()
	l, err := language.New(language.Definition{Info: language.Info{ID: "custom", Name: "Custom"}, Editing: rules})
	if err != nil {
		t.Fatal(err)
	}
	return l
}
func TestCustomWordAndPairs(t *testing.T) {
	v := NewSourceView()
	v.document.SetLanguageDefinition(extensionLanguage(t, language.EditingRules{Word: func(r rune) bool { return language.IdentifierRune(r) || r == '-' }, AutoClosingPairs: []language.Pair{{Open: "<<", Close: ">>"}}, Brackets: []language.Pair{{Open: "<<", Close: ">>"}}}))
	observe(t, v)
	m := v.document.model
	m.SetText("hello-world rest")
	if r := editor.WordRange(m, 4, v.document.definition.IsWord); r != (textedit.Range{Start: 0, End: 11}) {
		t.Fatal(r)
	}
	v.SetSelection(textedit.Selection{})
	v.MoveWord(true, false)
	if v.Selection().Caret != 11 {
		t.Fatal(v.Selection())
	}
	m.SetText("<")
	v.SetSelection(textedit.Selection{Anchor: 1, Caret: 1})
	if handled, err := v.pairText(v.Selection().Range(), "<"); !handled || err != nil || m.Text() != "<<>>" || v.Selection().Caret != 2 {
		t.Fatal(handled, err, m.Text(), v.Selection())
	}
	if ranges := v.matchingBrackets(); len(ranges) != 2 || ranges[0] != (textedit.Range{Start: 2, End: 4}) || ranges[1] != (textedit.Range{Start: 0, End: 2}) {
		t.Fatal(ranges)
	}
	if ok, err := v.deletePair(); !ok || err != nil || m.Text() != "" {
		t.Fatal(ok, err, m.Text())
	}
	if !v.Undo() || m.Text() != "<<>>" || v.Selection().Caret != 2 {
		t.Fatal("multi-pair undo")
	}
	// Ownership must be lost if an external edit splits a generated delimiter.
	m.SetText("")
	v.SetSelection(textedit.Selection{})
	if ok, err := v.pairText(textedit.Range{}, "<<"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if err := m.Replace(textedit.Range{Start: 3, End: 3}, "x"); err != nil {
		t.Fatal(err)
	}
	if len(v.pairs) != 0 {
		t.Fatal("retained split delimiter")
	}
}
func TestCustomNewlineAndReentry(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	rules := language.EditingRules{Newline: func(c language.NewlineContext) (language.NewlineEdit, bool) {
		if c.Before != "- item" || c.After != "" {
			t.Fatalf("newline context: %+v", c)
		}
		return language.NewlineEdit{Text: "\n- ", Caret: 3}, true
	}}
	v.document.SetLanguageDefinition(extensionLanguage(t, rules))
	v.document.model.SetText("- item")
	v.SetSelection(textedit.Selection{Anchor: 6, Caret: 6})
	if err := v.newline(); err != nil || v.document.model.Text() != "- item\n- " {
		t.Fatal(err, v.document.model.Text())
	}
	if !v.Undo() || v.document.model.Text() != "- item" {
		t.Fatal("newline undo")
	}
	rules.Newline = func(language.NewlineContext) (language.NewlineEdit, bool) {
		v.document.SetLanguageDefinition(nil)
		return language.NewlineEdit{Text: "bad", Caret: 3}, true
	}
	v.document.SetLanguageDefinition(extensionLanguage(t, rules))
	if err := v.newline(); err != ErrOperationChanged || v.document.model.Text() != "- item" {
		t.Fatal("reentry", err)
	}
}
func TestPublicEditsValidateBeforeMutation(t *testing.T) {
	v := NewSourceView()
	m := v.document.model
	m.SetText("é tail")
	rev := m.Revision()
	if err := v.ApplyEdits(v.document, rev, []textedit.Edit{{Range: textedit.Range{Start: 1, End: 2}, Text: "bad"}}, textedit.Selection{}); err == nil || m.Text() != "é tail" || m.Revision() != rev {
		t.Fatal("invalid UTF-8 edit mutated document", err)
	}
	if err := v.ApplyEdits(v.document, rev-1, nil, textedit.Selection{}); err != ErrOperationChanged {
		t.Fatal(err)
	}
	if err := v.ApplyEdits(v.document, rev, []textedit.Edit{{Range: textedit.Range{Start: 0, End: 2}, Text: "中"}}, textedit.Selection{Caret: 1}); err == nil || m.Revision() != rev {
		t.Fatal("invalid final selection accepted", err)
	}
}
func TestLanguageCommentsAndHistory(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	v.document.SetLanguageDefinition(extensionLanguage(t, language.EditingRules{Comments: language.Comments{Line: "#", Block: language.Pair{Open: "<!--", Close: "-->"}}}))
	m := v.document.model
	m.SetText("  a\n  b\nlast")
	s := textedit.Selection{Anchor: 0, Caret: 8}
	v.SetSelection(s)
	if err := v.ToggleLineComment(); err != nil || m.Text() != "  # a\n  # b\nlast" {
		t.Fatal(err, m.Text())
	}
	if err := v.ToggleLineComment(); err != nil || m.Text() != "  a\n  b\nlast" {
		t.Fatal(err, m.Text())
	}
	if !v.Undo() || m.Text() != "  # a\n  # b\nlast" {
		t.Fatal("comment undo")
	}
	m.SetText("text")
	v.SetSelection(textedit.Selection{Anchor: 4, Caret: 0})
	if err := v.ToggleBlockComment(); err != nil || m.Text() != "<!--text-->" || v.Selection() != (textedit.Selection{Anchor: 8, Caret: 4}) {
		t.Fatal(err, m.Text(), v.Selection())
	}
}

func TestPublicEditsRejectAnotherDocumentAtSameRevision(t *testing.T) {
	v := NewSourceView()
	first := v.document
	other := NewDocument("")
	if first.ID() == other.ID() || first.model.Revision() != other.model.Revision() {
		t.Fatal("invalid fixture")
	}
	v.SetDocument(other)
	if err := v.ApplyEdits(first, first.model.Revision(), []textedit.Edit{{Text: "wrong file"}}, textedit.Selection{Caret: 10}); err != ErrOperationChanged || other.model.Text() != "" {
		t.Fatal("applied to different document", err)
	}
}

type roleTokenizer struct {
	kind  language.Kind
	class language.Class
}

func (roleTokenizer) InitialState() language.State { return language.EmptyState{} }
func (s roleTokenizer) Scan(in language.Input, state language.State) (language.Result, error) {
	result := language.Result{Consumed: len(in.Text), State: state}
	for i, c := range in.Text {
		if c == '[' || c == ']' {
			result.Tokens = append(result.Tokens, language.Token{Start: in.Offset + i, End: in.Offset + i + 1, Kind: s.kind, Class: s.class})
		}
	}
	return result, nil
}
func TestMatchingUsesLexicalRoleIndependentlyOfStyle(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("[x]")
	definition := language.Definition{Info: language.Info{ID: "roles", Name: "Roles"}, Tokenizer: roleTokenizer{kind: "custom-brace", class: language.ClassDelimiter}, Editing: language.EditingRules{Brackets: []language.Pair{{Open: "[", Close: "]"}}}}
	l, err := language.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	v.document.SetLanguageDefinition(l)
	observe(t, v)
	v.document.engine.Advance(time.Time{})
	if ranges := v.matchingBrackets(); len(ranges) != 2 {
		t.Fatal("custom style lost structural role", ranges)
	}
	definition.Tokenizer = roleTokenizer{kind: language.KindDelimiter}
	l, err = language.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	v.document.SetLanguageDefinition(l)
	v.document.engine.Advance(time.Time{})
	if ranges := v.matchingBrackets(); len(ranges) != 0 {
		t.Fatal("inferred structural role from color key", ranges)
	}
}
