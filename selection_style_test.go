package sourceview

import (
	"image/color"
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

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
