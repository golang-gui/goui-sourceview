package sourceview

import (
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
	"strings"
	"testing"
	"time"
)

func TestStructuralSelectionRoundTrip(t *testing.T) {
	v := NewSourceView()
	d := v.document
	text := `{"items": ["alphaBeta", 2]}`
	d.model.SetText(text)
	d.SetLanguage(language.JSON)
	observe(t, v)
	for d.engine.Pending() {
		d.engine.Advance(time.Time{})
	}
	at := strings.Index(text, "alpha") + 2
	v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
	original := v.Selection()
	wants := []string{"alphaBeta", `"alphaBeta"`, `"alphaBeta", 2`, `["alphaBeta", 2]`, `"items": ["alphaBeta", 2]`, text}
	for _, want := range wants {
		if !v.ExpandSelection() {
			t.Fatal("未扩大", want)
		}
		got, _ := d.model.Slice(v.Selection().Range())
		if got != want {
			t.Fatalf("%q != %q", got, want)
		}
	}
	for range wants {
		if !v.ShrinkSelection() {
			t.Fatal("未缩小")
		}
	}
	if v.Selection() != original {
		t.Fatal("没有恢复原始光标")
	}
	v.ExpandSelection()
	v.SetSelection(v.Selection())
	if v.ShrinkSelection() {
		t.Fatal("外部设置选区没有失效")
	}
	v.SetReadOnly(true)
	if !v.ExpandSelection() {
		t.Fatal("只读禁止选择")
	}
	if err := v.DeleteWordPart(true); err != ErrReadOnly {
		t.Fatal(err)
	}
}
func TestStructuralSelectionQuotedProperty(t *testing.T) {
	v := NewSourceView()
	d := v.document
	d.model.SetText(`{"alphaBeta":1}`)
	d.SetLanguage(language.JSON)
	observe(t, v)
	for d.engine.Pending() {
		d.engine.Advance(time.Time{})
	}
	v.SetSelection(textedit.Selection{Anchor: 4, Caret: 4})
	for _, want := range []string{"alphaBeta", `"alphaBeta"`} {
		if !v.ExpandSelection() {
			t.Fatal("没有扩大属性选区")
		}
		got, _ := d.model.Slice(v.Selection().Range())
		if got != want {
			t.Fatalf("%q != %q", got, want)
		}
	}
}

func TestTagSelectionAndReverseTrail(t *testing.T) {
	v := NewSourceView()
	d := v.document
	text := "<root><b>hello</b></root>"
	d.model.SetText(text)
	d.SetLanguage(language.XML)
	observe(t, v)
	for d.engine.Pending() {
		d.engine.Advance(time.Time{})
	}
	v.SetSelection(textedit.Selection{Anchor: 13, Caret: 10})
	original := v.Selection()
	for _, want := range []string{"hello", "<b>hello</b>", text} {
		if !v.ExpandSelection() {
			t.Fatal("未扩大")
		}
		got, _ := d.model.Slice(v.Selection().Range())
		if got != want || v.Selection().Anchor < v.Selection().Caret {
			t.Fatal(got, v.Selection())
		}
	}
	for range 3 {
		v.ShrinkSelection()
	}
	if v.Selection() != original {
		t.Fatal("方向或范围错误")
	}
}
