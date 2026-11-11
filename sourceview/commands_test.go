package sourceview

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestEditingCommandsAndSelectionHistory(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	v.document.model.SetText("  foo.bar\nlast")
	v.SetSelection(textedit.Selection{Anchor: 5, Caret: 5})
	if err := v.InsertLine(true); err != nil {
		t.Fatal(err)
	}
	if v.document.model.Text() != "  foo.bar\n  \nlast" || v.Selection() != (textedit.Selection{Anchor: 12, Caret: 12}) {
		t.Fatal(v.document.model.Text(), v.Selection())
	}
	if !v.Undo() || v.Selection() != (textedit.Selection{Anchor: 5, Caret: 5}) {
		t.Fatal("insert line history")
	}
	v.MoveWord(true, false)
	if v.Selection() != (textedit.Selection{Anchor: 9, Caret: 9}) {
		t.Fatal("forward fast word", v.Selection())
	}
	v.MoveWord(false, true)
	if v.Selection() != (textedit.Selection{Anchor: 9, Caret: 6}) {
		t.Fatal("extend word", v.Selection())
	}
	if err := v.DeleteWord(false); err != nil {
		t.Fatal(err)
	}
	if v.document.model.Text() != "  foo.\nlast" || !v.Undo() || v.Selection() != (textedit.Selection{Anchor: 9, Caret: 6}) {
		t.Fatal("selected word delete or history")
	}
	v.SelectLine()
	if v.Selection() != (textedit.Selection{Caret: 10}) {
		t.Fatal("line selection", v.Selection())
	}
	v.SelectLine()
	if v.Selection() != (textedit.Selection{Caret: 14}) {
		t.Fatal("line expansion", v.Selection())
	}
}

func TestNewEditingCommandsRespectModesAndReentry(t *testing.T) {
	for _, run := range []func(*SourceView) error{func(v *SourceView) error { return v.InsertLine(false) }, func(v *SourceView) error { return v.DeleteWord(false) }} {
		v := NewSourceView()
		observe(t, v)
		v.document.model.SetText("foo")
		v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
		v.SetReadOnly(true)
		if err := run(v); !errors.Is(err, ErrReadOnly) {
			t.Fatal(err)
		}
		v.SetReadOnly(false)
		v.SetEnabled(false)
		if err := run(v); !errors.Is(err, ErrDisabled) {
			t.Fatal(err)
		}
		v.SetEnabled(true)
		old := v.Document()
		replacement := NewDocument("keep")
		v.projection = textedit.NewProjection(old.model, v.Selection())
		v.im = &resetContext{reset: func() { v.SetDocument(replacement) }}
		if err := run(v); !errors.Is(err, ErrOperationChanged) || old.model.Text() != "foo" || replacement.model.Text() != "keep" {
			t.Fatal("reentrant edit", err)
		}
	}
}

var viewLineCommands = []struct {
	name string
	run  func(*SourceView) error
}{
	{"move", func(v *SourceView) error { return v.MoveLines(false) }},
	{"duplicate", func(v *SourceView) error { return v.DuplicateLines(true) }},
	{"delete", (*SourceView).DeleteLines},
}

func TestLineCommandsShareHistoryAndMapOtherView(t *testing.T) {
	d := NewDocument("prev\nalpha\nomega")
	a, b := NewSourceView(), NewSourceView()
	a.SetDocument(d)
	b.SetDocument(d)
	observe(t, a)
	observe(t, b)
	a.SetSelection(textedit.Selection{Anchor: 6, Caret: 9})
	b.SetSelection(textedit.Selection{Anchor: 7, Caret: 7})
	if err := a.MoveLines(false); err != nil {
		t.Fatal(err)
	}
	if d.model.Text() != "alpha\nprev\nomega" || a.Selection() != (textedit.Selection{Anchor: 1, Caret: 4}) || b.Selection() != (textedit.Selection{Anchor: 2, Caret: 2}) {
		t.Fatal("move lost view-local selections", d.model.Text(), a.Selection(), b.Selection())
	}
	if !a.Undo() || d.model.Text() != "prev\nalpha\nomega" || a.Selection() != (textedit.Selection{Anchor: 6, Caret: 9}) || b.Selection() != (textedit.Selection{Anchor: 7, Caret: 7}) {
		t.Fatal("move history or sibling mapping", a.Selection(), b.Selection())
	}
	if !a.Redo() || a.Selection() != (textedit.Selection{Anchor: 1, Caret: 4}) {
		t.Fatal("move redo")
	}
	if err := a.DuplicateLines(true); err != nil {
		t.Fatal(err)
	}
	if d.model.Text() != "alpha\nalpha\nprev\nomega" || a.Selection() != (textedit.Selection{Anchor: 7, Caret: 10}) {
		t.Fatal("copy did not follow new block", d.model.Text(), a.Selection())
	}
	if !a.Undo() || d.model.Text() != "alpha\nprev\nomega" || a.Selection() != (textedit.Selection{Anchor: 1, Caret: 4}) {
		t.Fatal("copy history")
	}
	if err := a.DeleteLines(); err != nil {
		t.Fatal(err)
	}
	if d.model.Text() != "prev\nomega" || a.Selection() != (textedit.Selection{}) {
		t.Fatal("delete line boundary")
	}
	if !a.Undo() || d.model.Text() != "alpha\nprev\nomega" || a.Selection() != (textedit.Selection{Anchor: 1, Caret: 4}) {
		t.Fatal("delete history")
	}
}

func TestLineCommandsRespectRestrictionsAndResetReentry(t *testing.T) {
	for _, command := range viewLineCommands {
		t.Run(command.name, func(t *testing.T) {
			v := NewSourceView()
			v.document.model.SetText("a\nb")
			observe(t, v)
			v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
			v.SetReadOnly(true)
			if err := command.run(v); !errors.Is(err, ErrReadOnly) {
				t.Fatal("read-only", err)
			}
			v.SetReadOnly(false)
			v.SetEnabled(false)
			if err := command.run(v); !errors.Is(err, ErrDisabled) {
				t.Fatal("disabled", err)
			}
			v.SetEnabled(true)
			old := v.Document()
			replacement := NewDocument("keep")
			v.projection = textedit.NewProjection(old.model, v.Selection())
			v.im = &resetContext{reset: func() { v.SetDocument(replacement) }}
			if err := command.run(v); !errors.Is(err, ErrOperationChanged) {
				t.Fatal("preedit reentry applied old plan", err)
			}
			if old.model.Text() != "a\nb" || old.model.CanUndo() || replacement.model.Text() != "keep" {
				t.Fatal("stale plan changed content")
			}
		})
	}
}

func TestLineCommandCallbacksKeepNewDocumentAndSelection(t *testing.T) {
	for _, command := range viewLineCommands {
		t.Run(command.name, func(t *testing.T) {
			v := NewSourceView()
			v.document.model.SetText("a\nb")
			observe(t, v)
			v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
			replacement := NewDocument("new document")
			h := v.ConnectChange(func(textedit.Change) {
				v.SetDocument(replacement)
				v.SetSelection(textedit.Selection{Anchor: 1, Caret: 1})
			})
			defer h.Disconnect()
			if err := command.run(v); err != nil {
				t.Fatal(err)
			}
			if v.Document() != replacement || replacement.model.Text() != "new document" || v.Selection() != (textedit.Selection{Anchor: 1, Caret: 1}) {
				t.Fatal("post-commit callback state overwritten")
			}
		})
	}
}

type fakeClipboard struct {
	text      string
	err       error
	pending   func(string, error)
	write     func()
	immediate bool
}

func (c *fakeClipboard) SetText(text string) error {
	if c.err != nil {
		return c.err
	}
	c.text = text
	if c.write != nil {
		c.write()
	}
	return nil
}

func (c *fakeClipboard) RequestText(fn func(string, error)) {
	if c.immediate {
		fn(c.text, c.err)
	} else {
		c.pending = fn
	}
}

type clipboardApplication struct {
	gui.Application
	clipboard gui.Clipboard
}

func (a clipboardApplication) Clipboard() gui.Clipboard { return a.clipboard }

func clipboardFixture(t *testing.T) (*SourceView, *fakeClipboard) {
	t.Helper()
	previous := gui.App
	clip := &fakeClipboard{}
	gui.App = clipboardApplication{clipboard: clip}
	t.Cleanup(func() { gui.App = previous })
	v := NewSourceView()
	v.Document().Model().SetText("abc")
	observe(t, v)
	v.SetSelection(textedit.Selection{Caret: 2})
	return v, clip
}

func TestCutPreservesTextOnWriteFailureAndReentry(t *testing.T) {
	v, c := clipboardFixture(t)
	c.err = errors.New("native clipboard unavailable")
	if err := v.Cut(); !errors.Is(err, c.err) || v.Document().Model().Text() != "abc" {
		t.Fatal("failed cut deleted text", err)
	}
	c.err = nil
	c.write = func() { v.SetSelection(textedit.Selection{Caret: 1}) }
	if err := v.Cut(); err != nil || v.Document().Model().Text() != "abc" {
		t.Fatal("reentrant copy deleted stale range")
	}
	c.write = nil
	v.SetSelection(textedit.Selection{Caret: 2})
	if err := v.Cut(); err != nil || c.text != "ab" || v.Document().Model().Text() != "c" {
		t.Fatal("cut", err, c.text)
	}
	if !v.Undo() || v.Document().Model().Text() != "abc" {
		t.Fatal("cut history")
	}
}

func TestPasteIdentityErrorsAndSynchronousCompletion(t *testing.T) {
	v, c := clipboardFixture(t)
	reported := 0
	v.ConnectError(func(error) { reported++ })
	if err := v.Paste(); err != nil {
		t.Fatal(err)
	}
	v.SetSelection(textedit.Selection{Caret: 1})
	c.pending("stale", nil)
	if v.Document().Model().Text() != "abc" {
		t.Fatal("stale selection pasted")
	}
	v.SetSelection(textedit.Selection{Anchor: 1, Caret: 2})
	if err := v.Paste(); err != nil {
		t.Fatal(err)
	}
	c.pending("", nil)
	if v.Document().Model().Text() != "abc" || v.Selection() != (textedit.Selection{Anchor: 1, Caret: 2}) || v.pendingPaste != nil {
		t.Fatal("empty paste changed document/selection or retained pending state")
	}
	if reported != 0 {
		t.Fatal("no text emitted error")
	}
	if err := v.Paste(); err != nil {
		t.Fatal(err)
	}
	c.pending("", errors.New("conversion failure"))
	if reported != 1 {
		t.Fatal("read failure not reported")
	}
	c.immediate = true
	c.text = "Ω"
	v.SetSelection(textedit.Selection{Anchor: 1, Caret: 2})
	if err := v.Paste(); err != nil || v.Document().Model().Text() != "aΩc" {
		t.Fatal("synchronous callback", err, v.Document().Model().Text())
	}
	c.immediate = false
	_ = v.Paste()
	pending := v.pendingPaste
	v.unmount()
	if pending.view != nil || pending.document != nil || v.pendingPaste != nil {
		t.Fatal("cancelled callback retains a widget or document")
	}
	c.pending("stale", nil)
	if v.Document().Model().Text() != "aΩc" {
		t.Fatal("unmounted view pasted")
	}
}

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
