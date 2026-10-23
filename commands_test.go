package sourceview

import (
	"errors"
	"testing"

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
