package sourceview

import (
	"reflect"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

func TestSmartIndentBackspaceIsOneUndo(t *testing.T) {
	for _, tc := range []struct {
		before, after string
		caret         int
	}{{"\t  value", "\tvalue", 1}, {"      value", "    value", 4}, {"\tvalue", "    value", 4}} {
		v := NewSourceView()
		v.SetFoldingEnabled(false)
		v.document.model.SetText(tc.before)
		observe(t, v)
		options := v.IndentOptions()
		options.TabWidth = 8
		options.IndentWidth = 4
		_ = v.SetIndentOptions(options)
		n, _ := leadingColumns(tc.before, 8)
		v.SetSelection(textedit.Selection{Anchor: n, Caret: n})
		ok, err := v.deleteIndent()
		if !ok || err != nil || v.document.model.Text() != tc.after || v.Selection().Caret != tc.caret {
			t.Fatal(tc, v.document.model.Text(), v.Selection(), err)
		}
		if ok := v.Undo(); !ok || v.document.model.Text() != tc.before || v.Selection().Caret != n {
			t.Fatal("undo", ok, v.document.model.Text(), v.Selection())
		}
	}
}
func TestSmartEdgesAndShift(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("    word  ")
	observe(t, v)
	v.context = &shapedContext{}
	t.Cleanup(v.dropLayouts)
	v.SetSelection(textedit.Selection{Anchor: 6, Caret: 6})
	for _, tc := range []struct {
		key           events.Key
		shift         bool
		anchor, caret int
	}{{events.KeyHome, false, 4, 4}, {events.KeyHome, false, 0, 0}, {events.KeyEnd, true, 0, 8}, {events.KeyEnd, true, 0, 10}} {
		v.navigate(tc.key, tc.shift)
		if s := v.Selection(); s.Anchor != tc.anchor || s.Caret != tc.caret {
			t.Fatal(tc, s)
		}
	}
}
func TestAssistancePaintDoesNotChangeGeometry(t *testing.T) {
	v := NewSourceView()
	v.SetFoldingEnabled(false)
	v.document.model.SetText(" \tx \n")
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	p := v.paragraph(0)
	created := ctx.created
	for _, tc := range []struct {
		mode        WhitespaceMode
		end         bool
		dots, lines int
	}{{WhitespaceAll, true, 2, 7}, {WhitespaceTrailing, false, 1, 0}, {WhitespaceNone, true, 0, 4}, {WhitespaceSelection, false, 1, 0}} {
		_ = v.SetWhitespaceMode(tc.mode)
		v.SetShowLineEndings(tc.end)
		v.SetSelection(textedit.Selection{Caret: 1})
		out := &bracketPainter{}
		v.paintWhitespace(out, 0, p, geometry.Point{})
		if len(out.fills) != tc.dots || out.lines != tc.lines {
			t.Fatal(tc, len(out.fills), out.lines)
		}
		if v.paragraph(0) != p || ctx.created != created {
			t.Fatal("display options reshaped text")
		}
	}
	_ = v.SetRulers([]int{80, 40, 80})
	copy := v.Rulers()
	copy[0] = 1
	if !reflect.DeepEqual(v.Rulers(), []int{40, 80}) {
		t.Fatal("rulers alias or normalization")
	}
	if v.SetRulers([]int{-1}) == nil || v.SetWhitespaceMode(99) == nil {
		t.Fatal("invalid options accepted")
	}
}
func TestOccurrenceRequiresWholeSelectedWord(t *testing.T) {
	v := NewSourceView()
	v.SetFoldingEnabled(false)
	v.document.model.SetText("foo food foo")
	observe(t, v)
	v.context = &shapedContext{}
	t.Cleanup(v.dropLayouts)
	v.SetSelection(textedit.Selection{Caret: 3})
	v.SetSearch(SearchQuery{Text: "food"})
	word := v.occurrenceWord()
	if word != "foo" {
		t.Fatal(word)
	}
	out := &bracketPainter{}
	v.SetShowIndentGuides(false)
	v.paintAssistanceBackground(out, 0, v.paragraph(0), geometry.Point{}, word)
	if len(out.fills) != 1 || out.fills[0].rect.X != 90 || out.fills[0].rect.Width != 30 || v.Search().Text != "food" {
		t.Fatal(out.fills, v.Search())
	}
	v.SetSelection(textedit.Selection{Anchor: 4, Caret: 7})
	if v.occurrenceWord() != "" {
		t.Fatal("partial word highlighted")
	}
}
func TestGoToOffsetUsesOneBasedRuneColumns(t *testing.T) {
	m := textedit.NewModel("")
	m.SetText("abc\n中e\u0301x\n")
	for _, tc := range []struct {
		query string
		at    int
		valid bool
	}{{"1", 0, true}, {"2:2", 7, true}, {"2:4", 10, true}, {"3:1", 12, true}, {"0", 0, false}, {"2:6", 0, false}, {"1:0", 0, false}, {"1:2:3", 0, false}, {"no", 0, false}} {
		at, err := gotoOffset(m, tc.query)
		if (err == nil) != tc.valid || tc.valid && at != tc.at {
			t.Fatal(tc, at, err)
		}
	}
}

func TestGoToRetargetHonorsCloseCallback(t *testing.T) {
	bar := NewGoToBar()
	a, b, c := NewSourceView(), NewSourceView(), NewSourceView()
	bar.SetTarget(a)
	bar.open = true
	bar.ConnectOpen(func(open bool) {
		if !open {
			bar.SetTarget(c)
		}
	})
	bar.SetTarget(b)
	if bar.Target() != c {
		t.Fatal("outer setter overwrote callback target")
	}
}
func TestFoldBadgeExtentDoesNotDependOnWhitespaceFlag(t *testing.T) {
	v := foldingFixture(t)
	if !v.Fold(0) {
		t.Fatal("fold")
	}
	v.LayoutVisible(geometry.Size{Width: 30, Height: 200}, geometry.Point{})
	before := v.ContentSize()
	v.SetShowLineEndings(true)
	p := v.paragraph(0)
	r := v.foldBadgeRect(0, p)
	if v.ContentSize() != before || r.X+r.Width > v.ContentSize().Width {
		t.Fatal("badge clipped or marker changed content width", r, v.ContentSize(), before)
	}
}
