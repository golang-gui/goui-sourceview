package sourceview

import (
	"errors"
	"testing"

	"github.com/golang-gui/goui/gui/textedit"
)

func TestResetReentryCannotApplyOldEditRange(t *testing.T) {
	for _, switchDocument := range []bool{false, true} {
		t.Run(map[bool]string{false: "model edit", true: "document switch"}[switchDocument], func(t *testing.T) {
			v := NewSourceView()
			v.Document().Model().SetText("abc")
			observe(t, v)
			v.SetSelection(textedit.Selection{Caret: 2})
			old := v.Document()
			v.projection = textedit.NewProjection(old.model, v.Selection())
			v.projection.Update("preedit", 2)
			v.im = &resetContext{reset: func() {
				if switchDocument {
					v.SetDocument(NewDocument("new"))
				} else {
					_ = old.model.Replace(textedit.Range{Start: 3, End: 3}, "Z")
				}
			}}
			if err := v.ReplaceSelection("X"); !errors.Is(err, ErrOperationChanged) {
				t.Fatal("stale edit was accepted", err)
			}
			expected := "abcZ"
			if switchDocument {
				expected = "new"
			}
			if v.Document().model.Text() != expected {
				t.Fatal(v.Document().model.Text())
			}
			if switchDocument && len(old.views) != 0 {
				t.Fatal("old document retained view")
			}
		})
	}
}

func TestUnmountReleasesOriginalDocumentBeforeNativeReset(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	original := v.Document()
	v.projection = textedit.NewProjection(original.model, v.Selection())
	replacement := NewDocument("new")
	v.im = &resetContext{reset: func() { v.SetDocument(replacement) }}
	v.unmount()
	if original.connection != nil || original.engine != nil || len(original.views) != 0 || replacement.connection != nil {
		t.Fatal("unmount retained external subscription")
	}
}

func TestEditCallbackSelectionTakesPrecedence(t *testing.T) {
	v := NewSourceView()
	v.Document().model.SetText("abc")
	observe(t, v)
	v.SetSelection(textedit.Selection{Caret: 2})
	once := true
	v.ConnectChange(func(textedit.Change) {
		if once {
			once = false
			v.SetSelection(textedit.Selection{Caret: 1})
		}
	})
	if err := v.ReplaceSelection("X"); err != nil {
		t.Fatal(err)
	}
	if v.Selection() != (textedit.Selection{Caret: 1}) {
		t.Fatal("command overwrote callback selection", v.Selection())
	}
}

func TestProjectionOnlySuppressesAffectedLineHighlighting(t *testing.T) {
	v := NewSourceView()
	v.Document().model.SetText("zero\none\ntwo\nthree")
	v.projection = textedit.NewProjection(v.Document().model, textedit.Selection{Anchor: 5, Caret: 12})
	v.projection.Update("a\nb\nc", 2)
	// Two committed paragraphs become three display paragraphs.
	for row, expected := range []struct {
		row   int
		valid bool
	}{{0, true}, {0, false}, {0, false}, {0, false}, {3, true}} {
		actual, valid := v.highlightRow(row)
		if actual != expected.row || valid != expected.valid {
			t.Fatalf("display row %d maps to %d/%v", row, actual, valid)
		}
	}
}
