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

// 显示可暂用同一定义的旧颜色，但编辑上下文和 Class 不能借此变成有效结果。
func TestPaintTokensSurviveInvalidation(t *testing.T) {
	m := textedit.NewModel("/* note */\nconst value = true;\nreturn value;")
	e := New(m, language.JavaScript)
	h := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
	defer h.Disconnect()
	drain(t, e)
	old, version, _ := e.PaintTokens(1)
	old = append([]Token(nil), old...)
	if err := m.ReplaceAtomic(textedit.Range{Start: 8, End: 10}, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, valid := e.Tokens(1); valid {
		t.Fatal("stale semantic tokens exposed")
	}
	if _, valid := e.ContextAt(len("/* note \nconst")); valid {
		t.Fatal("stale predecessor accepted for editing")
	}
	got, current, ok := e.PaintTokens(1)
	if !ok || !reflect.DeepEqual(got, old) || current != version {
		t.Fatal("pending suffix lost display snapshot", got)
	}
	drain(t, e)
	got, _, ok = e.PaintTokens(1)
	if !ok || kindAt(got, 0) != language.KindComment {
		t.Fatal("new cross-line comment was not published", got)
	}
	checkPaintPayload(t, e)
	for _, id := range []language.ID{language.XML, language.Plain, language.JavaScript} {
		e.SetLanguage(id)
		if got, _, _ := e.PaintTokens(1); len(got) > 0 {
			t.Fatal("old definition display tokens leaked", id, got)
		}
		drain(t, e)
	}
}

func checkPaintPayload(t *testing.T, e *Engine) {
	t.Helper()
	bytes := 0
	for row := 0; row < e.LineCount(); row++ {
		bytes += len(e.lines.at(row).tokens) * 40
	}
	if e.work != nil {
		bytes += len(e.work.tokens) * 40
	}
	if bytes != e.payloadBytes || bytes > tokenBudget {
		t.Fatal("paint/work payload accounting", bytes, e.payloadBytes)
	}
}

// 字节范围映射覆盖 Unicode、分行、合行、替换及撤销/重做，不依赖 tokenizer 的最终修正。
func TestPaintTokensMapEdits(t *testing.T) {
	for _, tc := range []struct {
		text       string
		start, end int
		insert     string
		want       [][]Token
	}{
		{"\"ab字cd\"", 3, 3, "é", [][]Token{{{Start: 0, End: 11, Kind: language.KindString, Class: language.ClassString}}}},
		{"\"abcdef\"", 3, 5, "", [][]Token{{{Start: 0, End: 6, Kind: language.KindString, Class: language.ClassString}}}},
		{"\"abcdef\"", 3, 3, "\nx\n", [][]Token{{{Start: 0, End: 3, Kind: language.KindString, Class: language.ClassString}}, {{Start: 0, End: 1, Kind: language.KindString, Class: language.ClassString}}, {{Start: 0, End: 5, Kind: language.KindString, Class: language.ClassString}}}},
		{"\"abc\"\n\"def\"", 4, 8, "", [][]Token{{{Start: 0, End: 7, Kind: language.KindString, Class: language.ClassString}}}},
		{"\"abcdef\"", 3, 5, "XY", [][]Token{{{Start: 0, End: 3, Kind: language.KindString, Class: language.ClassString}, {Start: 5, End: 8, Kind: language.KindString, Class: language.ClassString}}}},
	} {
		m := textedit.NewModel(tc.text)
		e := New(m, language.JSON)
		h := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
		drain(t, e)
		if err := m.ReplaceAtomic(textedit.Range{Start: tc.start, End: tc.end}, tc.insert); err != nil {
			t.Fatal(err)
		}
		for row, want := range tc.want {
			got, _, ok := e.PaintTokens(row)
			if !ok || !reflect.DeepEqual(got, want) {
				t.Fatalf("%q row %d: got %v want %v", tc.text, row, got, want)
			}
		}
		checkPaintPayload(t, e)
		drain(t, e)
		if !m.Undo() || !m.Redo() {
			t.Fatal("missing edit history")
		}
		checkPaintPayload(t, e)
		drain(t, e)
		fresh := New(m, language.JSON)
		drain(t, fresh)
		for row := 0; row < e.LineCount(); row++ {
			got, _, _ := e.Tokens(row)
			want, _, _ := fresh.Tokens(row)
			if !reflect.DeepEqual(got, want) {
				t.Fatal("mapping changed final lexical result", got, want)
			}
		}
		h.Disconnect()
	}
}

type pacedScanner struct {
	language.Tokenizer
	slow bool
}

func (s *pacedScanner) Scan(in language.Input, state language.State) (language.Result, error) {
	r, err := s.Tokenizer.Scan(in, state)
	if s.slow {
		time.Sleep(2 * time.Millisecond)
	}
	return r, err
}

// 显式延迟一个扫描片段，使中间状态确定存在；不把测量/高亮同步塞进输入回调。
func TestPaintSnapshotSurvivesPartialWorkAndCancellation(t *testing.T) {
	base, _ := language.Builtin(language.JSON)
	scanner := &pacedScanner{Tokenizer: base.Tokenizer()}
	l, err := language.New(language.Definition{Info: language.Info{ID: "paced", Name: "Paced JSON"}, Tokenizer: scanner})
	if err != nil {
		t.Fatal(err)
	}
	m := textedit.NewModel("\"" + strings.Repeat("x", ChunkSize*8) + "\"\ntrue")
	e := NewWithLanguage(m, l)
	h := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
	defer h.Disconnect()
	drain(t, e)
	for i := 0; i < 5; i++ {
		if err := m.ReplaceAtomic(textedit.Range{Start: 10, End: 10}, "y"); err != nil {
			t.Fatal(err)
		}
		_, version, _ := e.PaintTokens(0)
		scanner.slow = true
		e.Advance(time.Now().Add(time.Millisecond))
		if e.work == nil || e.work.at == 0 || !e.Pending() {
			t.Fatal("fixture failed to suspend partial analysis")
		}
		tokens, current, ok := e.PaintTokens(0)
		if !ok || current != version || kindAt(tokens, m.Len()-8) != language.KindString {
			t.Fatal("partial work cleared old tail", tokens)
		}
		strict, _, ok := e.Tokens(0)
		if !ok || kindAt(strict, m.Len()-8) != language.KindPlain {
			t.Fatal("old display tail leaked as semantic tokens")
		}
		checkPaintPayload(t, e)
		scanner.slow = false
	}
	drain(t, e)
	checkPaintPayload(t, e)
	_, version, _ := e.PaintTokens(0)
	if version == 0 {
		t.Fatal("missing published version")
	}
	e.discard(e.Line(1))
	e.Ensure(1)
	drain(t, e)
	checkPaintPayload(t, e)
	m.SetText("true")
	drain(t, e)
	checkPaintPayload(t, e)
}

// 更早的缓存重载不能丢掉已暂停的后续分析；旧快照和工作缓冲共同计入预算。
func TestPaintWorkRewindAndBudget(t *testing.T) {
	base, _ := language.Builtin(language.JSON)
	scanner := &pacedScanner{Tokenizer: base.Tokenizer()}
	l, err := language.New(language.Definition{Info: language.Info{ID: "paced", Name: "Paced JSON"}, Tokenizer: scanner})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"\"" + strings.Repeat("x", ChunkSize*4) + "\"", "[" + strings.Repeat("0,", maxLineTokens/2) + "0]"} {
		m := textedit.NewModel("true\n" + body)
		e := NewWithLanguage(m, l)
		h := m.ConnectChange(func(c textedit.Change) { e.Change(c) })
		drain(t, e)
		if err := m.ReplaceAtomic(textedit.Range{Start: 15, End: 15}, "1"); err != nil {
			t.Fatal(err)
		}
		scanner.slow = true
		e.Advance(time.Now().Add(time.Millisecond))
		scanner.slow = false
		if e.work == nil {
			t.Fatal("fixture did not leave partial work")
		}
		checkPaintPayload(t, e)
		e.discard(e.Line(0))
		e.Ensure(0)
		checkPaintPayload(t, e)
		drain(t, e)
		got, _, ready := e.Tokens(1)
		if !ready || !e.Line(1).complete || len(got) == 0 {
			t.Fatal("rewind lost incomplete work")
		}
		fresh := NewWithLanguage(m, l)
		drain(t, fresh)
		want, _, _ := fresh.Tokens(1)
		if !reflect.DeepEqual(got, want) {
			t.Fatal("rewound result differs from full analysis")
		}
		checkPaintPayload(t, e)
		h.Disconnect()
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
	if tokens, _, _ := e.PaintTokens(0); len(tokens) != 0 {
		t.Fatal("same-ID replacement exposed old display tokens")
	}
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

// 单个元数据步骤复用模型行索引，不应按单行 byte 数分批重新读完整文本。
func TestMetadataUsesModelLineIndex(t *testing.T) {
	for _, text := range []string{strings.Repeat("x", 4<<20), "a\n\nb\n", strings.Repeat("a\n", 300)} {
		m := textedit.NewModel(text)
		e := New(m, language.Plain)
		steps := 0
		for e.building {
			if err := e.buildStep(); err != nil {
				t.Fatal(err)
			}
			steps++
		}
		if steps != (m.LineCount()+127)/128 {
			t.Fatalf("steps=%d lines=%d", steps, m.LineCount())
		}
		checkIndex(t, e, m)
	}
}
