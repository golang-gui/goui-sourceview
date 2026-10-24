package sourceview

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

func TestSmartIndentBackspaceIsOneUndo(t *testing.T) {
	for _, tc := range []struct {
		before, after string
		caret         int
	}{{"\t  value", "\tvalue", 1}, {"      value", "    value", 4}, {"\tvalue", "    value", 4}} {
		v := NewSourceView()
		v.SetFoldingEnabled(false)
		v.document.model.SetText(tc.before)
		observe(t, v)
		options := v.IndentOptions()
		options.TabWidth = 8
		options.IndentWidth = 4
		_ = v.SetIndentOptions(options)
		n, _ := leadingColumns(tc.before, 8)
		v.SetSelection(textedit.Selection{Anchor: n, Caret: n})
		ok, err := v.deleteIndent()
		if !ok || err != nil || v.document.model.Text() != tc.after || v.Selection().Caret != tc.caret {
			t.Fatal(tc, v.document.model.Text(), v.Selection(), err)
		}
		if ok := v.Undo(); !ok || v.document.model.Text() != tc.before || v.Selection().Caret != n {
			t.Fatal("undo", ok, v.document.model.Text(), v.Selection())
		}
	}
}

func TestSmartEdgesAndShift(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("    word  ")
	observe(t, v)
	v.context = &shapedContext{}
	t.Cleanup(v.dropLayouts)
	v.SetSelection(textedit.Selection{Anchor: 6, Caret: 6})
	for _, tc := range []struct {
		key           events.Key
		shift         bool
		anchor, caret int
	}{{events.KeyHome, false, 4, 4}, {events.KeyHome, false, 0, 0}, {events.KeyEnd, true, 0, 8}, {events.KeyEnd, true, 0, 10}} {
		v.navigate(tc.key, tc.shift)
		if s := v.Selection(); s.Anchor != tc.anchor || s.Caret != tc.caret {
			t.Fatal(tc, s)
		}
	}
}

func TestAssistancePaintDoesNotChangeGeometry(t *testing.T) {
	v := NewSourceView()
	v.SetFoldingEnabled(false)
	v.document.model.SetText(" \tx \n")
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	p := v.paragraph(0)
	created := ctx.created
	for _, tc := range []struct {
		mode        WhitespaceMode
		end         bool
		dots, lines int
	}{{WhitespaceAll, true, 2, 7}, {WhitespaceTrailing, false, 1, 0}, {WhitespaceNone, true, 0, 4}, {WhitespaceSelection, false, 1, 0}} {
		_ = v.SetWhitespaceMode(tc.mode)
		v.SetShowLineEndings(tc.end)
		v.SetSelection(textedit.Selection{Caret: 1})
		out := &recordingPainter{}
		v.paintWhitespace(out, 0, p, geometry.Point{})
		if len(out.fills) != tc.dots || out.lines != tc.lines {
			t.Fatal(tc, len(out.fills), out.lines)
		}
		if v.paragraph(0) != p || ctx.created != created {
			t.Fatal("display options reshaped text")
		}
	}
	_ = v.SetRulers([]int{80, 40, 80})
	copy := v.Rulers()
	copy[0] = 1
	if !reflect.DeepEqual(v.Rulers(), []int{40, 80}) {
		t.Fatal("rulers alias or normalization")
	}
	if v.SetRulers([]int{-1}) == nil || v.SetWhitespaceMode(99) == nil {
		t.Fatal("invalid options accepted")
	}
}

// 标记表示实际逻辑换行；CRLF 只显示一次，EOF 的空行不产生额外标记。
func TestCRLFEndMarkersPreserveTerminalNewlines(t *testing.T) {
	for _, tc := range []struct {
		text    string
		markers int
	}{
		{"", 0}, {"abc", 0}, {"abc\r\n", 1}, {"abc\r\n\r\n", 2}, {"\r\n", 1},
	} {
		t.Run(tc.text, func(t *testing.T) {
			v := NewSourceView()
			v.SetDocument(NewDocument(tc.text))
			v.SetFoldingEnabled(false)
			v.SetShowLineEndings(true)
			v.context = &shapedContext{}
			t.Cleanup(v.dropLayouts)
			markers := 0
			for row := 0; row < v.Document().Model().LineCount(); row++ {
				p := v.paragraph(row)
				if p == nil {
					t.Fatal("missing paragraph")
				}
				out := &recordingPainter{}
				v.paintWhitespace(out, row, p, geometry.Point{})
				if out.lines > 0 {
					markers++
				}
			}
			if markers != tc.markers {
				t.Fatal("logical line-ending markers", markers, tc.markers)
			}
		})
	}
}

func TestOccurrenceRequiresWholeSelectedWord(t *testing.T) {
	v := NewSourceView()
	v.SetFoldingEnabled(false)
	v.document.model.SetText("foo food foo")
	observe(t, v)
	v.context = &shapedContext{}
	t.Cleanup(v.dropLayouts)
	v.SetSelection(textedit.Selection{Caret: 3})
	v.SetSearch(SearchQuery{Text: "food"})
	word := v.occurrenceWord()
	if word != "foo" {
		t.Fatal(word)
	}
	out := &recordingPainter{}
	v.SetShowIndentGuides(false)
	// Painting now culls ranges outside the arranged viewport.
	v.LayoutVisible(geometry.Size{Width: 320, Height: 120}, geometry.Point{})
	v.paintAssistanceBackground(out, 0, v.paragraph(0), geometry.Point{}, word)
	if len(out.fills) != 1 || out.fills[0].rect.X != 90 || out.fills[0].rect.Width != 30 || v.Search().Text != "food" {
		t.Fatal(out.fills, v.Search())
	}
	v.SetSelection(textedit.Selection{Anchor: 4, Caret: 7})
	if v.occurrenceWord() != "" {
		t.Fatal("partial word highlighted")
	}
}

func TestOccurrenceWordDoesNotMeasureOffscreenSelection(t *testing.T) {
	for _, tc := range []struct {
		text       string
		start, end int
		want       string
	}{
		{"foo food foo", 0, 3, "foo"},
		{"foo food foo", 4, 7, ""},
		{"中 foo 文", 4, 7, "foo"},
		{"中foo文", 3, 6, ""},
	} {
		v := NewSourceView()
		v.SetFoldingEnabled(false)
		v.document.model.SetText(tc.text)
		observe(t, v)
		ctx := &shapedContext{}
		v.context = ctx
		v.SetSelection(textedit.Selection{Anchor: tc.start, Caret: tc.end})
		v.dropLayouts() // The selection's paragraph was evicted after scrolling.
		before := ctx.created
		got := v.occurrenceWord()
		v.dropLayouts()
		if got != tc.want || ctx.created != before || ctx.created != ctx.destroyed {
			t.Fatalf("%q: word=%q, native calls=%d, resources=%d/%d", tc.text, got, ctx.created-before, ctx.created, ctx.destroyed)
		}
	}
}

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

func TestConfigurationAndShellQuoteTyping(t *testing.T) {
	for _, tc := range []struct {
		id                   language.ID
		initial, typed, want string
	}{
		{language.TOML, "text = ", `"""`, `text = """`},
		{language.TOML, "text = ", `'''`, `text = '''`},
		{language.TOML, "text = ", "'\\'", "text = '\\'"},
		{language.YAML, "text: ", "'it''s'", "text: 'it''s'"},
		{language.Shell, "echo ", "'\\'", "echo '\\'"},
	} {
		t.Run(string(tc.id)+tc.typed, func(t *testing.T) {
			v := NewSourceView()
			v.document.model.SetText(tc.initial)
			_ = v.document.SetLanguage(tc.id)
			observe(t, v)
			v.SetSelection(textedit.Selection{Anchor: len(tc.initial), Caret: len(tc.initial)})
			for _, c := range tc.typed {
				v.document.engine.Advance(time.Time{})
				handled, err := v.pairText(v.Selection().Range(), string(c))
				if err != nil {
					t.Fatal(err)
				}
				if !handled {
					if err := v.ReplaceSelection(string(c)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if got := v.document.model.Text(); got != tc.want || v.Selection().Caret != len(tc.want) {
				t.Fatalf("got %q caret %d; want %q", got, v.Selection().Caret, tc.want)
			}
		})
	}
}

func TestConfigurationAndShellBracketContexts(t *testing.T) {
	for _, tc := range []struct {
		id          language.ID
		text        string
		open, close byte
		pair        bool
	}{
		{language.TOML, `a = ["[", "]"]`, '[', ']', true},
		{language.YAML, `a: ["]", {b: 1}]`, '[', ']', true},
		{language.Shell, `echo "$(printf ')')"`, '(', ')', true},
		{language.YAML, "a: |\n  [ ]\nnext: x", '[', ']', false},
		{language.Shell, "cat <<END\n[ ]\nEND", '[', ']', false},
	} {
		v := NewSourceView()
		v.document.model.SetText(tc.text)
		_ = v.document.SetLanguage(tc.id)
		observe(t, v)
		v.document.engine.Advance(time.Time{})
		at := strings.IndexByte(tc.text, tc.open)
		v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
		got := v.matchingBrackets()
		if !tc.pair {
			if len(got) != 0 {
				t.Fatalf("%s matched text block: %v", tc.id, got)
			}
			continue
		}
		end := strings.LastIndexByte(tc.text, tc.close)
		if len(got) != 2 || got[0] != (textedit.Range{Start: at, End: at + 1}) || got[1] != (textedit.Range{Start: end, End: end + 1}) {
			t.Fatalf("%s pair: %v", tc.id, got)
		}
	}
}

func TestNonBracketCaretDoesNotWaitForSearchWindowLexing(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("plain text\n" + strings.Repeat("<item>value</item>\n", 10000))
	if err := v.Document().SetLanguage(language.XML); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	if v.Document().engine.Ready() {
		t.Fatal("fixture unexpectedly finished indexing")
	}
	if len(v.matchingBrackets()) != 0 || !v.brackets.valid {
		t.Fatal("ordinary caret demanded unresolved search-window tokens")
	}
	if v.Document().engine.ScannedLines() != 0 {
		t.Fatal("matching ordinary text forced synchronous analysis")
	}
}

func TestBracketCandidateHandlesSequenceEdgesAndUTF8(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("字««x»»z")
	pairs := []language.Pair{{Open: "««", Close: "»»"}}
	for _, tc := range []struct {
		at   int
		want bool
	}{{0, false}, {3, true}, {5, true}, {7, true}, {8, true}, {10, true}, {12, true}, {13, false}} {
		if got := len(v.bracketCandidates(tc.at, pairs)) > 0; got != tc.want {
			t.Fatal("candidate mismatch", tc, got)
		}
	}
	// Preserve actual longest-sequence matching inside a custom delimiter.
	l, err := language.New(language.Definition{Info: language.Info{ID: "custom", Name: "Custom"}, Editing: language.EditingRules{Brackets: pairs}})
	if err != nil {
		t.Fatal(err)
	}
	v.Document().SetLanguageDefinition(l)
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 5, Caret: 5})
	r := v.matchingBrackets()
	if len(r) != 2 || r[0] != (textedit.Range{Start: 3, End: 7}) || r[1] != (textedit.Range{Start: 8, End: 12}) {
		t.Fatal("candidate check changed custom matching", r)
	}
}

type slowMarkupTokenizer struct{ language.Tokenizer }

func (s slowMarkupTokenizer) Scan(in language.Input, state language.State) (language.Result, error) {
	// Make a single scan exceed the fixture's soft deadline. This establishes
	// an exact partially analyzed state without depending on scanner speed.
	time.Sleep(2 * time.Millisecond)
	return s.Tokenizer.Scan(in, state)
}

func TestNonStructuralCandidateOnlyNeedsItsOwnLexicalLine(t *testing.T) {
	v := NewSourceView()
	xml, _ := language.Builtin(language.XML)
	d := xml.Definition()
	d.Tokenizer = slowMarkupTokenizer{d.Tokenizer}
	l, err := language.New(d)
	if err != nil {
		t.Fatal(err)
	}
	v.Document().SetLanguageDefinition(l)
	v.Document().Model().SetText("<?xml version=\"1.0\"?>\n" + strings.Repeat("<item>value</item>\n", 100))
	observe(t, v)
	e := v.Document().engine
	for {
		e.Advance(time.Now().Add(time.Millisecond))
		if _, _, ready := e.Tokens(0); ready {
			break
		}
	}
	if _, _, ready := e.Tokens(1); ready {
		t.Fatal("fixture analyzed more than the declaration")
	}
	if len(v.matchingBrackets()) != 0 || !v.brackets.valid {
		t.Fatal("non-structural '<' demanded tokens from unrelated search-window lines")
	}
}

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

func TestTagMatchingCachesNegativeResultWhileLexing(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("[<?xml version=\"1.0\"?>\n" + strings.Repeat("<item>value</item>\n", 10000))
	if err := v.Document().SetLanguage(language.XML); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	if ranges := v.matchingBrackets(); len(ranges) != 0 || !v.tagMatches.valid || v.brackets.valid {
		t.Fatal("unresolved bracket lexing did not retain its independent negative tag result", ranges)
	}
	cache := v.tagMatches
	for range 10 {
		if len(v.matchingBrackets()) != 0 || !v.tagMatches.valid || v.tagMatches.document != cache.document || v.tagMatches.revision != cache.revision || v.tagMatches.caret != cache.caret {
			t.Fatal("unchanged matching request lost its negative cache")
		}
	}
	v.Document().engine.Advance(time.Time{})
	if len(v.matchingBrackets()) != 0 || !v.brackets.valid {
		t.Fatal("tag cache prevented the bracket scanner from completing")
	}
}

func TestTagMatchCacheInvalidatesForDocumentPositionAndLanguage(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("<root><a></a><b></b></root>")
	if err := v.Document().SetLanguage(language.XML); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	check := func(at, opening, closing int) {
		t.Helper()
		v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
		r := v.matchingTags()
		if len(r) != 2 || r[0] != (textedit.Range{Start: opening, End: opening + 1}) || r[1] != (textedit.Range{Start: closing, End: closing + 1}) {
			t.Fatal("incorrect matched pair", r)
		}
		if got := v.matchingTags(); &got[0] != &r[0] {
			t.Fatal("unchanged tag query reparsed instead of retaining its result")
		}
	}
	check(7, 7, 11)
	check(14, 14, 18)
	if err := v.Document().Model().ReplaceAtomic(textedit.Range{}, "\n"); err != nil {
		t.Fatal(err)
	}
	check(15, 15, 19)
	if err := v.Document().SetLanguage(language.Plain); err != nil {
		t.Fatal(err)
	}
	if len(v.matchingTags()) != 0 {
		t.Fatal("plain language retained markup decoration")
	}
	v.SetDocument(NewDocument("other"))
	if v.tagMatches.valid || v.tagMatches.document != nil || len(v.matchingTags()) != 0 {
		t.Fatal("document replacement retained old markup ownership")
	}
}
