package highlight

import (
	"math/rand"
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
