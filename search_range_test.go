package sourceview

import (
	"github.com/golang-gui/goui/gui/textedit"
	"testing"
)

func TestSelectionSearchRangeSurvivesResultSelectionsAndEdits(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("a a a")
	observe(t, v)
	v.SetSearch(SearchQuery{Text: "a"})
	scope := textedit.Range{Start: 2, End: 5}
	if err := v.SetSearchRange(&scope); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{2, 4, 2} {
		r, ok := v.FindNext(false)
		if !ok || r.Start != want {
			t.Fatal(r, want)
		}
	}
	if r, ok := v.SearchRange(); !ok || r != scope {
		t.Fatal("result changed scope", r)
	}
	if n, err := v.ReplaceAll("bb"); err != nil || n != 2 || v.document.model.Text() != "a bb bb" {
		t.Fatal(n, err, v.document.model.Text())
	}
	if r, _ := v.SearchRange(); r != (textedit.Range{Start: 2, End: 7}) {
		t.Fatal("mapped scope", r)
	}
	v.SetDocument(NewDocument("other"))
	if _, ok := v.SearchRange(); ok {
		t.Fatal("scope leaked to new document")
	}
}
func TestZeroWidthFindProgressAndInvalidRegex(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("a\n中")
	observe(t, v)
	v.SetSearch(SearchQuery{Text: `(?m)^`, Regex: true})
	for _, want := range []int{0, 2, 0, 2} {
		r, ok := v.FindNext(false)
		if !ok || r.Start != want || r.End != want {
			t.Fatal(r, want)
		}
	}
	v.SetSearch(SearchQuery{Text: "[", Regex: true})
	if v.SearchError() == nil {
		t.Fatal("syntax accepted")
	}
	if _, err := v.ReplaceAll("x"); err == nil || v.document.model.Text() != "a\n中" {
		t.Fatal(err)
	}
}
