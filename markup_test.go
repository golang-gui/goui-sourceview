package sourceview

import (
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
	"strings"
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

func TestMarkdownTypedFenceAndNewline(t *testing.T) {
	v := NewSourceView()
	v.document.SetLanguage(language.Markdown)
	observe(t, v)
	for range 3 {
		v.document.engine.Advance(time.Time{})
		r := v.Selection().Range()
		handled, err := v.pairText(r, "`")
		if !handled && err == nil {
			err = v.replace(r, "`", false)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if v.document.model.Text() != "``````" || v.Selection().Caret != 3 {
		t.Fatal(v.document.model.Text(), v.Selection())
	}
	v.document.engine.Advance(time.Time{})
	if err := v.newline(); err != nil || v.document.model.Text() != "```\n\n```" || v.Selection().Caret != 4 {
		t.Fatal(v.document.model.Text(), v.Selection(), err)
	}
}

func TestLinkedTagEditNotRepeatedBySharedView(t *testing.T) {
	v := NewSourceView()
	doc := NewDocument("<a></a>")
	doc.SetLanguage(language.XML)
	v.SetDocument(doc)
	observe(t, v)
	other := NewSourceView()
	other.SetDocument(doc)
	observe(t, other)
	revisions := 0
	h := doc.Model().ConnectChange(func(textedit.Change) { revisions++ })
	defer h.Disconnect()
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	handled, err := v.linkedTag(v.Selection().Range(), "b")
	if !handled || err != nil || revisions != 1 || doc.Model().Text() != "<ab></ab>" {
		t.Fatal(doc.Model().Text(), revisions, err)
	}
	if !other.Undo() || doc.Model().Text() != "<a></a>" {
		t.Fatal("shared undo lost atomic names")
	}
}

func TestTagAssistanceRemainsAvailableBeyondSmallDocuments(t *testing.T) {
	prefix := "<!--" + strings.Repeat("x", 300<<10) + "-->\n"
	v := NewSourceView()
	doc := NewDocument(prefix + "<node")
	doc.SetLanguage(language.XML)
	v.SetDocument(doc)
	observe(t, v)
	at := doc.Model().Len()
	v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
	handled, err := v.closeTag(v.Selection().Range(), ">")
	if !handled || err != nil || doc.Model().Text() != prefix+"<node></node>" {
		t.Fatal("tag close disabled by document size", err)
	}
	at = len(prefix) + 5
	v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
	handled, err = v.linkedTag(v.Selection().Range(), "x")
	if !handled || err != nil || doc.Model().Text() != prefix+"<nodex></nodex>" {
		t.Fatal("linked name disabled by document size", err)
	}
}
