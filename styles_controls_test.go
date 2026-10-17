package sourceview

import (
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
	"image/color"
	"testing"
)

// The former scheme UI palette tests live here, where those colors are owned.
func TestControlPalettesAreIndependentOfCodeSchemes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		v := NewSourceView()
		sheet := modern.Sheet(modern.Options{Dark: dark, FontFamily: "ui-font", FontSize: 19, AccentColor: rgb(0x357f40)})
		themedScope(v, sheet)
		l := newCompletionList(v, &completionSession{})
		b := NewFindBar()
		// A companion uses its own explicit tree location; SetTarget does not
		// transfer a stylesheet from an unrelated subtree.
		themedScope(b, sheet)
		b.StyleChanged()
		before, find := l.colors, b.appearance
		for _, c := range []color.NRGBA{before.Background, before.Foreground, before.DetailForeground, before.SelectedForeground, before.SelectedDetailForeground} {
			if c.A != 255 {
				t.Fatal("unreadable built-in popup surface/text", c)
			}
		}
		if l.font.Family != "ui-font" || l.font.Size != 19 || l.detailFont().Size != 19*.9 || b.appearance.surface.radius != 8 || b.appearance.focused.width != 2 {
			t.Fatal("Modern font, radius or focus metrics were not used")
		}
		if got := foreground(gui.ResolveWidgetStyle(b.find, "sourceview.find.input", "", style.Normal)); got.A != 255 || background(gui.ResolveWidgetStyle(b.find, "sourceview.find.input", "frame", style.Normal)).A != 255 {
			t.Fatal("unreadable built-in find surface/text")
		}
		d := scheme.Dark().Definition()
		d.SelectionBackground, d.SelectionForeground = rgb(0xff0000), rgb(0x00ff00)
		s, err := scheme.New(d)
		if err != nil {
			t.Fatal(err)
		}
		v.SetScheme(s)
		l.readAppearance()
		b.SetTarget(v)
		b.StyleChanged()
		if l.colors != before || b.appearance != find {
			t.Fatal("code selection changed UI palettes")
		}
		if got := foreground(sheet.Resolve(style.Sel{Name: "icon"})); got != foreground(modern.Sheet(modern.Options{Dark: dark}).Resolve(style.Sel{Name: "icon"})) {
			t.Fatal("SourceView registration modified the global icon style")
		}
	}
}

func TestApplicationOverridesAndPopupOwnerScope(t *testing.T) {
	rules := modern.Rules(modern.Options{Dark: true})
	rules = append(rules,
		style.Name("sourceview.completion").BorderColor(color.NRGBA{}).FontFamily("owner-font").FontSize(16),
		style.Name("sourceview.completion").Part("scroll-thumb").BackgroundColor(color.NRGBA{R: 200, A: 80}),
		style.Name("sourceview.find").Shadow(style.Shadow{}),
		style.Name("sourceview.find.option").Part("button-checked").BorderColor(color.NRGBA{R: 90, A: 80}),
	)
	v := NewSourceView()
	owner := themedScope(v, style.Sheet(rules...))
	l := newCompletionList(v, &completionSession{})
	// Popup roots may have a completely different application sheet. Resolution
	// is explicitly tied to the owner, before and after attachment to that root.
	themedScope(l, basic.Sheet())
	l.StyleChanged()
	if l.font.Family != "owner-font" || l.font.Size != 16 || l.colors.Border.A != 0 || l.colors.ScrollThumb != (color.NRGBA{R: 200, A: 80}) {
		t.Fatal("popup lost its owner scope or decorative alpha")
	}
	b := NewFindBar()
	themedScope(b, owner.StyleSheet())
	b.StyleChanged()
	border, _ := gui.ResolveWidgetStyle(b.matchCase, "sourceview.find.option", "button-checked", style.Normal).BorderColor()
	if b.appearance.surface.shadow.Color.A != 0 || nrgba(border).A != 80 || b.inset != 2 {
		t.Fatal("find overrides lost explicit alpha or retained shadow space")
	}
}

func TestCompletionThemeChangesReleaseOwnedLayouts(t *testing.T) {
	l, ctx := completionFixture(t, []Completion{{Label: "transform", Detail: "JSON"}})
	owner := themedScope(l.view, modern.Sheet(modern.Options{}))
	l.StyleChanged()
	arrangeCompletion(l)
	l.Paint(&completionPainter{})
	created, destroyed, epoch := ctx.created, ctx.destroyed, l.epoch
	l.StyleChanged()
	if ctx.created != created || ctx.destroyed != destroyed || l.epoch != epoch {
		t.Fatal("unchanged appearance released native resources")
	}
	owner.SetStyleSheet(modern.Sheet(modern.Options{Dark: true, FontSize: 18}))
	l.StyleChanged()
	if len(l.rows) != 0 || l.measured || ctx.destroyed != created {
		t.Fatal("theme change retained native layouts or old metrics")
	}
	arrangeCompletion(l)
	l.Paint(&completionPainter{})
	if l.rows[0].label.native.(*shapedLayout).format.Font.Size != 18 || l.colors.Background != rgb(0x28282d) {
		t.Fatal("popup rebuild used stale font or palette")
	}
}
