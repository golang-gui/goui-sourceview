package sourceview

import (
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"slices"
	"testing"
	"time"
)

func TestLanguageAndDocumentCompletions(t *testing.T) {
	for _, tt := range []struct {
		id           language.ID
		text         string
		at           int
		prefix, want string
	}{
		{language.JSON, "tr", 2, "tr", "true"},
		{language.HTML, "<di", 3, "di", "div"},
		{language.HTML, `<a cl`, 5, "cl", "class"},
		{language.XML, `<x xml:l`, 8, "xml:l", "xml:lang"},
		{language.XML, "<widget/>\n<wid", 14, "wid", "widget"},
		{language.Plain, "你好世界 你好", len("你好世界 你好"), "你好", "你好世界"},
	} {
		t.Run(tt.text, func(t *testing.T) {
			v := NewSourceView()
			v.document.model.SetText(tt.text)
			_ = v.document.SetLanguage(tt.id)
			observe(t, v)
			v.document.engine.Advance(time.Time{})
			v.SetSelection(textedit.Selection{Anchor: tt.at, Caret: tt.at})
			c, ok := v.completionContext(false)
			if !ok || c.Prefix != tt.prefix {
				t.Fatal(c, ok)
			}
			items := normalizeCompletions(v.proposals(c), c)
			if !slices.ContainsFunc(items, func(i Completion) bool { return i.Text == tt.want }) {
				t.Fatal(items, tt.want)
			}
		})
	}
}

type closingPopup struct {
	gui.Popover
	onClose func()
}

func (p closingPopup) Destroy() { p.onClose() }

func TestCompletionCloseCallbackCannotApplyStaleRange(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	v.document.model.SetText("tr")
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	original := v.document
	replacement := NewDocument("keep")
	s := &completionSession{state: v.operationState(), range_: textedit.Range{Start: 0, End: 2}, items: []Completion{{Text: "true"}}, list: &completionList{}}
	s.popup = closingPopup{onClose: func() { v.SetDocument(replacement) }}
	v.completion = s
	if err := v.acceptCompletion(s, 0); err != ErrOperationChanged {
		t.Fatal(err)
	}
	if v.document != replacement || replacement.model.Text() != "keep" || original.model.Text() != "tr" {
		t.Fatal("stale completion modified document")
	}
}
func TestCompletionReplacesWholeWordAndBoundsResults(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("alpha tail")
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	c, ok := v.completionContext(true)
	if !ok || c.Prefix != "al" || c.Range != (textedit.Range{Start: 0, End: 5}) {
		t.Fatal(c, ok)
	}
	items := normalizeCompletions([]Completion{{Label: "alpha"}, {Label: "alpha"}, {Label: "alpine"}, {Label: "al\nbad"}, {Label: "z"}}, c)
	if len(items) != 2 || items[0].Text != "alpha" || items[1].Text != "alpine" {
		t.Fatal(items)
	}
	v.SetSelection(textedit.Selection{Caret: 2})
	if _, ok := v.completionContext(true); ok {
		t.Fatal("selection offered completion")
	}
	if v.ShowCompletion() != ErrNotMounted {
		t.Fatal("unmounted completion")
	}
}

func TestCompletionLabelsStayOnOneNativeLine(t *testing.T) {
	for _, separator := range []string{"\r", "\n", "\t", "\v", "\f", "\u0085", "\u2028", "\u2029"} {
		items := normalizeCompletions([]Completion{{Label: "a" + separator + "b"}}, CompletionContext{})
		if len(items) != 0 {
			t.Fatalf("accepted native line separator %q", separator)
		}
	}
}
