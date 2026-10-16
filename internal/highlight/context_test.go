package highlight

import (
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
	"strings"
	"testing"
	"time"
)

func TestEditingContextAcrossLinesAndInvalidation(t *testing.T) {
	m := textedit.NewModel("<!--\nstill comment\n-->\n<x a=\"")
	e := New(m, language.XML)
	if _, ok := e.ContextAt(8); ok {
		t.Fatal("guessed unresolved previous line")
	}
	e.Advance(time.Time{})
	if c, ok := e.ContextAt(8); !ok || !c.Comment {
		t.Fatal(c, ok)
	}
	if c, ok := e.ContextAt(m.Len()); !ok || !c.String {
		t.Fatal(c, ok)
	}
	e.SetLanguage(language.JSON)
	if _, ok := e.ContextAt(m.Len()); ok {
		t.Fatal("used previous language context")
	}
	m = textedit.NewModel(strings.Repeat(" ", 17<<10))
	e = New(m, language.JSON)
	if _, ok := e.ContextAt(m.Len()); ok {
		t.Fatal("unbounded prefix scan")
	}
}
