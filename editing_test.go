package sourceview

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestCleanupPlansAndUndo(t *testing.T) {
	for _, tc := range []struct {
		text, kind, want string
		s                textedit.Selection
	}{
		{"one\n  two\nthree", "join", "one two\nthree", textedit.Selection{}},
		{"one\n  two\nthree", "join", "one two three", textedit.Selection{Anchor: 15}},
		{"one\n  two\nthree", "join", "one two\nthree", textedit.Selection{Caret: 10}},
		{"a \t\nb\u00a0\t\n", "trim", "a\nb\u00a0\n", textedit.Selection{}},
		{"a \nb \nc ", "trim", "a\nb \nc ", textedit.Selection{Caret: 3}},
		{"one", "join", "one", textedit.Selection{}},
	} {
		m := textedit.NewModel(tc.text)
		p, err := cleanupPlan(context.Background(), tc.text, tc.s, tc.kind)
		if err != nil {
			t.Fatal(err)
		}
		if err = p.Apply(m, nil); err != nil {
			t.Fatal(err)
		}
		if m.Text() != tc.want {
			t.Fatalf("%s: %q != %q", tc.kind, m.Text(), tc.want)
		}
		if tc.want != tc.text {
			r := m.UndoWithSelection()
			if m.Text() != tc.text || r.Selection == nil || *r.Selection != tc.s {
				t.Fatal("撤销", r)
			}
		} else if m.CanUndo() {
			t.Fatal("空操作产生历史")
		}
	}
}
func TestReindentPlanValidatesBeforeMutation(t *testing.T) {
	text := "{\n\"a\": 1\n}"
	s := textedit.Selection{Anchor: len(text)}
	for _, bad := range [][]language.LineIndent{{{Line: 1, Text: "x"}}, {{Line: 1, Text: "\n"}}, {{Line: 1, Text: " "}, {Line: 1, Text: "  "}}, {{Line: 3, Text: " "}}, {{Line: -1, Text: " "}}} {
		if _, err := reindentPlan(context.Background(), text, s, 0, 2, bad); err == nil {
			t.Fatal("未拒绝非法结果", bad)
		}
	}
	p, err := reindentPlan(context.Background(), text, s, 0, 2, []language.LineIndent{{Line: 0, Text: ""}, {Line: 1, Text: "  "}, {Line: 2, Text: ""}})
	if err != nil {
		t.Fatal(err)
	}
	m := textedit.NewModel(text)
	if err = p.Apply(m, nil); err != nil {
		t.Fatal(err)
	}
	if m.Text() != "{\n  \"a\": 1\n}" || p.After.Anchor != len(text)+2 || p.After.Caret != 0 {
		t.Fatal(m.Text(), p.After)
	}
	r := m.UndoWithSelection()
	if *r.Selection != s || m.Text() != text {
		t.Fatal("撤销未恢复")
	}
}
func TestPasteRelativeIndentAndLiteralProtection(t *testing.T) {
	o := IndentOptions{TabWidth: 4, IndentWidth: 4, InsertSpaces: true}
	for _, tc := range []struct {
		id                 language.ID
		text, prefix, want string
	}{
		{language.JSON, "    {\n      \"a\": 1\n    }", "  ", "{\n    \"a\": 1\n  }"},
		{language.Go, "x := `a\n  b`", "  ", "x := `a\n  b`"},
		{language.TOML, "text = \"\"\"a\n  b\"\"\"", "  ", "text = \"\"\"a\n  b\"\"\""},
		{language.Python, "x = '''a\n  b'''", "  ", "x = '''a\n  b'''"},
		{language.JSON, "\t{\r\n\t\t1\r\n\t}", "  ", "{\n      1\n  }"},
	} {
		l, _ := language.Builtin(tc.id)
		got, err := reindentedPaste(context.Background(), tc.text, tc.prefix, o, l)
		if err != nil || got != tc.want {
			t.Fatalf("%s: %q != %q (%v)", tc.id, got, tc.want, err)
		}
	}
}
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

func TestBuiltinNewlineUsesLexicalState(t *testing.T) {
	for _, tc := range []struct {
		id         language.ID
		text, want string
	}{
		{language.YAML, "key:", "key:\n    "},
		{language.YAML, "- key:", "- key:\n    "},
		{language.YAML, "text: |2", "text: |2\n    "},
		{language.YAML, "text: |2\n  - data", "text: |2\n  - data\n  "},
		{language.YAML, "list: [", "list: [\n    "},
		{language.Go, "s := `raw\n  literal", "s := `raw\n  literal\n  "},
		{language.XML, "<node>", "<node>\n    "},
	} {
		t.Run(string(tc.id)+tc.text, func(t *testing.T) {
			v := NewSourceView()
			d := v.document
			d.model.SetText(tc.text)
			d.SetLanguage(tc.id)
			observe(t, v)
			for d.engine.Pending() {
				d.engine.Advance(time.Time{})
			}
			v.SetSelection(textedit.Selection{Anchor: len(tc.text), Caret: len(tc.text)})
			if err := v.newline(); err != nil {
				t.Fatal(err)
			}
			if d.model.Text() != tc.want {
				t.Fatalf("%q != %q", d.model.Text(), tc.want)
			}
			if !v.Undo() || d.model.Text() != tc.text {
				t.Fatal("Enter 撤销")
			}
		})
	}
}
