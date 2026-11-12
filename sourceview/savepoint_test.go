package sourceview

import (
	"io"
	"slices"
	"testing"

	"github.com/golang-gui/goui/core/signal"
)

func TestSavePointMergedInputBoundaryAndOldSaveCompletion(t *testing.T) {
	d := NewDocument("a")
	if d.Modified() {
		t.Fatal("new document starts modified")
	}
	d.Replace(TextRange{Start: 1, End: 1}, "b")
	point := d.CaptureSavePoint()
	if !d.Modified() {
		t.Fatal("capture changed saved baseline")
	}
	d.Replace(TextRange{Start: 2, End: 2}, "c")
	if err := d.MarkSaved(point); err != nil || !d.Modified() {
		t.Fatal("old save completion hid newer edits", err)
	}
	if !d.Undo() || d.Text() != "ab" || d.Modified() {
		t.Fatal("capture did not preserve save boundary")
	}
	if !d.Redo() || d.Text() != "abc" || !d.Modified() {
		t.Fatal("redo did not leave saved state")
	}
	d.Undo()
	d.Replace(TextRange{Start: 2, End: 2}, "d")
	if !d.Modified() || d.CanRedo() {
		t.Fatal("branch incorrectly reused clean state")
	}
	if err := d.MarkSaved(d.CaptureSavePoint()); err != nil || d.Modified() {
		t.Fatal(err)
	}
}

func TestSavePointLineEndingsIdentityAndIsolation(t *testing.T) {
	d := NewDocument("a\r\nb")
	if d.Modified() || d.LineEnding() != LineEndingCRLF {
		t.Fatal("initial CRLF baseline")
	}
	d.SetLineEnding(LineEndingLF)
	if !d.Modified() {
		t.Fatal("line ending change ignored")
	}
	d.SetLineEnding(LineEndingCRLF)
	if d.Modified() {
		t.Fatal("line ending round trip remained modified")
	}
	p := d.CaptureSavePoint()
	other := NewDocument("a\r\nb")
	for _, bad := range []SavePoint{{}, other.CaptureSavePoint()} {
		if err := d.MarkSaved(bad); err == nil || d.Modified() {
			t.Fatal("foreign point changed baseline")
		}
	}
	d.ReplaceAtomic(TextRange{Start: 0, End: 1}, "a")
	if !d.Modified() {
		t.Fatal("equal bytes treated as saved history identity")
	}
	d.Undo()
	if d.Modified() {
		t.Fatal("undo to initial state is not clean")
	}
	d.SetText("different")
	if !d.Modified() {
		t.Fatal("SetText silently marked saved")
	}
	d.MarkSaved(p)
	if !d.Modified() {
		t.Fatal("unreachable baseline marked different reset state clean")
	}
}

func TestSavePointBatchViewsAndFailedWrite(t *testing.T) {
	d := NewDocument("abc\ndef")
	a, b := NewSourceView(), NewSourceView()
	a.SetDocument(d)
	b.SetDocument(d)
	a.SetSelections([]TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 5, Caret: 5}})
	if err := a.ReplaceSelection("x"); err != nil || !d.Modified() {
		t.Fatal(err)
	}
	if !a.Undo() || d.Modified() {
		t.Fatal("multi-edit undo did not return to clean")
	}
	a.Redo()
	point := d.CaptureSavePoint()
	if _, err := d.WriteTo(closedSaveWriter{}); err == nil || !d.Modified() {
		t.Fatal("failed write changed baseline")
	}
	if _, err := d.WriteTo(io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := d.MarkSaved(point); err != nil || d.Modified() || b.Document().Modified() {
		t.Fatal("shared baseline", err)
	}
	if d.Undo() && !d.Modified() {
		t.Fatal("undo away from marked batch state remained clean")
	}
}

type closedSaveWriter struct{}

func (closedSaveWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestModifiedSignalReentryBlockDisconnectAndNoMount(t *testing.T) {
	d := NewDocument("a")
	var observed []bool
	h := d.ConnectModifiedChanged(func() { observed = append(observed, d.Modified()) })
	if len(observed) != 0 {
		t.Fatal("subscription called immediately")
	}
	d.Replace(TextRange{Start: 1, End: 1}, "b")
	d.Replace(TextRange{Start: 2, End: 2}, "c")
	if !slices.Equal(observed, []bool{true}) {
		t.Fatal(observed)
	}
	d.MarkSaved(d.CaptureSavePoint())
	if !slices.Equal(observed, []bool{true, false}) {
		t.Fatal(observed)
	}
	h.Block()
	d.SetLineEnding(LineEndingCRLF)
	h.Unblock()
	d.Replace(TextRange{Start: 3, End: 3}, "d")
	if !slices.Equal(observed, []bool{true, false, true}) {
		t.Fatal("blocked state did not coalesce", observed)
	}
	h.Disconnect()
	d.MarkSaved(d.CaptureSavePoint())
	if len(observed) != 3 {
		t.Fatal("disconnected observer retained")
	}
	var reentrant signal.Handle
	calls := 0
	reentrant = d.ConnectModifiedChanged(func() {
		calls++
		if d.Modified() {
			d.MarkSaved(d.CaptureSavePoint())
		} else {
			reentrant.Disconnect()
		}
	})
	d.Replace(TextRange{Start: 4, End: 4}, "e")
	if calls != 2 || d.Modified() {
		t.Fatal("reentrant mark/disconnect", calls)
	}
	d.Replace(TextRange{Start: 5, End: 5}, "f")
	if calls != 2 {
		t.Fatal("reentrant disconnection lost")
	}
}
