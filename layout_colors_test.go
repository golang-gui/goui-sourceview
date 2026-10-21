package sourceview

import (
	"image/color"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestPlainIndexCompletionDoesNotMutateNativeColors(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("ordinary text without syntax colors")
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	if native.colorCalls != 0 {
		t.Fatal("plain layout overwrote the foreground already supplied in TextFormat")
	}
	v.Document().engine.Advance(time.Time{})
	if v.paragraph(0) != p || native.colorCalls != 0 || !p.style.tokenValid {
		t.Fatal("index completion changed native colors despite identical rendered text")
	}
}

func TestEquivalentTokenColorsDoNotMutateNativeLayout(t *testing.T) {
	v, _ := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	tokens, _, _ := v.Document().engine.Tokens(0)
	tokens = append([]highlight.Token(nil), tokens...)
	before := append([]color.NRGBA(nil), native.colors...)
	calls := native.colorCalls
	// Different lexical categories can resolve to the same theme color. Split
	// one token into adjacent ranges as well: rendered colors are unchanged.
	for i, token := range tokens {
		if token.End-token.Start > 1 {
			tokens = append(tokens[:i:i], append([]highlight.Token{{Start: token.Start, End: token.Start + 1, Kind: token.Kind}, {Start: token.Start + 1, End: token.End, Kind: token.Kind}}, tokens[i+1:]...)...)
			break
		}
	}
	style := p.style
	style.tokenVersion++
	if !v.colorParagraph(v.paragraphKey(0), p, style, tokens, false) || native.colorCalls != calls || p.style != style {
		t.Fatal("equivalent color ranges invalidated native text or failed to advance style identity")
	}
	for i, c := range before {
		if native.colors[i] != c {
			t.Fatal("unchanged colors were lost", i)
		}
	}
}

func TestParagraphColorsPreserveSelectionAndClearObsoleteTokens(t *testing.T) {
	v, _ := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	tokens, _, _ := v.Document().engine.Tokens(0)
	v.colors.SelectionForeground = color.NRGBA{R: 100, G: 10, B: 50, A: 255}
	// Exercise effective range construction directly; focus/lifetime validation
	// still uses the same inactive fixture as the other color tests.
	style := p.style
	style.focused, style.selection = true, textedit.Range{Start: 3, End: 9}
	if !v.colorParagraph(v.paragraphKey(0), p, style, tokens, false) {
		t.Fatal("selection color application failed")
	}
	for i := 3; i < 9; i++ {
		if native.colors[i] != v.colors.SelectionForeground {
			t.Fatal("selection did not override token color", i)
		}
	}
	style.focused, style.selection, style.tokenValid = false, textedit.Range{}, false
	style.language = language.JSON
	if !v.colorParagraph(v.paragraphKey(0), p, style, nil, false) {
		t.Fatal("clearing token and selection colors failed")
	}
	for i, c := range native.colors {
		if c != v.colors.Foreground {
			t.Fatal("obsolete token or selection foreground survived", i)
		}
	}
}
