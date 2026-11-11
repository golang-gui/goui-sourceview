package sourceview_test

import (
	"reflect"
	"testing"

	source "github.com/golang-gui/goui-sourceview/sourceview"
)

func TestDocumentTextContract(t *testing.T) {
	d := source.NewDocument("a\r\n中\r\nb")
	if d.Text() != "a\n中\nb" || !d.EqualText("a\r\n中\nb") || d.Len() != 7 || d.LineCount() != 3 || d.Revision() != 0 || d.LineEnding() != source.LineEndingCRLF {
		t.Fatal("document normalization or initial state", d.Text(), d.Len(), d.LineCount())
	}
	for line, want := range []source.TextRange{{Start: 0, End: 1}, {Start: 2, End: 5}, {Start: 6, End: 7}} {
		got, err := d.LineRange(line)
		if err != nil || got != want || d.LineAt(want.Start) != line {
			t.Fatal("line query", line, got, err)
		}
	}
	if d.IsRuneBoundary(3) || !d.IsRuneBoundary(5) || d.LineAt(-1) != 0 || d.LineAt(100) != 2 {
		t.Fatal("byte boundaries or clamped line query")
	}
	if _, err := d.Slice(source.TextRange{Start: 3, End: 5}); err == nil {
		t.Fatal("accepted UTF-8 interior range")
	}
	var changes []source.TextChange
	h := d.ConnectChange(func(c source.TextChange) { changes = append(changes, c) })
	defer h.Disconnect()
	invalid := []source.TextEdit{{Range: source.TextRange{Start: 0, End: 1}, Text: "A"}, {Range: source.TextRange{Start: 3, End: 5}}}
	if err := d.ApplyEdits(invalid); err == nil || d.Revision() != 0 || d.CanUndo() || len(changes) != 0 || d.Text() != "a\n中\nb" {
		t.Fatal("invalid batch partially committed", err, d.Text(), changes)
	}
	edits := []source.TextEdit{{Range: source.TextRange{Start: 0, End: 1}, Text: "A\r\n"}, {Range: source.TextRange{Start: 6, End: 7}, Text: "B"}}
	if err := d.ApplyEdits(edits); err != nil {
		t.Fatal(err)
	}
	wantChanges := []source.TextChange{{Revision: 1, Edits: []source.TextEdit{{Range: source.TextRange{Start: 6, End: 7}, Text: "B"}, {Range: source.TextRange{Start: 0, End: 1}, Text: "A\n"}}}}
	if d.Text() != "A\n\n中\nB" || !reflect.DeepEqual(changes, wantChanges) || !d.CanUndo() || d.CanRedo() {
		t.Fatal("batch result or sequential notification", d.Text(), changes)
	}
	if !d.Undo() || d.Text() != "a\n中\nb" || !d.CanRedo() || !d.Redo() || d.Text() != "A\n\n中\nB" {
		t.Fatal("batch is not one history item")
	}
	revision := d.Revision()
	d.ClearHistory()
	if d.CanUndo() || d.CanRedo() || d.Revision() != revision {
		t.Fatal("history clear changed content revision")
	}
	if err := d.Replace(source.TextRange{Start: d.Len(), End: d.Len()}, "x"); err != nil {
		t.Fatal(err)
	}
	d.BreakUndoGroup()
	if err := d.ReplaceAtomic(source.TextRange{}, "prefix"); err != nil {
		t.Fatal(err)
	}
	d.SetText("reloaded\n")
	if d.CanUndo() || d.CanRedo() || d.LineEnding() != source.LineEndingCRLF || !changes[len(changes)-1].Reset {
		t.Fatal("reload lost output configuration or retained history")
	}
	revision = d.Revision()
	d.SetText("reloaded\r\n")
	if d.Revision() != revision {
		t.Fatal("equal reload is not a no-op")
	}
	other := source.NewDocument(d.Text())
	if other.ID() == d.ID() || other.CanUndo() || other.Revision() != 0 || other.LineEnding() != source.LineEndingLF {
		t.Fatal("separate documents shared state")
	}
}

func TestDocumentChangeReentryAndDisconnect(t *testing.T) {
	d := source.NewDocument("a")
	h := d.ConnectChange(func(c source.TextChange) {
		if c.Revision == 1 {
			if err := d.ReplaceAtomic(source.TextRange{Start: d.Len(), End: d.Len()}, "!"); err != nil {
				t.Fatal(err)
			}
		}
	})
	defer h.Disconnect()
	var revisions []uint64
	observer := d.ConnectChange(func(c source.TextChange) { revisions = append(revisions, c.Revision) })
	if err := d.ReplaceAtomic(source.TextRange{Start: 1, End: 1}, "中"); err != nil {
		t.Fatal(err)
	}
	if d.Text() != "a中!" || !reflect.DeepEqual(revisions, []uint64{1, 2}) {
		t.Fatal("reentry reordered notifications", d.Text(), revisions)
	}
	observer.Disconnect()
	if !d.Undo() || d.Text() != "a中" || len(revisions) != 2 {
		t.Fatal("observer or reentrant history leaked", d.Text(), revisions)
	}
}
