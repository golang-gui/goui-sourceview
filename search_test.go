package sourceview

import (
	"slices"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestSearchKeepsRawMatchSeparateFromClusterSelection(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("e\u0301 z")
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	v.SetSearch(SearchQuery{Text: "\u0301"})
	raw, ok := v.FindNext(false)
	if !ok || raw != (textedit.Range{Start: 1, End: 3}) || v.Selection() != (textedit.Selection{Caret: 3}) {
		t.Fatalf("raw %v selection %v", raw, v.Selection())
	}
	if ok, err := v.ReplaceCurrent("X"); !ok || err != nil || v.Document().Model().Text() != "eX z" {
		t.Fatal("visual selection replaced the base character", ok, err, v.Document().Model().Text())
	}
	v.dropLayouts()
	// Every native object, including measurement probes, has one view owner.
	if ctx.created != ctx.destroyed {
		t.Fatalf("native layout leak %d/%d", ctx.created, ctx.destroyed)
	}
}

func TestFindReentryDoesNotRestoreAnObsoleteRawMatch(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("abc abc")
	observe(t, v)
	v.SetSearch(SearchQuery{Text: "abc"})
	once := true
	v.ConnectSelection(func(textedit.Selection) {
		if once {
			once = false
			v.SetSelection(textedit.Selection{Anchor: 4, Caret: 4})
		}
	})
	_, _ = v.FindNext(false)
	if ok, err := v.ReplaceCurrent("X"); ok || err != nil || v.Document().Model().Text() != "abc abc" {
		t.Fatal("reentrant selection ignored", ok, err)
	}
}

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

func TestSearchResultDiscardedWhenSelectionCallbackChangesScope(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("foo foo")
	v.SetSearch(SearchQuery{Text: "foo"})
	h := v.ConnectSelection(func(textedit.Selection) {
		r := textedit.Range{Start: 4, End: 7}
		if err := v.SetSearchRange(&r); err != nil {
			t.Fatal(err)
		}
	})
	defer h.Disconnect()
	v.FindNext(false)
	if v.currentMatch != nil {
		t.Fatal("obsolete raw match survived a reentrant scope change")
	}
}

func TestSearchHighlightVisibilityPreservesSyntaxAndSelection(t *testing.T) {
	v, ctx := colorFixture(t)
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false)
	v.Arrange(geometry.Rect(0, 0, 400, 200))
	v.SetSearch(SearchQuery{Text: "{"})
	if _, ok := v.FindNext(false); !ok {
		t.Fatal("missing current search result")
	}
	paint := func() *recordingPainter { p := &recordingPainter{}; v.Paint(p); return p }
	if !v.HighlightSearchMatches() || len(bracketFills(paint(), v.colors.CurrentSearchBackground)) != 1 {
		t.Fatal("new view did not highlight the current match")
	}
	item := v.paragraph(0)
	native := item.layout.(*shapedLayout)
	foreground, created, selected, query := slices.Clone(native.colors), ctx.created, v.Selection(), v.Search()
	v.SetHighlightSearchMatches(false)
	p := paint()
	if len(bracketFills(p, v.colors.SearchBackground)) != 0 || len(bracketFills(p, v.colors.CurrentSearchBackground)) != 0 {
		t.Fatal("hidden search still painted backgrounds")
	}
	if len(bracketFills(p, v.colors.InactiveSelectionBackground)) != 1 || v.Selection() != selected || v.Search() != query {
		t.Fatal("hiding search changed ordinary selection or query")
	}
	if v.paragraph(0) != item || ctx.created != created || !slices.Equal(native.colors, foreground) {
		t.Fatal("hiding search invalidated native syntax layouts")
	}
	v.SetHighlightSearchMatches(true)
	if len(bracketFills(paint(), v.colors.CurrentSearchBackground)) != 1 {
		t.Fatal("restoring highlights lost the current result")
	}
}

func TestHiddenSearchKeepsZeroWidthNavigationAndReplacement(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("a\nb")
	observe(t, v)
	v.SetSearch(SearchQuery{Text: `(?m)^`, Regex: true})
	if r, ok := v.FindNext(false); !ok || r != (textedit.Range{}) {
		t.Fatal(r, ok)
	}
	v.SetHighlightSearchMatches(false)
	if r, ok := v.FindNext(false); !ok || r != (textedit.Range{Start: 2, End: 2}) || v.HighlightSearchMatches() {
		t.Fatal("hidden navigation repeated or showed highlights", r, ok)
	}
	if n, err := v.ReplaceAll(">"); err != nil || n != 2 || v.Document().Model().Text() != ">a\n>b" {
		t.Fatal("visibility affected replacement", n, err)
	}
}
