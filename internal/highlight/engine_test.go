package highlight

import (
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func drain(t *testing.T, e *Engine) {
	t.Helper()
	limit := time.Now().Add(3 * time.Second)
	for e.Pending() {
		e.Advance(time.Now().Add(time.Millisecond))
		if time.Now().After(limit) {
			t.Fatal("analysis did not finish")
		}
	}
	if e.Err() != nil {
		t.Fatal(e.Err())
	}
}

func checkIndex(t *testing.T, e *Engine, m *textedit.Model) {
	t.Helper()
	parts := strings.Split(m.Text(), "\n")
	if e.LineCount() != len(parts) {
		t.Fatalf("rows=%d want %d", e.LineCount(), len(parts))
	}
	offset := 0
	for i, part := range parts {
		l := e.Line(i)
		want := len(part)
		if i < len(parts)-1 {
			want++
		}
		if l.Bytes != want || e.Offset(i) != offset || l.LF != (i < len(parts)-1) {
			t.Fatalf("row %d: %+v offset %d want %d", i, l, e.Offset(i), offset)
		}
		actual, err := e.Text(i)
		if err != nil || actual != part {
			t.Fatal(actual, err, part)
		}
		offset += want
	}
}

func TestIncrementalIndexAndHighlight(t *testing.T) {
	m := textedit.NewModel("<x>\n<!-- a\n b -->\n<p/>\n</x>")
	e := New(m, language.XML)
	h := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
	defer h.Disconnect()
	drain(t, e)
	oldID := e.Line(4).ID
	if err := m.Replace(textedit.Range{Start: 4, End: 8}, ""); err != nil {
		t.Fatal(err)
	}
	drain(t, e)
	checkIndex(t, e, m)
	if e.Line(4).ID != oldID {
		t.Fatal("unchanged suffix lost identity")
	}
	full := New(m, language.XML)
	drain(t, full)
	for row := 0; row < e.LineCount(); row++ {
		got, _, ok := e.Tokens(row)
		want, _, _ := full.Tokens(row)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("row %d got %+v want %+v", row, got, want)
		}
	}
}

func TestRandomLineSplices(t *testing.T) {
	m := textedit.NewModel("a\nb\n")
	e := New(m, language.XML)
	h := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
	defer h.Disconnect()
	drain(t, e)
	random := rand.New(rand.NewSource(71))
	for iteration := 0; iteration < 500; iteration++ {
		text := m.Text()
		a := random.Intn(len(text) + 1)
		b := a + random.Intn(len(text)-a+1)
		insert := []string{"", "x", "\n", "\n\n", "<p>\n"}[random.Intn(5)]
		if err := m.Replace(textedit.Range{Start: a, End: b}, insert); err != nil {
			t.Fatal(err)
		}
		drain(t, e)
		checkIndex(t, e, m)
	}
}

func TestConvergenceAndStaleRows(t *testing.T) {
	m := textedit.NewModel(strings.Repeat("<p>x</p>\n", 1000))
	e := New(m, language.HTML)
	h := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
	defer h.Disconnect()
	drain(t, e)
	before := e.ScannedLines()
	if err := m.Replace(textedit.Range{Start: 3, End: 4}, "hello"); err != nil {
		t.Fatal(err)
	}
	if _, _, valid := e.Tokens(700); valid {
		t.Fatal("published stale future state")
	}
	drain(t, e)
	if scanned := e.ScannedLines() - before; scanned > 3 {
		t.Fatalf("scanned %d unchanged rows", scanned)
	}
	if _, _, valid := e.Tokens(700); !valid {
		t.Fatal("unchanged suffix did not become valid")
	}
}

func TestReentryAndLongLine(t *testing.T) {
	m := textedit.NewModel("a")
	var e *Engine
	first := m.ConnectChange(func(c textedit.Change) {
		if c.Revision == 1 {
			m.Replace(textedit.Range{Start: 0, End: 0}, "b")
		}
	})
	defer first.Disconnect()
	e = New(m, language.JSON)
	drain(t, e)
	h := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
	defer h.Disconnect()
	m.Replace(textedit.Range{Start: 0, End: 1}, "1")
	drain(t, e)
	checkIndex(t, e, m)
	m.SetText("<x a='" + strings.Repeat("é", 20000) + "'>x</x>")
	e.Reset(language.XML)
	drain(t, e)
	checkIndex(t, e, m)
	tokens, _, ok := e.Tokens(0)
	if !ok || kindAt(tokens, 40000) != language.KindString {
		t.Fatal("long UTF-8 attribute was not continued", ok)
	}
}

func TestMalformedMarkupProgress(t *testing.T) {
	for _, text := range []string{"<x / x>", "<x ===>", "<x a=>", "<x < <", "<!DOCTYPE x [<!-- ] > -->]><p/>"} {
		tokens, state := scanAll(language.XML, text, 2)
		_ = tokens
		if text[len(text)-4:] == "<p/>" && !state.Equal(Initial()) {
			t.Fatal("declaration comment lost context")
		}
	}
}

func TestLanguageSwitchKeepsMetadataAndHidesOldTokens(t *testing.T) {
	m := textedit.NewModel("true\nfalse")
	e := New(m, language.JSON)
	drain(t, e)
	ids := []uint64{e.Line(0).ID, e.Line(1).ID}
	tokens, _, ok := e.Tokens(0)
	if !ok || !reflect.DeepEqual(tokens, []Token{{Start: 0, End: 4, Kind: language.KindKeyword}}) {
		t.Fatal("missing JSON keyword", tokens)
	}
	e.SetLanguage(language.XML)
	if tokens, _, ok := e.Tokens(1); ok || len(tokens) != 0 {
		t.Fatal("old language tokens exposed while dirty", tokens)
	}
	drain(t, e)
	e.SetLanguage(language.Plain)
	if tokens, _, ok := e.Tokens(0); !ok || len(tokens) != 0 {
		t.Fatal("plain language retained lexical tokens", tokens)
	}
	e.SetLanguage(language.JSON)
	drain(t, e)
	for row, id := range ids {
		if e.Line(row).ID != id {
			t.Fatal("language changed text identity", row)
		}
	}
	tokens, _, ok = e.Tokens(0)
	if !ok || !reflect.DeepEqual(tokens, []Token{{Start: 0, End: 4, Kind: language.KindKeyword}}) {
		t.Fatal("language switch skipped reanalysis", tokens)
	}
}

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

type customScanner struct{ kind language.Kind }

func (customScanner) InitialState() language.State { return language.EmptyState{} }

func (s customScanner) Scan(in language.Input, state language.State) (language.Result, error) {
	r := language.Result{Consumed: len(in.Text), State: state}
	if len(in.Text) > 0 {
		r.Tokens = []language.Token{{Start: in.Offset, End: in.Offset + len(in.Text), Kind: s.kind, Class: language.ClassDelimiter}}
	}
	return r, nil
}

func TestSameIDDefinitionInvalidation(t *testing.T) {
	makeLanguage := func(kind language.Kind) *language.Language {
		l, err := language.New(language.Definition{Info: language.Info{ID: "custom", Name: "Custom"}, Tokenizer: customScanner{kind}})
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	m := textedit.NewModel("abc\nxyz")
	e := NewWithLanguage(m, makeLanguage("first"))
	drain(t, e)
	before := e.Line(0).ID
	e.SetDefinition(makeLanguage("second"))
	if _, _, valid := e.Tokens(0); valid {
		t.Fatal("old tokens exposed")
	}
	drain(t, e)
	tokens, _, ok := e.Tokens(0)
	if !ok || len(tokens) != 1 || tokens[0].Kind != "second" || tokens[0].Class != language.ClassDelimiter || e.Line(0).ID != before {
		t.Fatalf("new definition: %v", tokens)
	}
}

type badScanner struct{ customScanner }

func (s badScanner) Scan(in language.Input, state language.State) (language.Result, error) {
	return language.Result{Consumed: len(in.Text) + 1, State: state}, nil
}

func TestInvalidProviderStopsAnalysis(t *testing.T) {
	l, _ := language.New(language.Definition{Info: language.Info{ID: "bad", Name: "Bad"}, Tokenizer: badScanner{}})
	e := NewWithLanguage(textedit.NewModel("text"), l)
	for e.Pending() {
		e.Advance(time.Now().Add(time.Second))
	}
	if e.Err() == nil {
		t.Fatal("invalid scan accepted")
	}
	if _, _, ok := e.Tokens(0); ok {
		t.Fatal("invalid scan published")
	}
}

func TestNoTokenizerEditsOnlyUpdateLineMetadata(t *testing.T) {
	l, _ := language.New(language.Definition{Info: language.Info{ID: "uncolored", Name: "No colors"}})
	for _, definition := range []*language.Language{l, func() *language.Language { plain, _ := language.Builtin(language.Plain); return plain }()} {
		m := textedit.NewModel("one\ntwo")
		e := NewWithLanguage(m, definition)
		drain(t, e)
		handle := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
		if err := m.Replace(textedit.Range{Start: 0, End: 3}, "a\nb"); err != nil {
			t.Fatal(err)
		}
		if e.Pending() {
			t.Fatal("scheduled nonexistent tokenizer")
		}
		e.Advance(time.Time{})
		checkIndex(t, e, m)
		if tokens, _, ok := e.Tokens(0); !ok || len(tokens) != 0 {
			t.Fatal(tokens, ok)
		}
		handle.Disconnect()
	}
}

// Optional pure-analysis benchmark. Never downloads files or creates a GUI.
// Reading and model construction are outside timing; metadata building and
// complete initial tokenization are inside. Cached native rendering is excluded.
func BenchmarkCorpusAnalysis(b *testing.B) {
	dir := os.Getenv("GOUI_CORPUS_DIR")
	if dir == "" {
		b.Skip("optional corpus benchmark: set GOUI_CORPUS_DIR to the downloaded directory")
	}
	for _, name := range []string{"war-and-peace.txt", "github-api.json", "opengl.xml", "war-and-peace.html", "rust-releases.md"} {
		b.Run(name, func(b *testing.B) {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				b.Fatal(err)
			}
			model := textedit.NewModel(string(data))
			id := language.Guess(name, data[:min(len(data), 4096)])
			b.ReportAllocs()
			b.SetBytes(int64(model.Len()))
			b.ResetTimer()
			for range b.N {
				engine := New(model, id)
				engine.Advance(time.Time{})
				if engine.Err() != nil || engine.Pending() || !engine.Ready() || engine.LineCount() != model.LineCount() {
					b.Fatal("analysis incomplete", engine.Err(), engine.LineCount())
				}
			}
		})
	}
}

func TestLineStoreBlockAggregatesMatchEveryBytePosition(t *testing.T) {
	var parts []string
	for row := 0; row < 4096; row++ {
		parts = append(parts, strings.Repeat("x", row%37)+"中")
	}
	parts = append(parts, "") // The trailing empty paragraph is a legal endpoint.
	model := textedit.NewModel(strings.Join(parts, "\n"))
	engine := New(model, language.Plain)
	engine.Advance(time.Time{})
	if engine.Err() != nil || engine.Pending() || engine.LineCount() != len(parts) {
		t.Fatal("metadata incomplete", engine.Err())
	}
	start := 0
	for row, part := range parts {
		if got := engine.lines.offset(row); got != start {
			t.Fatalf("row %d offset=%d want=%d", row, got, start)
		}
		end := start + len(part)
		if row+1 < len(parts) {
			end++
		}
		for at := start; at < end; at++ {
			if got := engine.lines.lineAt(at); got != row {
				t.Fatalf("byte %d row=%d want=%d", at, got, row)
			}
		}
		start = end
	}
	if engine.lines.lineAt(model.Len()) != len(parts)-1 || engine.lines.offset(len(parts)) != model.Len() {
		t.Fatal("exclusive block/document endpoint mapping")
	}
}
