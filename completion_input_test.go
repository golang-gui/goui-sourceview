package sourceview

import (
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

func TestCompletionInputWordCallbackReplacement(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	original, replacement := v.Document(), NewDocument("keep")
	armed := false
	definition, err := language.New(language.Definition{Info: language.Info{ID: "callback", Name: "Callback"}, Editing: language.EditingRules{Word: func(r rune) bool {
		if armed {
			armed = false
			v.SetDocument(replacement)
		}
		return language.IdentifierRune(r)
	}}})
	if err != nil {
		t.Fatal(err)
	}
	original.SetLanguageDefinition(definition)
	original.Model().SetText("word")
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	s := &completionSession{state: v.operationState(), definition: definition, range_: textedit.Range{Start: 0, End: 4}, list: &completionList{}, popup: closingPopup{onClose: func() {}}}
	v.completion = s
	armed = true
	_, err = v.completionInput(textedit.Range{Start: 2, End: 2}, "a", false)
	if err != ErrOperationChanged || v.Document() != replacement || replacement.Model().Text() != "keep" || original.Model().Text() != "word" {
		t.Fatalf("word callback edited replacement: err=%v old=%q replacement=%q", err, original.Model().Text(), replacement.Model().Text())
	}
}

func TestCompletionOwnedEditRetention(t *testing.T) {
	for _, kind := range []string{"insert", "backspace", "delete", "external", "reenter model", "reenter selection", "shared view"} {
		t.Run(kind, func(t *testing.T) {
			v := NewSourceView()
			observe(t, v)
			v.document.model.SetText("word tail")
			v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
			c, _ := v.completionContext(true)
			closed := 0
			s := &completionSession{state: v.operationState(), definition: c.Definition, language: c.Language, range_: c.Range, explicit: true, items: []Completion{{Label: "worker", Text: "worker"}}, list: &completionList{}, popup: closingPopup{onClose: func() { closed++ }}}
			v.completion = s
			t.Cleanup(v.closeCompletion)
			r, text := textedit.Range{Start: 2, End: 2}, "a"
			end := 5
			if kind == "backspace" {
				r, text, end = textedit.Range{Start: 1, End: 2}, "", 3
			}
			if kind == "delete" {
				r, text, end = textedit.Range{Start: 2, End: 3}, "", 3
			}
			if kind == "reenter model" {
				v.ConnectChange(func(textedit.Change) {
					if v.document.model.Text() != "replacement" {
						v.document.model.SetText("replacement")
					}
				})
			}
			if kind == "reenter selection" {
				v.ConnectSelection(func(textedit.Selection) { v.SetSelection(textedit.Selection{Anchor: 1, Caret: 1}) })
			}
			keep := &completionEdit{state: v.operationState(), epoch: v.completionEpoch, session: s, request: CompletionContext{Definition: c.Definition, Range: c.Range, Explicit: true}, edit: textedit.Edit{Range: r, Text: text}}
			switch kind {
			case "external":
				if err := v.ReplaceSelection(text); err != nil {
					t.Fatal(err)
				}
			case "shared view":
				other := NewSourceView()
				other.SetDocument(v.document)
				observe(t, other)
				other.SetSelection(v.Selection())
				if err := other.ReplaceSelection(text); err != nil {
					t.Fatal(err)
				}
			default:
				if err := v.replaceKeepingCompletion(r, text, false, keep); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "insert" || kind == "backspace" || kind == "delete" {
				if v.completion != s || closed != 0 || !s.pending || !s.state.valid(v) || s.range_ != (textedit.Range{Start: 0, End: end}) || v.completionEdit != nil {
					t.Fatalf("lost owned session: closed=%d pending=%t range=%v", closed, s.pending, s.range_)
				}
				before := v.document.model.Text()
				if err := v.acceptCompletion(s, 0); err != nil || v.document.model.Text() != before {
					t.Fatal("stale candidate accepted", err)
				}
				v.document.model.SetText("external afterward")
			}
			if v.completion != nil || closed != 1 || v.completionEdit != nil {
				t.Fatal("retention escaped its one edit", closed)
			}
		})
	}
}

// The key precedes a separate text commit. Text/Shift/AltGr must leave the
// session available; a shortcut or non-text navigation cancels it.
func TestCompletionTextKeyRetention(t *testing.T) {
	for _, tc := range []struct {
		key  events.Key
		mods events.Modifiers
		keep bool
	}{
		{events.KeyA, 0, true}, {events.KeyA, events.ModifierShift, true}, {events.KeyShift, events.ModifierShift, true},
		{events.KeyUnknown, 0, true}, {events.KeyUnknown, events.ModifierAltGraph, true},
		{events.KeySpace, 0, true}, {events.KeyPeriod, 0, true},
		{events.KeyA, events.ModifierControl, false}, {events.KeyHome, 0, false}, {events.KeyF3, 0, false},
	} {
		v := NewSourceView()
		closed := 0
		s := &completionSession{state: v.operationState(), definition: v.document.definition, list: &completionList{}, popup: closingPopup{onClose: func() { closed++ }}}
		v.completion = s
		if v.completionKey(events.KeyEvent{Key: tc.key, Modifiers: tc.mods}) {
			t.Fatal("completion consumed a delegated key")
		}
		if (v.completion == s) != tc.keep || (!tc.keep && closed != 1) || (tc.keep && closed != 0) {
			t.Fatalf("%v/%v: session kept=%t closed=%d", tc.key, tc.mods, v.completion == s, closed)
		}
		v.closeCompletion()
	}
}
