package sourceview

import (
	"slices"
	"strings"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
)

func TestSelectionUndoCompleteSetAndIndependentTextHistory(t *testing.T) {
	v := NewSourceView()
	v.SetDocument(NewDocument("abc\ndef"))
	before := []cursorState{
		{selection: TextSelection{Anchor: 3, Caret: 1}, upstream: true, preferredX: 17, preferredSet: true},
		{selection: TextSelection{Anchor: 5, Caret: 5}},
	}
	v.setCursorStatesKeepingCompletion(before, false, nil)
	v.SetSelection(TextSelection{Anchor: 7, Caret: 7})
	v.SetReadOnly(true)
	if !v.UndoSelection() || !slices.Equal(v.cursors(), before) || v.Document().CanUndo() {
		t.Fatal("selection undo lost complete cursor state or changed text history", v.cursors())
	}
	v.SetReadOnly(false)
	if err := v.ReplaceSelection("x"); err != nil {
		t.Fatal(err)
	}
	if v.UndoSelection() {
		t.Fatal("selection history survived a text revision")
	}
	if !v.Undo() || v.Document().Text() != "abc\ndef" || v.UndoSelection() {
		t.Fatal("text undo did not independently restore its selection", v.Selections())
	}
	if err := v.Document().Replace(TextRange{}, "z"); err != nil {
		t.Fatal(err)
	}
	if v.UndoSelection() {
		t.Fatal("unmounted external edit left stale selection history")
	}
}

func TestSelectionUndoBoundsReentryAndDocumentIsolation(t *testing.T) {
	v := NewSourceView()
	v.SetDocument(NewDocument(strings.Repeat("x", 10000)))
	for i := 1; i <= 100; i++ {
		v.SetSelection(TextSelection{Anchor: i, Caret: i})
	}
	count := 0
	for v.UndoSelection() {
		count++
	}
	if count != 64 || v.Selection().Caret != 36 {
		t.Fatal("unbounded or incorrect selection history", count, v.Selection())
	}
	for n := 0; n < 8; n++ {
		set := make([]TextSelection, maxSelections)
		for i := range set {
			set[i] = TextSelection{Anchor: i*2 + n%2, Caret: i*2 + n%2}
		}
		if err := v.SetSelections(set); err != nil {
			t.Fatal(err)
		}
	}
	if v.cursorUndo.count > maxCursorUndoStates || len(v.cursorUndo.steps) > 64 {
		t.Fatal("cursor snapshots exceeded memory bound")
	}
	h := v.ConnectSelections(func(s []TextSelection) {
		if len(s) > 1 {
			v.SetSelection(TextSelection{Anchor: 9999, Caret: 9999})
		}
	})
	v.UndoSelection()
	h.Disconnect()
	if v.Selection().Caret != 9999 || len(v.Selections()) != 1 {
		t.Fatal("selection undo overwrote a reentrant setter")
	}
	v.SetDocument(NewDocument("other"))
	if v.UndoSelection() {
		t.Fatal("selection history crossed documents")
	}
}

func TestRemoveSelectionPromotesAndRetainsFinalCursor(t *testing.T) {
	v := NewSourceView()
	v.SetDocument(NewDocument("abcdef"))
	set := []TextSelection{{Anchor: 5, Caret: 5}, {Anchor: 1, Caret: 1}, {Anchor: 3, Caret: 3}}
	if err := v.SetSelections(set); err != nil {
		t.Fatal(err)
	}
	if err := v.RemoveSelection(-1); err == nil || !slices.Equal(v.Selections(), set) {
		t.Fatal("invalid index changed the set")
	}
	v.SetReadOnly(true)
	if err := v.RemoveSelection(0); err != nil || !slices.Equal(v.Selections(), set[1:]) {
		t.Fatal("removing primary did not promote first remaining cursor", v.Selections(), err)
	}
	if !v.UndoSelection() || !slices.Equal(v.Selections(), set) {
		t.Fatal("removal was not selection-undoable")
	}
	v.SetSelection(TextSelection{Anchor: 2, Caret: 2})
	if err := v.RemoveSelection(0); err != nil || v.Selection().Caret != 2 || len(v.Selections()) != 1 {
		t.Fatal("removed the final cursor")
	}
}

func TestSkipOccurrenceKeepsOtherMatchesAndWraps(t *testing.T) {
	v, _ := colorFixture(t)
	v.Document().SetText("foo foo foo foo")
	set := []TextSelection{{Anchor: 4, Caret: 7}, {Anchor: 0, Caret: 3}}
	if err := v.SetSelections(set); err != nil {
		t.Fatal(err)
	}
	for _, at := range []int{8, 12, 4} {
		if err := v.SkipNextOccurrence(); err != nil {
			t.Fatal(err)
		}
		drainSelections(t, v)
		if !slices.Equal(v.Selections(), []TextSelection{{Anchor: at, Caret: at + 3}, set[1]}) {
			t.Fatal("skip added a selection or lost another match", v.Selections())
		}
	}
	if !v.UndoSelection() || v.Selection().Anchor != 12 {
		t.Fatal("skip did not record one selection operation")
	}
	v.Document().SetText("foo foo")
	set = []TextSelection{{Anchor: 0, Caret: 3}, {Anchor: 4, Caret: 7}}
	v.SetSelections(set)
	v.SkipNextOccurrence()
	drainSelections(t, v)
	if !slices.Equal(v.Selections(), set) {
		t.Fatal("no available match should preserve the primary")
	}
}

func TestSelectionDragHistoryCoalescesAndStationaryToggleKeepsSet(t *testing.T) {
	v, _ := colorFixture(t)
	v.Document().SetText("abcdef")
	v.SetSelection(TextSelection{Anchor: 1, Caret: 1})
	v.dragging, v.selectionGestureID = true, 7
	for _, end := range []int{2, 3, 4} {
		v.setCursorStatesKeepingCompletion([]cursorState{{selection: TextSelection{Anchor: 1, Caret: end}}}, false, nil)
	}
	v.dragging = false
	if !v.UndoSelection() || v.Selection() != (TextSelection{Anchor: 1, Caret: 1}) {
		t.Fatal("pointer drag recorded intermediate states", v.Selections())
	}
	v.additiveDrag, v.dragRemoveSelection = true, 0
	v.dragStartPoint = geometry.Point{X: 20, Y: 20}
	before := v.Selections()
	v.dragSelection(v.dragStartPoint)
	if !slices.Equal(v.Selections(), before) {
		t.Fatal("stationary toggle changed primary before release")
	}
}
