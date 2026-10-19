package sourceview

import (
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
	"testing"
	"time"
)

func TestTagCloseLinkedEditAndUndo(t *testing.T) {
	v := NewSourceView()
	v.document.SetLanguage(language.XML)
	v.document.model.SetText("<ns:a")
	observe(t, v)
	v.document.engine.Advance(time.Time{})
	v.SetSelection(textedit.Selection{Anchor: 5, Caret: 5})
	handled, err := v.closeTag(v.Selection().Range(), ">")
	if !handled || err != nil || v.document.model.Text() != "<ns:a></ns:a>" || v.Selection().Caret != 6 {
		t.Fatal(v.document.model.Text(), v.Selection(), err)
	}
	v.SetSelection(textedit.Selection{Anchor: 5, Caret: 5})
	handled, err = v.linkedTag(v.Selection().Range(), "b")
	if !handled || err != nil || v.document.model.Text() != "<ns:ab></ns:ab>" {
		t.Fatal(v.document.model.Text(), err)
	}
	if !v.Undo() || v.document.model.Text() != "<ns:a></ns:a>" {
		t.Fatal("linked undo")
	}
	if !v.Undo() || v.document.model.Text() != "<ns:a" {
		t.Fatal("close undo")
	}
}
func TestEmbeddedEditingAndCompletion(t *testing.T) {
	v := NewSourceView()
	v.document.SetLanguage(language.Markdown)
	v.document.model.SetText("```json\n{\"value\": tr}\n```\n- item")
	observe(t, v)
	v.document.engine.Advance(time.Time{})
	v.SetSelection(textedit.Selection{Anchor: 20, Caret: 20})
	ctx, ok := v.completionContext(true)
	if !ok || ctx.Language != language.JSON || ctx.Definition.ID() != language.JSON || v.document.Language() != language.Markdown {
		t.Fatal(ctx, ok)
	}
	v.SetSelection(textedit.Selection{Anchor: v.document.model.Len(), Caret: v.document.model.Len()})
	if err := v.newline(); err != nil || v.document.model.Text() != "```json\n{\"value\": tr}\n```\n- item\n- " {
		t.Fatal(v.document.model.Text(), err)
	}
}
