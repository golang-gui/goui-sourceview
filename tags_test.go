package sourceview

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestTagMatchingCachesNegativeResultWhileLexing(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("[<?xml version=\"1.0\"?>\n" + strings.Repeat("<item>value</item>\n", 10000))
	if err := v.Document().SetLanguage(language.XML); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	if ranges := v.matchingBrackets(); len(ranges) != 0 || !v.tagMatches.valid || v.brackets.valid {
		t.Fatal("unresolved bracket lexing did not retain its independent negative tag result", ranges)
	}
	cache := v.tagMatches
	for range 10 {
		if len(v.matchingBrackets()) != 0 || !v.tagMatches.valid || v.tagMatches.document != cache.document || v.tagMatches.revision != cache.revision || v.tagMatches.caret != cache.caret {
			t.Fatal("unchanged matching request lost its negative cache")
		}
	}
	v.Document().engine.Advance(time.Time{})
	if len(v.matchingBrackets()) != 0 || !v.brackets.valid {
		t.Fatal("tag cache prevented the bracket scanner from completing")
	}
}

func TestTagMatchCacheInvalidatesForDocumentPositionAndLanguage(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("<root><a></a><b></b></root>")
	if err := v.Document().SetLanguage(language.XML); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	check := func(at, opening, closing int) {
		t.Helper()
		v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
		r := v.matchingTags()
		if len(r) != 2 || r[0] != (textedit.Range{Start: opening, End: opening + 1}) || r[1] != (textedit.Range{Start: closing, End: closing + 1}) {
			t.Fatal("incorrect matched pair", r)
		}
		if got := v.matchingTags(); &got[0] != &r[0] {
			t.Fatal("unchanged tag query reparsed instead of retaining its result")
		}
	}
	check(7, 7, 11)
	check(14, 14, 18)
	if err := v.Document().Model().ReplaceAtomic(textedit.Range{}, "\n"); err != nil {
		t.Fatal(err)
	}
	check(15, 15, 19)
	if err := v.Document().SetLanguage(language.Plain); err != nil {
		t.Fatal(err)
	}
	if len(v.matchingTags()) != 0 {
		t.Fatal("plain language retained markup decoration")
	}
	v.SetDocument(NewDocument("other"))
	if v.tagMatches.valid || v.tagMatches.document != nil || len(v.matchingTags()) != 0 {
		t.Fatal("document replacement retained old markup ownership")
	}
}
