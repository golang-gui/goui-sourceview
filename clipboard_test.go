package sourceview

import (
	"errors"
	"testing"

	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

type fakeClipboard struct {
	text      string
	err       error
	pending   func(string, error)
	write     func()
	immediate bool
}

func (c *fakeClipboard) SetText(text string) error {
	if c.err != nil {
		return c.err
	}
	c.text = text
	if c.write != nil {
		c.write()
	}
	return nil
}
func (c *fakeClipboard) RequestText(fn func(string, error)) {
	if c.immediate {
		fn(c.text, c.err)
	} else {
		c.pending = fn
	}
}

type clipboardApplication struct {
	gui.Application
	clipboard gui.Clipboard
}

func (a clipboardApplication) Clipboard() gui.Clipboard { return a.clipboard }
func clipboardFixture(t *testing.T) (*SourceView, *fakeClipboard) {
	t.Helper()
	previous := gui.App
	clip := &fakeClipboard{}
	gui.App = clipboardApplication{clipboard: clip}
	t.Cleanup(func() { gui.App = previous })
	v := NewSourceView()
	v.Document().Model().SetText("abc")
	observe(t, v)
	v.SetSelection(textedit.Selection{Caret: 2})
	return v, clip
}
func TestCutPreservesTextOnWriteFailureAndReentry(t *testing.T) {
	v, c := clipboardFixture(t)
	c.err = errors.New("native clipboard unavailable")
	if err := v.Cut(); !errors.Is(err, c.err) || v.Document().Model().Text() != "abc" {
		t.Fatal("failed cut deleted text", err)
	}
	c.err = nil
	c.write = func() { v.SetSelection(textedit.Selection{Caret: 1}) }
	if err := v.Cut(); err != nil || v.Document().Model().Text() != "abc" {
		t.Fatal("reentrant copy deleted stale range")
	}
	c.write = nil
	v.SetSelection(textedit.Selection{Caret: 2})
	if err := v.Cut(); err != nil || c.text != "ab" || v.Document().Model().Text() != "c" {
		t.Fatal("cut", err, c.text)
	}
	if !v.Undo() || v.Document().Model().Text() != "abc" {
		t.Fatal("cut history")
	}
}
func TestPasteIdentityErrorsAndSynchronousCompletion(t *testing.T) {
	v, c := clipboardFixture(t)
	reported := 0
	v.ConnectError(func(error) { reported++ })
	if err := v.Paste(); err != nil {
		t.Fatal(err)
	}
	v.SetSelection(textedit.Selection{Caret: 1})
	c.pending("stale", nil)
	if v.Document().Model().Text() != "abc" {
		t.Fatal("stale selection pasted")
	}
	v.SetSelection(textedit.Selection{Anchor: 1, Caret: 2})
	if err := v.Paste(); err != nil {
		t.Fatal(err)
	}
	c.pending("", nil)
	if v.Document().Model().Text() != "abc" || v.Selection() != (textedit.Selection{Anchor: 1, Caret: 2}) || v.pendingPaste != nil {
		t.Fatal("empty paste changed document/selection or retained pending state")
	}
	if reported != 0 {
		t.Fatal("no text emitted error")
	}
	if err := v.Paste(); err != nil {
		t.Fatal(err)
	}
	c.pending("", errors.New("conversion failure"))
	if reported != 1 {
		t.Fatal("read failure not reported")
	}
	c.immediate = true
	c.text = "Ω"
	v.SetSelection(textedit.Selection{Anchor: 1, Caret: 2})
	if err := v.Paste(); err != nil || v.Document().Model().Text() != "aΩc" {
		t.Fatal("synchronous callback", err, v.Document().Model().Text())
	}
	c.immediate = false
	_ = v.Paste()
	pending := v.pendingPaste
	v.unmount()
	if pending.view != nil || pending.document != nil || v.pendingPaste != nil {
		t.Fatal("cancelled callback retains a widget or document")
	}
	c.pending("stale", nil)
	if v.Document().Model().Text() != "aΩc" {
		t.Fatal("unmounted view pasted")
	}
}
