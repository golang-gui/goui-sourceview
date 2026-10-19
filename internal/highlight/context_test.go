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

func TestConfigurationAndShellEditingContext(t *testing.T) {
	for _, tc := range []struct {
		id                    language.ID
		text                  string
		str, comment, literal bool
	}{
		{language.TOML, "text = '''raw\nbody", true, false, true},
		{language.TOML, "text = \"\"\"body\nmore", true, false, false},
		{language.TOML, "text = 1 # note", false, true, false},
		{language.YAML, "text: |\n  content", true, false, false},
		{language.YAML, "text: 'raw\\", true, false, true},
		{language.YAML, "# comment", false, true, false},
		{language.Shell, "echo 'literal\\", true, false, true},
		{language.Shell, "echo \"$(echo ", false, false, false},
		{language.Shell, "cat <<'END'\nbody", true, false, true},
		{language.Shell, "echo value # note", false, true, false},
	} {
		m := textedit.NewModel(tc.text)
		e := New(m, tc.id)
		e.Advance(time.Time{})
		c, ok := e.ContextAt(m.Len())
		if !ok || c.String != tc.str || c.Comment != tc.comment || c.Literal != tc.literal {
			t.Errorf("%s %q: %+v known=%v", tc.id, tc.text, c, ok)
		}
	}
}
