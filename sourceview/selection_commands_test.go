package sourceview

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
)

func drainSelections(t *testing.T, v *SourceView) {
	t.Helper()
	for i := 0; i < 2000 && v.pendingInput != nil; i++ {
		v.inputStep(time.Time{})
	}
	if v.pendingInput != nil {
		t.Fatal("selection work did not converge")
	}
}
func TestOccurrenceSelectionWrapAndChunkBoundary(t *testing.T) {
	v, _ := colorFixture(t)
	v.Document().SetText("foo bar foo baz foo")
	v.SetSelection(TextSelection{Anchor: 8, Caret: 11})
	if err := v.SelectNextOccurrence(); err != nil {
		t.Fatal(err)
	}
	drainSelections(t, v)
	if !slices.Equal(v.Selections(), []TextSelection{{Anchor: 16, Caret: 19}, {Anchor: 8, Caret: 11}}) {
		t.Fatal(v.Selections())
	}
	v.SelectNextOccurrence()
	drainSelections(t, v)
	if len(v.Selections()) != 3 || v.Selection().Anchor != 0 {
		t.Fatal(v.Selections())
	}
	v.SelectNextOccurrence()
	drainSelections(t, v)
	if len(v.Selections()) != 3 {
		t.Fatal("repeat", v.Selections())
	}
	text := strings.Repeat("x", 65534) + "abcd" + strings.Repeat("x", 65534) + "abcd"
	v.Document().SetText(text)
	v.SetSelection(TextSelection{Anchor: 65534, Caret: 65538})
	v.SelectAllOccurrences()
	drainSelections(t, v)
	if !slices.Equal(v.Selections(), []TextSelection{{Anchor: 65534, Caret: 65538}, {Anchor: 131072, Caret: 131076}}) {
		t.Fatal(v.Selections())
	}
}
func TestSplitSelectionLinesActiveEndAndFinalLine(t *testing.T) {
	v, _ := colorFixture(t)
	v.Document().SetText("abc\ndef\nghi")
	for _, tc := range []struct {
		s    TextSelection
		want []TextSelection
	}{
		{TextSelection{Anchor: 1, Caret: 8}, []TextSelection{{Anchor: 4, Caret: 7}, {Anchor: 1, Caret: 3}}},
		{TextSelection{Anchor: 10, Caret: 1}, []TextSelection{{Anchor: 3, Caret: 1}, {Anchor: 7, Caret: 4}, {Anchor: 10, Caret: 8}}},
	} {
		v.SetSelection(tc.s)
		v.SplitSelectionLines()
		drainSelections(t, v)
		if !slices.Equal(v.Selections(), tc.want) {
			t.Fatal(v.Selections(), tc.want)
		}
	}
}
func TestColumnStationaryTargetConvergesAndPrimary(t *testing.T) {
	v, _ := colorFixture(t)
	focusBracketFixture(t, v)
	v.Document().SetText(strings.Repeat("abcdef\n", 100))
	v.LayoutVisible(v.viewport, v.offset)
	p := geometry.Point{X: v.gutter + padding + 10, Y: padding + 10}
	v.beginColumn(p, textcore.Position{Offset: 1})
	target := geometry.Point{X: p.X + 20, Y: p.Y + 80*20}
	v.updateColumn(target)
	c := v.columnSelection
	for i := 0; i < 100 && c.pending; i++ {
		v.updateColumn(target)
		_, err := v.advanceColumn(c, time.Time{})
		if err != nil && err != errSelectionGeometry {
			t.Fatal(err)
		}
	}
	if c.pending || len(v.Selections()) != 81 || v.Selection() != (TextSelection{Anchor: 561, Caret: 563}) {
		t.Fatal("rectangle", len(v.Selections()), v.Selection(), c.pending)
	}
	// Editing fixes positions and discards the temporary rectangle intent.
	v.ReplaceSelection("Z")
	if v.columnSelection != nil {
		t.Fatal("stale intent after edit")
	}
}
func TestColumnLimitAndDocumentCancellation(t *testing.T) {
	v, _ := colorFixture(t)
	v.Document().SetText(strings.Repeat("ab\n", maxSelections+1))
	v.LayoutVisible(v.viewport, v.offset)
	c := &columnSelection{state: v.operationState(), layout: v.layoutEpoch, anchor: textcore.Position{}, target: textcore.Position{Offset: maxSelections * 3}, left: 0, right: 10, pending: true, before: v.cursors()}
	v.columnSelection = c
	var err error
	for i := 0; i < 1000; i++ {
		_, err = v.advanceColumn(c, time.Time{})
		if err != nil && err != errSelectionGeometry {
			break
		}
	}
	if !errors.Is(err, ErrOperationLimit) || len(v.Selections()) != 1 {
		t.Fatal(err, len(v.Selections()))
	}
	v.SetDocument(NewDocument("other"))
	if v.columnSelection != nil {
		t.Fatal("retained document")
	}
}
func TestSelectionModifiersValidation(t *testing.T) {
	v := NewSourceView()
	old := v.SelectionModifiers()
	for _, bad := range []SelectionModifiers{{Add: gui.ModShift}, {Add: gui.ModAlt, Column: gui.ModAlt}} {
		if v.SetSelectionModifiers(bad) == nil || v.SelectionModifiers() != old {
			t.Fatal(bad)
		}
	}
	if err := v.SetSelectionModifiers(SelectionModifiers{}); err != nil || v.selectionGesture(0) != 0 {
		t.Fatal(err)
	}
}

func TestColumnLongUnwrappedRowsAndKeyboardContinuation(t *testing.T) {
	v := pendingInputFixture(t, "abc")
	focusBracketFixture(t, v)
	row := strings.Repeat("a", 100000)
	v.Document().SetText(row + "\n" + row + "\n" + row)
	left := float64(850000)
	c := &columnSelection{state: v.operationState(), layout: v.layoutEpoch, anchor: textcore.Position{Offset: 85000}, target: textcore.Position{Offset: 285002}, left: left, right: left + 20, before: v.cursors(), pending: true}
	v.columnSelection = c
	for i := 0; i < 1000 && c.pending; i++ {
		_, err := v.advanceColumn(c, time.Time{})
		if err != nil && err != errSelectionGeometry {
			t.Fatal(err)
		}
	}
	if c.pending || !slices.Equal(v.Selections(), []TextSelection{{Anchor: 285002, Caret: 285004}, {Anchor: 85000, Caret: 85002}, {Anchor: 185001, Caret: 185003}}) {
		t.Fatal(v.Selections(), c.pending)
	}
}

func TestColumnSkipsFoldedRowsAndFailureDoesNotEdit(t *testing.T) {
	v, _ := colorFixture(t)
	focusBracketFixture(t, v)
	v.Document().SetText("abcdef\nabcdef\nabcdef\nabcdef\nabcdef")
	folds, err := NewFolds(v.Document().Text(), []FoldRange{{StartLine: 0, EndLine: 2}})
	if err != nil {
		t.Fatal(err)
	}
	v.Document().SetFolds(v.Document().Revision(), folds)
	finishFoldIntake(t, v)
	if !v.Fold(0) {
		t.Fatal("fold")
	}
	c := &columnSelection{state: v.operationState(), layout: v.layoutEpoch, anchor: textcore.Position{Offset: 1}, target: textcore.Position{Offset: 29}, left: 10, right: 30, before: v.cursors(), pending: true}
	v.columnSelection = c
	for i := 0; i < 100 && c.pending; i++ {
		_, err = v.advanceColumn(c, time.Time{})
		if err != nil && err != errSelectionGeometry {
			t.Fatal(err)
		}
	}
	if c.pending || !slices.Equal(v.Selections(), []TextSelection{{Anchor: 29, Caret: 31}, {Anchor: 1, Caret: 3}, {Anchor: 22, Caret: 24}}) {
		t.Fatal(v.Selections())
	}
	old := v.Selections()
	c.pending = true
	v.invalidateLayout()
	if _, err = v.advanceColumn(c, time.Time{}); err != ErrOperationChanged || !slices.Equal(v.Selections(), old) {
		t.Fatal("layout invalidation", err, v.Selections())
	}
}

func TestKeyboardColumnCrossesLongParagraphPages(t *testing.T) {
	v := pendingInputFixture(t, "abc")
	v.Document().SetText(strings.Repeat("a", 100000))
	c := &columnSelection{state: v.operationState(), layout: v.layoutEpoch, anchor: textcore.Position{}, target: textcore.Position{Offset: 8190}, left: 0, right: 81900, before: v.cursors()}
	v.columnSelection = c
	for _, command := range []Command{CommandColumnSelectRight, CommandColumnSelectRight, CommandColumnSelectRight, CommandColumnSelectRight, CommandColumnSelectRight, CommandColumnSelectLeft} {
		var err error
		for retry := 0; retry < 1000; retry++ {
			err = v.beginKeyboardColumn(command)
			if err != errSelectionGeometry {
				break
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		c = v.columnSelection
		for retry := 0; retry < 1000 && c.pending; retry++ {
			_, err = v.advanceColumn(c, time.Time{})
			if err != nil && err != errSelectionGeometry {
				t.Fatal(err)
			}
		}
		if c.pending {
			t.Fatal("keyboard rectangle did not converge")
		}
	}
	if v.Selection() != (TextSelection{Anchor: 0, Caret: 8194}) {
		t.Fatal("stuck on anchor's native page", v.Selection())
	}
}

func TestSelectAllOccurrencesKeepsOverlappingPrimaryMatch(t *testing.T) {
	v, _ := colorFixture(t)
	v.Document().SetText("aaaaa")
	v.SetSelection(TextSelection{Anchor: 3, Caret: 1})
	v.SelectAllOccurrences()
	drainSelections(t, v)
	if !slices.Equal(v.Selections(), []TextSelection{{Anchor: 3, Caret: 1}, {Anchor: 3, Caret: 5}}) {
		t.Fatal(v.Selections())
	}
}

func TestDefaultAltSelectionGesturesAndOverride(t *testing.T) {
	v := NewSourceView()
	if got := v.SelectionModifiers(); got != (SelectionModifiers{Add: gui.ModAlt, Column: gui.ModAlt | gui.ModShift}) {
		t.Fatalf("default mouse gestures: got %v, want Alt / Alt+Shift", got)
	}
	for _, tc := range []struct {
		mods events.Modifiers
		want int
	}{
		{events.ModifierAlt, 1}, {events.ModifierAlt | events.ModifierShift, 2},
		{events.ModifierControl, 0}, {events.ModifierShift, 0}, {events.ModifierAlt | events.ModifierControl, 0},
	} {
		if got := v.selectionGesture(tc.mods); got != tc.want {
			t.Fatalf("modifiers %v: got %d, want %d", tc.mods, got, tc.want)
		}
	}
	if err := v.SetSelectionModifiers(SelectionModifiers{Add: gui.ModControl, Column: gui.ModControl | gui.ModShift}); err != nil {
		t.Fatal(err)
	}
	if v.selectionGesture(events.ModifierAlt) != 0 || v.selectionGesture(events.ModifierControl) != 1 || v.selectionGesture(events.ModifierControl|events.ModifierShift) != 2 {
		t.Fatal("custom pointer gestures did not replace defaults")
	}
}
