package sourceview

import (
	"slices"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestSearchHighlightVisibilityPreservesSyntaxAndSelection(t *testing.T) {
	v, ctx := colorFixture(t)
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false)
	v.Arrange(geometry.Rect(0, 0, 400, 200))
	v.SetSearch(SearchQuery{Text: "{"})
	if _, ok := v.FindNext(false); !ok {
		t.Fatal("missing current search result")
	}
	paint := func() *bracketPainter { p := &bracketPainter{}; v.Paint(p); return p }
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
