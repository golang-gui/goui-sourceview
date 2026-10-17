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
