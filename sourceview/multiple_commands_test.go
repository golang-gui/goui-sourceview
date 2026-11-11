package sourceview

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
)

func multiFixture(t *testing.T, text string, selections []TextSelection) *SourceView {
	t.Helper()
	v, _ := colorFixture(t)
	focusBracketFixture(t, v)
	v.Document().SetLanguage(language.Plain)
	v.Document().SetText(text)
	v.LayoutVisible(v.viewport, v.offset)
	mustSelections(t, v, selections)
	return v
}
func TestMultipleTypingAndDeleteMerge(t *testing.T) {
	before := []TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 5, Caret: 5}}
	v := multiFixture(t, "abc\ndef", before)
	for _, text := range []string{"x", "中", "y"} {
		v.commit(gui.IMCommit{Text: text})
	}
	drainSelections(t, v)
	if v.Document().Text() != "ax中ybc\ndx中yef" {
		t.Fatal(v.Document().Text())
	}
	if !v.Undo() || v.Document().Text() != "abc\ndef" || !slices.Equal(v.Selections(), before) {
		t.Fatal("typing undo", v.Document().Text(), v.Selections())
	}
	v.Redo()
	want := v.Document().Text()
	after := v.Selections()
	for range 3 {
		v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyBackspace})
	}
	drainSelections(t, v)
	if v.Document().Text() != "abc\ndef" {
		t.Fatal(v.Document().Text())
	}
	if !v.Undo() || v.Document().Text() != want || !slices.Equal(v.Selections(), after) {
		t.Fatal("delete undo", v.Document().Text(), v.Selections())
	}
	v.SetSelections(v.Selections())
	v.commit(gui.IMCommit{Text: "!", Composed: true})
	drainSelections(t, v)
	v.commit(gui.IMCommit{Text: "z"})
	drainSelections(t, v)
	v.Undo()
	if v.Document().Text() != "ax中y!bc\ndx中y!ef" {
		t.Fatal("IME merged", v.Document().Text())
	}
}
func TestMultiplePairsSurroundSkipAndDelete(t *testing.T) {
	v := multiFixture(t, "\n", []TextSelection{{}, {Anchor: 1, Caret: 1}})
	v.Document().SetLanguageDefinition(extensionLanguage(t, language.EditingRules{AutoClosingPairs: []language.Pair{{Open: "(", Close: ")"}}, SurroundingPairs: []language.Pair{{Open: "(", Close: ")"}}, Brackets: []language.Pair{{Open: "(", Close: ")"}}}))
	v.commit(gui.IMCommit{Text: "("})
	drainSelections(t, v)
	if v.Document().Text() != "()\n()" || len(v.pairs) != 2 {
		t.Fatal("pairs", v.Document().Text(), v.pairs)
	}
	v.commit(gui.IMCommit{Text: ")"})
	drainSelections(t, v)
	if v.Document().Text() != "()\n()" || len(v.pairs) != 0 || !slices.Equal(v.Selections(), []TextSelection{{Anchor: 2, Caret: 2}, {Anchor: 5, Caret: 5}}) {
		t.Fatal("skip", v.Document().Text(), v.Selections(), v.pairs)
	}
	v.Undo()
	if v.Document().Text() != "\n" {
		t.Fatal("skip created history")
	}
	v.commit(gui.IMCommit{Text: "("})
	drainSelections(t, v)
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyBackspace})
	drainSelections(t, v)
	if v.Document().Text() != "\n" {
		t.Fatal("pair delete", v.Document().Text())
	}
	v.Document().SetText("ab\ncd")
	mustSelections(t, v, []TextSelection{{Anchor: 2, Caret: 0}, {Anchor: 3, Caret: 5}})
	v.commit(gui.IMCommit{Text: "("})
	drainSelections(t, v)
	if v.Document().Text() != "(ab)\n(cd)" || !slices.Equal(v.Selections(), []TextSelection{{Anchor: 3, Caret: 1}, {Anchor: 6, Caret: 8}}) {
		t.Fatal("surround", v.Document().Text(), v.Selections())
	}
}
func TestMultipleLineCommandsDeduplicateAndPreserveCursors(t *testing.T) {
	before := []TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 2, Caret: 2}, {Anchor: 5, Caret: 5}}
	for _, tc := range []struct {
		name, want string
		run        func(*SourceView) error
	}{
		{"move", "ccc\naaa\nbbb\nddd", func(v *SourceView) error { return v.MoveLines(true) }},
		{"duplicate", "aaa\nbbb\naaa\nbbb\nccc\nddd", func(v *SourceView) error { return v.DuplicateLines(true) }},
		{"delete", "ccc\nddd", func(v *SourceView) error { return v.DeleteLines() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := multiFixture(t, "aaa\nbbb\nccc\nddd", before)
			if err := tc.run(v); err != nil || v.Document().Text() != tc.want {
				t.Fatal(err, v.Document().Text())
			}
			if !v.Undo() || v.Document().Text() != "aaa\nbbb\nccc\nddd" || !slices.Equal(v.Selections(), before) {
				t.Fatal("undo", v.Selections())
			}
		})
	}
}
func TestMultipleStructuralTrailAndWords(t *testing.T) {
	before := []TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 3, Caret: 3}, {Anchor: 10, Caret: 10}}
	v := multiFixture(t, "hello fooBar", before)
	if !v.ExpandSelection() || len(v.Selections()) != 2 {
		t.Fatal("expand", v.Selections())
	}
	if !v.ShrinkSelection() || !slices.Equal(v.Selections(), before) {
		t.Fatal("trail lost merged cursor", v.Selections())
	}
	v.MoveWordPart(true, false)
	if !slices.Equal(v.Selections(), []TextSelection{{Anchor: 5, Caret: 5}, {Anchor: 12, Caret: 12}}) {
		t.Fatal(v.Selections())
	}
}
func TestMultipleCleanupPlansAndComments(t *testing.T) {
	before := []cursorState{{selection: TextSelection{Anchor: 1, Caret: 1}}, {selection: TextSelection{Anchor: 5, Caret: 6}}}
	edits, after, err := multipleCleanupPlan(context.Background(), "a \nb \nc ", before, "trim")
	if err != nil {
		t.Fatal(err)
	}
	v := multiFixture(t, "a \nb \nc ", []TextSelection{before[0].selection, before[1].selection})
	if err = v.applyMultiple(edits, after); err != nil || v.Document().Text() != "a\nb\nc" {
		t.Fatal(err, v.Document().Text())
	}
	v.Document().SetLanguageDefinition(extensionLanguage(t, language.EditingRules{Comments: language.Comments{Line: "//", Block: language.Pair{Open: "/*", Close: "*/"}}}))
	mustSelections(t, v, []TextSelection{{Anchor: 0, Caret: 0}, {Anchor: 2, Caret: 2}})
	if err = v.ToggleLineComment(); err != nil || v.Document().Text() != "// a\n// b\nc" {
		t.Fatal(err, v.Document().Text())
	}
	if err = v.ToggleLineComment(); err != nil || v.Document().Text() != "a\nb\nc" {
		t.Fatal(err, v.Document().Text())
	}
}
func TestMultipleNewlinePolicyReentryAndSmartIndent(t *testing.T) {
	v := multiFixture(t, "- item\n- item", []TextSelection{{Anchor: 6, Caret: 6}, {Anchor: 13, Caret: 13}})
	v.Document().SetLanguageDefinition(extensionLanguage(t, language.EditingRules{Newline: func(c language.NewlineContext) (language.NewlineEdit, bool) {
		return language.NewlineEdit{Text: "\n- ", Caret: 3}, true
	}}))
	if err := v.newline(); err != nil || v.Document().Text() != "- item\n- \n- item\n- " {
		t.Fatal(err, v.Document().Text())
	}
	v.Undo()
	called := 0
	v.Document().SetLanguageDefinition(extensionLanguage(t, language.EditingRules{Newline: func(c language.NewlineContext) (language.NewlineEdit, bool) {
		called++
		if called == 2 {
			v.SetSelection(TextSelection{})
		}
		return language.NewlineEdit{Text: "\n", Caret: 1}, true
	}}))
	if err := v.newline(); err != ErrOperationChanged || v.Document().Text() != "- item\n- item" {
		t.Fatal("reentry", err, v.Document().Text())
	}
	v.Document().SetText("    a\n    b")
	mustSelections(t, v, []TextSelection{{Anchor: 4, Caret: 4}, {Anchor: 10, Caret: 10}})
	v.SetIndentOptions(IndentOptions{TabWidth: 4, IndentWidth: 4, InsertSpaces: true})
	v.SetSmartBackspace(true)
	if err := v.multipleDelete(false); err != nil || v.Document().Text() != "a\nb" {
		t.Fatal("indent", err, v.Document().Text())
	}
}
func TestMultipleBracketJumpAndFoldPreservesSet(t *testing.T) {
	v := multiFixture(t, "(x)\n(y)\nlast", []TextSelection{{}, {Anchor: 4, Caret: 4}, {Anchor: 10, Caret: 10}})
	v.Document().SetLanguageDefinition(extensionLanguage(t, language.EditingRules{Brackets: []language.Pair{{Open: "(", Close: ")"}}}))
	if !v.JumpToBracket() || !slices.Equal(v.Selections(), []TextSelection{{Anchor: 2, Caret: 2}, {Anchor: 6, Caret: 6}, {Anchor: 10, Caret: 10}}) {
		t.Fatal(v.Selections())
	}
	// A folded secondary endpoint moves to the header without discarding others.
	v.Document().SetText("head\ninside\ntail\nafter")
	mustSelections(t, v, []TextSelection{{Anchor: 18, Caret: 18}, {Anchor: 7, Caret: 7}})
	folds, err := NewFolds(v.Document().Text(), []FoldRange{{StartLine: 0, EndLine: 2}})
	if err != nil {
		t.Fatal(err)
	}
	v.Document().SetFolds(v.Document().Revision(), folds)
	for i := 0; i < 100 && v.foldIntake != nil; i++ {
		v.foldingIntakeStep()
	}
	if !v.Fold(0) {
		t.Fatal("fold")
	}
	if len(v.Selections()) != 2 || v.Selection().Caret != 18 || v.Selections()[1].Caret != 4 {
		t.Fatal(v.Selections())
	}
}

func TestMultipleLineSelectionsAndResultOffsets(t *testing.T) {
	before := []TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 2, Caret: 2}, {Anchor: 5, Caret: 5}}
	for _, tc := range []struct {
		kind string
		want []TextSelection
	}{
		{"move", []TextSelection{{Anchor: 5, Caret: 5}, {Anchor: 6, Caret: 6}, {Anchor: 9, Caret: 9}}},
		{"duplicate", []TextSelection{{Anchor: 9, Caret: 9}, {Anchor: 10, Caret: 10}, {Anchor: 13, Caret: 13}}},
		{"delete", []TextSelection{{}}},
	} {
		v := multiFixture(t, "aaa\nbbb\nccc\nddd", before)
		if err := v.multipleLineCommand(tc.kind, true); err != nil || !slices.Equal(v.Selections(), tc.want) {
			t.Fatal(tc.kind, err, v.Selections())
		}
	}
	v := multiFixture(t, "one\ntwo\nthree", []TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 5, Caret: 5}})
	if err := v.InsertLine(true); err != nil || v.Document().Text() != "one\n\ntwo\n\nthree" {
		t.Fatal(err, v.Document().Text())
	}
}
func TestMultipleLinkedTagsAndBlockComments(t *testing.T) {
	v := multiFixture(t, "<a></a>\n<b></b>", []TextSelection{{Anchor: 2, Caret: 2}, {Anchor: 10, Caret: 10}})
	v.Document().SetLanguage(language.XML)
	v.SetLinkedTags(true)
	for v.document.engine.Pending() {
		v.document.engine.Advance(time.Time{})
	}
	v.commit(gui.IMCommit{Text: "x"})
	drainSelections(t, v)
	if v.Document().Text() != "<ax></ax>\n<bx></bx>" {
		t.Fatal(v.Document().Text())
	}
	v.Undo()
	if v.Document().Text() != "<a></a>\n<b></b>" {
		t.Fatal("linked undo")
	}
	v.Document().SetLanguageDefinition(extensionLanguage(t, language.EditingRules{Comments: language.Comments{Block: language.Pair{Open: "/*", Close: "*/"}}}))
	v.Document().SetText("ab\ncd")
	before := []TextSelection{{Anchor: 2, Caret: 0}, {Anchor: 3, Caret: 5}}
	mustSelections(t, v, before)
	if err := v.ToggleBlockComment(); err != nil || v.Document().Text() != "/*ab*/\n/*cd*/" {
		t.Fatal(err, v.Document().Text())
	}
	v.Undo()
	if !slices.Equal(v.Selections(), before) {
		t.Fatal(v.Selections())
	}
}
func TestMultipleCrossParagraphClusterEndpoints(t *testing.T) {
	v := multiFixture(t, "e\u0301\nx\ne\u0301\ny", []TextSelection{{Anchor: 1, Caret: 5}, {Anchor: 7, Caret: 11}})
	v.commit(gui.IMCommit{Text: "Z", Composed: true})
	drainSelections(t, v)
	if v.Document().Text() != "Z\nZ" {
		t.Fatal("anchor cluster not snapped", v.Document().Text())
	}
}
func TestMultipleDirectionalDeletionAndReadOnly(t *testing.T) {
	v := multiFixture(t, "fooBar baz\nfooBar baz", []TextSelection{{Anchor: 3, Caret: 3}, {Anchor: 14, Caret: 14}})
	if err := v.DeleteWordPart(true); err != nil || v.Document().Text() != "foo baz\nfoo baz" {
		t.Fatal(err, v.Document().Text())
	}
	v.Undo()
	if err := v.DeleteWord(true); err != nil || v.Document().Text() != "foo baz\nfoo baz" {
		t.Fatal(err, v.Document().Text())
	}
	v.Undo()
	v.SetReadOnly(true)
	before := v.Document().Text()
	if err := v.DeleteWord(true); err != ErrReadOnly || v.Document().Text() != before {
		t.Fatal(err)
	}
}

func TestMultiplePolicyOptionsAndReentrantInput(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(map[bool]string{false: "option", true: "new-input"}[queued], func(t *testing.T) {
			v := multiFixture(t, "\n", []TextSelection{{}, {Anchor: 1, Caret: 1}})
			calls := 0
			rules := language.EditingRules{AutoClosingPairs: []language.Pair{{Open: "(", Close: ")"}}, AllowPair: func(language.PairContext) bool {
				calls++
				if calls == 2 {
					if queued {
						v.SetSelection(TextSelection{})
						v.commit(gui.IMCommit{Text: "X", Composed: true})
					} else {
						v.SetAutoPairs(false)
					}
				}
				return true
			}}
			v.Document().SetLanguageDefinition(extensionLanguage(t, rules))
			v.commit(gui.IMCommit{Text: "("})
			drainSelections(t, v)
			if queued {
				drainSelections(t, v)
				if v.Document().Text() != "X\n" {
					t.Fatal("lost newly queued input", v.Document().Text())
				}
			} else if v.Document().Text() != "\n" || len(v.pairs) != 0 {
				t.Fatal("mixed option plans committed", v.Document().Text())
			}
		})
	}
}

func TestMultipleExplicitTagEndpointsDoNotMirrorTwice(t *testing.T) {
	v := multiFixture(t, "<a></a>", []TextSelection{{Anchor: 2, Caret: 2}, {Anchor: 6, Caret: 6}})
	v.Document().SetLanguage(language.XML)
	for v.document.engine.Pending() {
		v.document.engine.Advance(time.Time{})
	}
	v.commit(gui.IMCommit{Text: "x"})
	drainSelections(t, v)
	if v.Document().Text() != "<ax></ax>" {
		t.Fatal(v.Document().Text())
	}
}

func TestMultipleReindentedPasteKnownAndUnknownContexts(t *testing.T) {
	v := multiFixture(t, "  \n    ", []TextSelection{{Anchor: 2, Caret: 2}, {Anchor: 7, Caret: 7}})
	v.Document().SetLanguage(language.JSON)
	// No analysis turn has resolved the predecessor of row 1. Row 0 is known;
	// row 1 must use literal paste, independently of event-loop scheduling.
	if _, known := v.lexicalContext(2); !known {
		t.Fatal("row 0 context")
	}
	if _, known := v.lexicalContext(7); known {
		t.Fatal("predecessor unexpectedly resolved")
	}
	plan := v.reindentedPastePlanner("{\n  \"a\":1\n}")
	edits, after, err := plan(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err = v.applyMultiple(edits, after); err != nil || v.Document().Text() != "  {\n    \"a\":1\n  }\n    {\n  \"a\":1\n}" || !slices.Equal(v.Selections(), []TextSelection{{Anchor: 17, Caret: 17}, {Anchor: 33, Caret: 33}}) {
		t.Fatal("unknown literal fallback", err, v.Document().Text(), v.Selections())
	}
	v.Undo()
	for v.document.engine.Pending() {
		v.document.engine.Advance(time.Time{})
	}
	plan = v.reindentedPastePlanner("{\n  \"a\":1\n}")
	edits, after, err = plan(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err = v.applyMultiple(edits, after); err != nil || v.Document().Text() != "  {\n    \"a\":1\n  }\n    {\n      \"a\":1\n    }" {
		t.Fatal("known shifted paste", err, v.Document().Text())
	}
}
