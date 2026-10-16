package sourceview

import (
	"errors"
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

// Bind only the pure document observer; native mount is covered by tests/window.
func observe(t *testing.T, v *SourceView) {
	t.Helper()
	v.mounted = true
	v.document.acquire(v)
	t.Cleanup(func() {
		if v.mounted {
			v.mounted = false
			v.document.release(v)
		}
	})
}
func TestIndependentDocumentsAndViews(t *testing.T) {
	d := NewDocument("abc\nxyz")
	a, b := NewSourceView(), NewSourceView()
	a.SetDocument(d)
	b.SetDocument(d)
	a.SetSelection(textedit.Selection{Anchor: 1, Caret: 2})
	if d.connection != nil || d.engine != nil {
		t.Fatal("constructor subscribed to external model")
	}
	observe(t, a)
	observe(t, b)
	if len(d.views) != 2 {
		t.Fatal("shared subscriptions")
	}
	b.SetSelection(textedit.Selection{Anchor: 5, Caret: 5})
	if err := a.ReplaceSelection("ABC"); err != nil {
		t.Fatal(err)
	}
	if got := d.Model().Text(); got != "aABCc\nxyz" {
		t.Fatal(got)
	}
	if a.Selection() != (textedit.Selection{Anchor: 4, Caret: 4}) || b.Selection().Caret != 7 {
		t.Fatalf("selections %v / %v", a.Selection(), b.Selection())
	}
	if !a.Undo() || d.Model().Text() != "abc\nxyz" || a.Selection() != (textedit.Selection{Anchor: 1, Caret: 2}) {
		t.Fatal("undo selection", a.Selection())
	}
	b.SetReadOnly(true)
	if !errors.Is(b.ReplaceSelection("bad"), ErrReadOnly) || b.Undo() {
		t.Fatal("read only edited")
	}
	a.mounted = false
	d.release(a)
	if d.engine == nil {
		t.Fatal("one mounted view still needs analysis")
	}
	b.mounted = false
	d.release(b)
	if d.engine != nil || d.connection != nil {
		t.Fatal("last unmount retained observer")
	}
}
func TestHistoryIndentSearchAndPrivateDocument(t *testing.T) {
	v := NewSourceView()
	original := v.Document()
	m := original.Model()
	m.SetText("  alpha\n  alpha\nlast")
	v.SetSelection(textedit.Selection{Caret: 15})
	observe(t, v)
	if err := v.Indent(false); err != nil {
		t.Fatal(err)
	}
	if m.Text() != "      alpha\n      alpha\nlast" {
		t.Fatal(m.Text())
	}
	if !v.Undo() || m.Text() != "  alpha\n  alpha\nlast" {
		t.Fatal("atomic indent undo")
	}
	v.SetSearch(SearchQuery{Text: "alpha"})
	n, err := v.ReplaceAll("β")
	if err != nil || n != 2 || m.Text() != "  β\n  β\nlast" {
		t.Fatal(n, err, m.Text())
	}
	if !v.Undo() || m.Text() != "  alpha\n  alpha\nlast" {
		t.Fatal("atomic replace undo")
	}
	r, ok := v.FindNext(false)
	if !ok || r != (textedit.Range{Start: 2, End: 7}) {
		t.Fatal(r, ok)
	}
	if ok, err := v.ReplaceCurrent("x"); !ok || err != nil || m.Text() != "  x\n  alpha\nlast" {
		t.Fatal(ok, err, m.Text())
	}
	other := NewDocument("shared")
	v.SetDocument(other)
	v.SetDocument(nil)
	if v.Document() != original || m.Text() != "  x\n  alpha\nlast" {
		t.Fatal("private document lost")
	}
}
func TestNotificationReentryDoesNotOverwriteNewSelection(t *testing.T) {
	d := NewDocument("abc")
	v := NewSourceView()
	v.SetDocument(d)
	// This listener runs before the mounted view and sees a newer revision
	// while the older notification is still being delivered.
	fired := false
	handle := d.Model().ConnectChange(func(c textedit.Change) {
		if fired {
			return
		}
		fired = true
		if err := d.Model().Replace(textedit.Range{Start: 0, End: 0}, "Ω"); err != nil {
			t.Fatal(err)
		}
		v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	})
	defer handle.Disconnect()
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
	if err := v.ReplaceSelection("!"); err != nil {
		t.Fatal(err)
	}
	if got := v.Selection(); got != (textedit.Selection{Anchor: 2, Caret: 2}) {
		t.Fatal("older edit overwrote reentrant selection", got)
	}
	if v.observed != d.Model().Revision() {
		t.Fatal("notifications not drained")
	}
}
func TestResetAndLanguageValidation(t *testing.T) {
	d := NewDocument("abc")
	v := NewSourceView()
	v.SetDocument(d)
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
	d.Model().SetText("new")
	if v.Selection() != (textedit.Selection{}) {
		t.Fatal("reset selection")
	}
	if err := d.SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	engine := d.engine
	if err := d.SetLanguage(language.ID("missing")); !errors.Is(err, ErrUnknownLanguage) || d.Language() != language.JSON || d.engine != engine {
		t.Fatal("invalid language mutated document")
	}
	info := v.Snapshot()
	if info.Role != "sourceview" || info.TextEditing == nil || info.TextEditing.Length != 3 {
		t.Fatalf("%+v", info)
	}
}
