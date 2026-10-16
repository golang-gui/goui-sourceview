package sourceview

import (
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
	"testing"
	"time"
)

func TestAutoPairsHistoryAndTracking(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	m := v.document.model
	typeText := func(text string) {
		t.Helper()
		handled, err := v.pairText(v.Selection().Range(), text)
		if err != nil {
			t.Fatal(err)
		}
		if !handled {
			if err := v.ReplaceSelection(text); err != nil {
				t.Fatal(err)
			}
		}
	}
	typeText("(")
	if m.Text() != "()" || v.Selection().Caret != 1 {
		t.Fatal(m.Text(), v.Selection())
	}
	typeText("[")
	typeText("]")
	typeText(")")
	if m.Text() != "([])" || v.Selection().Caret != 4 {
		t.Fatal("nested overtype", m.Text(), v.Selection())
	}
	if !v.Undo() || m.Text() != "()" || v.Selection().Caret != 1 {
		t.Fatal("pair undo", m.Text(), v.Selection())
	}
	if !v.Redo() || m.Text() != "([])" || v.Selection().Caret != 2 {
		t.Fatal("pair redo selection", m.Text(), v.Selection())
	}
	// Redo does not fabricate generated-pair ownership.
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	typeText("]")
	if m.Text() != "([]])" {
		t.Fatal("skipped a non-generated closer", m.Text())
	}
	m.SetText("")
	v.SetSelection(textedit.Selection{})
	typeText("{")
	if ok, err := v.deletePair(); !ok || err != nil || m.Text() != "" {
		t.Fatal("paired backspace", ok, err, m.Text())
	}
	if !v.Undo() || m.Text() != "{}" {
		t.Fatal("paired backspace undo")
	}
	m.SetText("word")
	v.SetSelection(textedit.Selection{Anchor: 4, Caret: 0})
	typeText("\"")
	if m.Text() != `"word"` || v.Selection() != (textedit.Selection{Anchor: 5, Caret: 1}) {
		t.Fatal("surround", m.Text(), v.Selection())
	}
	if !v.Undo() || m.Text() != "word" || v.Selection() != (textedit.Selection{Anchor: 4, Caret: 0}) {
		t.Fatal("surround history")
	}
	m.SetText("")
	v.SetSelection(textedit.Selection{})
	typeText("[")
	if err := m.Replace(textedit.Range{}, "x"); err != nil {
		t.Fatal(err)
	}
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	typeText("]")
	if m.Text() != "x[]" || v.Selection().Caret != 3 {
		t.Fatal("mapped pair", m.Text(), v.Selection())
	}
}

func TestPairsRespectLexicalContext(t *testing.T) {
	for _, tt := range []struct {
		id                language.ID
		text, typed, want string
		at                int
	}{
		{language.JSON, `{"x":"abc"}`, "{", `{"x":"ab{c"}`, 8},
		{language.JSON, "", "\"", `""`, 0},
		{language.JSON, "", "'", "'", 0},
		{language.XML, "<x a=", `"`, `<x a=""`, 5},
		{language.HTML, "<!-- ", "<", "<!-- <", 5},
		{language.XML, "", "<", "<>", 0},
	} {
		t.Run(tt.text+tt.typed, func(t *testing.T) {
			v := NewSourceView()
			v.document.model.SetText(tt.text)
			if err := v.document.SetLanguage(tt.id); err != nil {
				t.Fatal(err)
			}
			observe(t, v)
			v.document.engine.Advance(time.Time{})
			v.SetSelection(textedit.Selection{Anchor: tt.at, Caret: tt.at})
			handled, err := v.pairText(v.Selection().Range(), tt.typed)
			if err != nil {
				t.Fatal(err)
			}
			if !handled {
				err = v.ReplaceSelection(tt.typed)
			}
			if err != nil || v.document.model.Text() != tt.want {
				t.Fatal(v.document.model.Text(), tt.want, err)
			}
		})
	}
}

func TestMatchingBracketsUsesLexicalTokens(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText(`{"x": "}", "y": [1,2]}`)
	_ = v.document.SetLanguage(language.JSON)
	observe(t, v)
	v.document.engine.Advance(time.Time{})
	v.SetSelection(textedit.Selection{})
	r := v.matchingBrackets()
	if len(r) != 2 || r[0].Start != 0 || r[1].End != v.document.model.Len() {
		t.Fatal("string brace changed matching", r)
	}
	v.document.model.SetText("{[}]")
	v.document.engine.Advance(time.Time{})
	v.SetSelection(textedit.Selection{})
	if r := v.matchingBrackets(); len(r) != 1 {
		t.Fatal("crossed mismatched bracket", r)
	}
	v.document.model.SetText(`{"x":"[]"}`)
	v.document.engine.Advance(time.Time{})
	v.SetSelection(textedit.Selection{Anchor: 6, Caret: 6})
	if r := v.matchingBrackets(); len(r) != 0 {
		t.Fatal("matched a string bracket", r)
	}
}

func TestQuoteEscapeAndPairNewline(t *testing.T) {
	v := NewSourceView()
	_ = v.document.SetLanguage(language.JSON)
	observe(t, v)
	if ok, err := v.pairText(textedit.Range{}, `"`); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if err := v.ReplaceSelection(`\`); err != nil {
		t.Fatal(err)
	}
	if handled, err := v.pairText(v.Selection().Range(), `"`); handled || err != nil {
		t.Fatal("escaped quote skipped generated closer", handled, err)
	}
	v.document.model.SetText("  {}")
	v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
	if err := v.newline(); err != nil {
		t.Fatal(err)
	}
	if v.document.model.Text() != "  {\n      \n  }" || v.Selection().Caret != 10 {
		t.Fatal(v.document.model.Text(), v.Selection())
	}
	if !v.Undo() || v.document.model.Text() != "  {}" || v.Selection().Caret != 3 {
		t.Fatal("newline history")
	}
}
