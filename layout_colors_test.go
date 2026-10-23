package sourceview

import (
	"image/color"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
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

func TestColorChangesReuseParagraph(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	original := p.layout.(*shapedLayout)
	created := ctx.created
	height, width := v.heights.Total(), v.contentWidth
	for i := 0; i < 20; i++ {
		v.SetSelection(textedit.Selection{Anchor: i % 4, Caret: 8})
		v.SetScheme([]*scheme.Scheme{scheme.Dark(), scheme.Light()}[i%2])
		if got := v.paragraph(0); got != p || got.layout != original || ctx.created != created {
			t.Fatal("color-only change recreated native paragraph", ctx.created, created)
		}
		if v.heights.Total() != height || v.contentWidth != width {
			t.Fatal("color-only change invalidated size estimates")
		}
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("native layout leak", ctx.created, ctx.destroyed)
	}
}

func TestLanguageChangePreservesTextLayoutAndClearsTokens(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	if native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("fixture lacks property color", native.colors[2])
	}
	created := ctx.created
	id := v.Document().engine.Line(0).ID
	if err := v.Document().SetLanguage(language.Plain); err != nil {
		t.Fatal(err)
	}
	v.Document().engine.Advance(time.Time{})
	if v.Document().engine.Line(0).ID != id {
		t.Fatal("language changed text-line identity")
	}
	if got := v.paragraph(0); got != p || ctx.created != created {
		t.Fatal("language change recreated paragraph")
	}
	for _, c := range native.colors {
		if c != v.colors.Foreground {
			t.Fatal("obsolete token color remained", c)
		}
	}
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.Document().engine.Advance(time.Time{})
	if got := v.paragraph(0); got != p || ctx.created != created || native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("language recolor failed")
	}
}

func TestUnchangedColorStateDoesNoNativeWork(t *testing.T) {
	v, _ := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	calls := native.colorCalls
	for i := 0; i < 10; i++ {
		if v.paragraph(0) != p {
			t.Fatal("cache miss")
		}
	}
	if native.colorCalls != calls {
		t.Fatal("unchanged paragraph recolored")
	}
}

func TestColdIndexAdoptsPlainLayout(t *testing.T) {
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	v.Document().Model().SetText("true")
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	p := v.paragraph(0)
	created := ctx.created
	v.Document().engine.Advance(time.Time{})
	if got := v.paragraph(0); got != p || ctx.created != created {
		t.Fatal("initial index completion reshaped unchanged text")
	}
	native := p.layout.(*shapedLayout)
	for _, c := range native.colors {
		if c != v.colors.Tokens[language.KindKeyword] {
			t.Fatal("first lexical colors were not applied")
		}
	}
}

func TestRecolorReentryStopsUsingReleasedLayout(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	native.onColor = func() { native.onColor = nil; v.SetDocument(NewDocument("replacement")) }
	v.SetScheme(scheme.Dark())
	if got := v.paragraph(0); got != nil {
		t.Fatal("reentrant document switch returned stale paragraph")
	}
	if !native.dead || v.Document().Model().Text() != "replacement" {
		t.Fatal("reentry was not completed")
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("layout ownership leak", ctx.created, ctx.destroyed)
	}
}

func TestGutterRecolorKeepsNativeNumber(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	number, err := ctx.NewTextLayout("1", typography.TextFormat{TextColor: v.colors.GutterForeground}, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	p.number, p.numberText = number, "1"
	v.SetScheme(scheme.Dark())
	if v.paragraph(0) != p || p.number != number {
		t.Fatal("gutter color rebuilt native number")
	}
	if got := number.(*shapedLayout).colors[0]; got != v.colors.GutterForeground {
		t.Fatal("gutter retained old scheme", got)
	}
}

func TestSameIDWithoutTokenizerClearsNativeColors(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	if native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("fixture has no syntax color")
	}
	created := ctx.created
	id := v.document.engine.Line(0).ID
	uncolored, err := language.New(language.Definition{Info: language.Info{ID: language.JSON, Name: "Uncolored JSON"}})
	if err != nil {
		t.Fatal(err)
	}
	v.document.SetLanguageDefinition(uncolored)
	if got := v.paragraph(0); got != p || ctx.created != created || v.document.engine.Line(0).ID != id {
		t.Fatal("definition recreated text layout or line identity")
	}
	for _, c := range native.colors {
		if c != v.colors.Foreground {
			t.Fatal("same-ID definition retained obsolete syntax colors", c)
		}
	}
	if err := v.document.SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.document.engine.Advance(time.Time{})
	if got := v.paragraph(0); got != p || ctx.created != created || native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("same-ID tokenizer restore failed")
	}
}

func TestSelectionStylesPreserveSyntaxByDefault(t *testing.T) {
	for _, sheet := range []style.StyleSheet{basic.Sheet(), modern.Sheet(modern.Options{}), modern.Sheet(modern.Options{Dark: true})} {
		selected := sheet.Resolve(style.Sel{Name: "source-view", Part: "selection"})
		fg, ok := selected.ForegroundColor()
		if !ok || nrgba(fg) != (color.NRGBA{}) {
			t.Fatal("default selection inherited a uniform foreground", fg)
		}
		if c := background(selected); c.A != 64 {
			t.Fatal("selection background is preblended or has wrong alpha", c)
		}
		if c := background(sheet.Resolve(style.Sel{Name: "source-view", Part: "inactive-selection"})); c.A != 32 {
			t.Fatal("inactive selection is opaque or too strong", c)
		}
	}
}

func TestSelectionForegroundOverridesAndReset(t *testing.T) {
	rules := modern.Rules(modern.Options{Dark: true})
	rules = append(rules, style.Name("source-view").ForegroundColor(rgb(0xeeddcc)))
	v := NewSourceView()
	scope := themedScope(v, style.Sheet(rules...))
	v.StyleChanged()
	if v.colors.SelectionForeground != (color.NRGBA{}) {
		t.Fatal("root foreground leaked into selection")
	}
	rules = append(rules, style.Name("source-view").Part("selection").ForegroundColor(rgb(0xffeeaa)))
	scope.SetStyleSheet(style.Sheet(rules...))
	v.StyleChanged()
	if v.colors.SelectionForeground != rgb(0xffeeaa) {
		t.Fatal("explicit selection foreground was ignored")
	}
	rules = append(rules, style.Name("source-view").Part("selection").ForegroundColor(color.NRGBA{R: 255}))
	scope.SetStyleSheet(style.Sheet(rules...))
	v.StyleChanged()
	if v.colors.SelectionForeground != (color.NRGBA{}) {
		t.Fatal("reset did not restore syntax foregrounds")
	}
	before := v.appliedScheme
	v.StyleChanged()
	if v.appliedScheme != before {
		t.Fatal("ignored RGB in an unset foreground invalidated the scheme cache")
	}
}

func TestSelectionMovementWithoutOverrideDoesNotRecolor(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	calls, created := native.colorCalls, ctx.created
	height, width := v.heights.Total(), v.contentWidth
	for _, selected := range []textedit.Selection{{Anchor: 0, Caret: 7}, {Anchor: 12, Caret: 2}, {}, {Anchor: 0, Caret: 15}} {
		v.SetSelection(selected)
		if v.paragraph(0) != p || native.colorCalls != calls || ctx.created != created || v.heights.Total() != height || v.contentWidth != width {
			t.Fatal("background-only selection changed native colors or geometry")
		}
		if native.colors[2] != scheme.Light().Color(language.KindProperty) || native.colors[10] != scheme.Light().Color(language.KindString) {
			t.Fatal("selection changed property/string syntax colors")
		}
	}
}
