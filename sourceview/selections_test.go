package sourceview

import (
	"errors"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/typography"
	"image/color"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/golang-gui/goui/platform/events"
)

func TestSelectionSetValidationOwnershipAndSignals(t *testing.T) {
	v := NewSourceView()
	v.Document().SetText("a中bc\nxyz")
	var sets [][]TextSelection
	primary := 0
	h := v.ConnectSelections(func(s []TextSelection) { sets = append(sets, s); s[0].Caret = 99 })
	defer h.Disconnect()
	h2 := v.ConnectSelection(func(TextSelection) { primary++ })
	defer h2.Disconnect()
	in := []TextSelection{{Anchor: 8, Caret: 6}, {Anchor: 0, Caret: 1}, {Anchor: 0, Caret: 0}}
	if err := v.SetSelections(in); err != nil {
		t.Fatal(err)
	}
	want := []TextSelection{{Anchor: 8, Caret: 6}, {Anchor: 0, Caret: 1}}
	in[0].Caret = 0
	if !slices.Equal(v.Selections(), want) || len(sets) != 1 || primary != 1 {
		t.Fatal(v.Selections(), sets, primary)
	}
	got := v.Selections()
	got[0].Caret = 0
	for _, bad := range [][]TextSelection{nil, {{Anchor: 2, Caret: 3}}, make([]TextSelection, maxSelections+1)} {
		if err := v.SetSelections(bad); err == nil || !slices.Equal(v.Selections(), want) {
			t.Fatal("invalid input changed set", err)
		}
	}
	v.SetSelection(v.Selection())
	if len(v.Selections()) != 1 || primary != 1 || len(sets) != 2 {
		t.Fatal("scalar setter did not collapse only extras", primary, sets)
	}
}
func TestMultipleReplacementHistoryAndSharedMapping(t *testing.T) {
	a, b := NewSourceView(), NewSourceView()
	d := NewDocument("one two three")
	a.SetDocument(d)
	b.SetDocument(d)
	observe(t, a)
	observe(t, b)
	before := []TextSelection{{Anchor: 7, Caret: 4}, {Anchor: 0, Caret: 3}}
	mustSelections(t, a, before)
	mustSelections(t, b, []TextSelection{{Anchor: 8, Caret: 8}, {Anchor: 13, Caret: 13}})
	notifications := 0
	a.ConnectSelections(func([]TextSelection) { notifications++ })
	if err := a.ReplaceSelection("X"); err != nil {
		t.Fatal(err)
	}
	want := []TextSelection{{Anchor: 3, Caret: 3}, {Anchor: 1, Caret: 1}}
	if d.Text() != "X X three" || !slices.Equal(a.Selections(), want) || notifications != 1 || !slices.Equal(b.Selections(), []TextSelection{{Anchor: 4, Caret: 4}, {Anchor: 9, Caret: 9}}) {
		t.Fatal(d.Text(), a.Selections(), b.Selections(), notifications)
	}
	if !a.Undo() || d.Text() != "one two three" || !slices.Equal(a.Selections(), before) {
		t.Fatal("undo", d.Text(), a.Selections())
	}
	if !b.Redo() || d.Text() != "X X three" || !slices.Equal(b.Selections(), want) {
		t.Fatal("shared redo", d.Text(), b.Selections())
	}
}
func mustSelections(t *testing.T, v *SourceView, s []TextSelection) {
	t.Helper()
	if err := v.SetSelections(s); err != nil {
		t.Fatal(err)
	}
}
func TestMultipleDeleteUnionAndPasteLines(t *testing.T) {
	v := NewSourceView()
	v.Document().SetText("ab\ncd\nef")
	observe(t, v)
	mustSelections(t, v, []TextSelection{{Anchor: 0, Caret: 0}, {Anchor: 3, Caret: 3}, {Anchor: 6, Caret: 6}})
	if err := v.multiplePaste("X\nY\n"); err != nil {
		t.Fatal(err)
	}
	if v.Document().Text() != "Xab\nYcd\nef" {
		t.Fatal(v.Document().Text())
	}
	if !v.Undo() {
		t.Fatal("undo")
	}
	if err := v.multiplePaste("x\n"); err != nil {
		t.Fatal(err)
	}
	if v.Document().Text() != "x\nab\nx\ncd\nx\nef" {
		t.Fatal("tail newline discarded", v.Document().Text())
	}
	v.Document().SetText("abc")
	mustSelections(t, v, []TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 2, Caret: 2}})
	if err := v.multipleDelete(false); err != nil || v.Document().Text() != "c" || len(v.Selections()) != 1 {
		t.Fatal(err, v.Document().Text(), v.Selections())
	}
}
func TestMultipleReentrantChangesCancelOldResult(t *testing.T) {
	v := NewSourceView()
	v.Document().SetText("abc def")
	observe(t, v)
	mustSelections(t, v, []TextSelection{{Anchor: 0, Caret: 0}, {Anchor: 4, Caret: 4}})
	once := false
	v.ConnectSelections(func([]TextSelection) {
		if !once {
			once = true
			v.SetSelection(TextSelection{Anchor: 2, Caret: 2})
		}
	})
	if err := v.ReplaceSelection("!"); err != nil || v.Selection().Caret != 2 || len(v.Selections()) != 1 {
		t.Fatal(err, v.Selections())
	}
}
func TestMultipleIMEAndDeferredNavigation(t *testing.T) {
	v, _ := colorFixture(t)
	focusBracketFixture(t, v)
	v.Document().SetText("abcd\nefgh")
	v.LayoutVisible(v.viewport, v.offset)
	mustSelections(t, v, []TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 6, Caret: 6}})
	v.applyPreedit("中", len("中"))
	if v.projection == nil || v.Document().Text() != "abcd\nefgh" {
		t.Fatal("preedit changed document")
	}
	v.applyCommit(guiCommit("中"))
	if v.Document().Text() != "a中bcd\ne中fgh" || len(v.Selections()) != 2 {
		t.Fatal(v.Document().Text(), v.Selections())
	}
	if !v.Undo() || v.Document().Text() != "abcd\nefgh" {
		t.Fatal("IME undo")
	}
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowRight})
	for i := 0; i < 20 && v.pendingInput != nil; i++ {
		v.inputStep(time.Time{})
	}
	if !reflect.DeepEqual(v.Selections(), []TextSelection{{Anchor: 2, Caret: 2}, {Anchor: 7, Caret: 7}}) {
		t.Fatal(v.Selections())
	}
}
func guiCommit(text string) gui.IMCommit { return gui.IMCommit{Text: text, Composed: true} }
func TestMultipleLimitAndAtomicEdits(t *testing.T) {
	v := NewSourceView()
	v.Document().SetText("abc")
	observe(t, v)
	before := v.Selections()
	rev := v.Document().Revision()
	err := v.ApplyEditsWithSelections(v.Document(), rev, []TextEdit{{Range: TextRange{}, Text: "中"}}, []TextSelection{{Anchor: 1, Caret: 1}})
	if err == nil || v.Document().Revision() != rev || !slices.Equal(v.Selections(), before) {
		t.Fatal("invalid result mutated")
	}
	if !errors.Is(v.SetSelections(make([]TextSelection, 4097)), ErrOperationLimit) {
		t.Fatal("missing limit")
	}
}

func TestMultipleInputBeyondPageCacheAndCancellation(t *testing.T) {
	v := pendingInputFixture(t, "abc")
	cursors := make([]TextSelection, 32)
	for i := range cursors {
		at := 100 + i*8192
		cursors[i] = TextSelection{Anchor: at, Caret: at}
	}
	mustSelections(t, v, cursors)
	original := v.Document().Text()
	v.commit(gui.IMCommit{Text: "X"})
	if v.pendingInput == nil || v.Document().Text() != original {
		t.Fatal("multi input bypassed atomic preparation")
	}
	for i := 0; i < 1500 && v.pendingInput != nil; i++ {
		v.inputStep(time.Time{})
	}
	if v.pendingInput != nil {
		t.Fatal("dispersed pages failed to converge")
	}
	want := original
	for i := len(cursors) - 1; i >= 0; i-- {
		at := cursors[i].Caret
		want = want[:at] + "X" + want[at:]
	}
	if v.Document().Text() != want || len(v.Selections()) != 32 {
		t.Fatal("not all dispersed inputs applied")
	}
	if !v.Undo() || v.Document().Text() != original || !slices.Equal(v.Selections(), cursors) {
		t.Fatal("dispersed undo")
	}
	v.commit(gui.IMCommit{Text: "Y"})
	v.SetSelection(TextSelection{})
	v.inputStep(time.Time{})
	if v.pendingInput != nil || v.Document().Text() != original {
		t.Fatal("cancelled dispersed input escaped")
	}
}
func TestMultipleOpaqueForegroundAndRestore(t *testing.T) {
	v, _ := colorFixture(t)
	focusBracketFixture(t, v)
	v.Document().SetText("abcdef")
	colors := v.colors
	colors.SelectionForeground = color.NRGBA{R: 200, G: 20, B: 100, A: 255}
	v.colors = colors
	selections := []TextSelection{{Anchor: 0, Caret: 1}, {Anchor: 3, Caret: 5}}
	mustSelections(t, v, selections)
	p := v.paragraph(0)
	if p == nil {
		t.Fatal("paragraph")
	}
	l := p.Layout.(*shapedLayout)
	for i, c := range l.colors {
		selected := i == 0 || i == 3 || i == 4
		if (c == colors.SelectionForeground) != selected {
			t.Fatal("selection foreground", i, c)
		}
	}
	v.upstream = true
	v.additional[0].upstream = true
	state, err := v.SaveState()
	if err != nil {
		t.Fatal(err)
	}
	v.SetSelection(TextSelection{Anchor: 2, Caret: 2})
	if err := v.RestoreState(state); err != nil || !slices.Equal(v.Selections(), selections) || !v.upstream || !v.additional[0].upstream {
		t.Fatal(err, v.Selections())
	}
}

type failedSelectionContext struct{ typography.Context }

func (*failedSelectionContext) NewTextLayout(string, typography.TextFormat, float32, float32) (typography.TextLayout, error) {
	return nil, errors.New("test native failure")
}
func TestMultipleNativeFailureCancelsOnce(t *testing.T) {
	v := NewSourceView()
	v.Document().SetText("abcdef")
	observe(t, v)
	focusBracketFixture(t, v)
	mustSelections(t, v, []TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 4, Caret: 4}})
	v.context = &failedSelectionContext{}
	v.frameLayout = true
	reported := 0
	v.ConnectError(func(error) { reported++ })
	v.commit(gui.IMCommit{Text: "x"})
	v.inputStep(time.Time{})
	if v.pendingInput != nil || v.Document().Text() != "abcdef" || reported != 1 {
		t.Fatal("failed work retained or duplicated error", v.pendingInput, reported)
	}
}
