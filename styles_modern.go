package sourceview

import (
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/modern"
)

func init() { modern.Register(sourceStyleID, modernRules) }
func modernRules(o modern.Options, base style.StyleSheet) []style.Rule {
	s := scheme.Light()
	if o.Dark {
		s = scheme.Dark()
	}
	d := s.Definition()
	root := base.Resolve(style.Sel{Name: "text-view"})
	d.Background, d.Foreground = background(root), foreground(root)
	// Source selection preserves syntax foregrounds and blends a real tint under
	// the text. TextView's resolved selection inherits its ordinary foreground
	// and uses a preblended, opaque background, which is a different contract.
	focus := base.Resolve(style.Sel{Name: "text-input", State: style.Focused})
	focusBorder, _ := focus.BorderColor()
	d.SelectionBackground = nrgba(focusBorder)
	d.SelectionBackground.A = 64
	d.InactiveSelectionBackground = d.SelectionBackground
	d.InactiveSelectionBackground.A = 32
	rules := codeRules(d)
	menu := base.Resolve(style.Sel{Name: "menu"})
	input := base.Resolve(style.Sel{Name: "text-input"})
	border, _ := input.BorderColor()
	menuBorder, _ := menu.BorderColor()
	shadow, _ := menu.Shadow()
	muted := foreground(base.Resolve(style.Sel{Name: modern.MutedText}))
	accent := foreground(base.Resolve(style.Sel{Name: modern.AccentIcon}))
	hover := background(base.Resolve(style.Sel{Name: "menu-item", State: style.Hovered}))
	selected := background(base.Resolve(style.Sel{Name: "text-input", Part: "selection"}))
	f := findColors{Background: background(menu), Foreground: foreground(input), MutedForeground: muted, Border: nrgba(menuBorder), InputBackground: background(input), InputBorder: nrgba(border), FocusBorder: nrgba(focusBorder),
		HoverBackground: hover, OptionBackground: selected, OptionBorder: nrgba(focusBorder), ErrorForeground: foreground(base.Resolve(style.Sel{Name: modern.Error})), Shadow: nrgba(shadow.Color)}
	radius, _ := menu.Radius()
	inputRadius, _ := input.Radius()
	focusWidth, _ := focus.BorderWidth()
	rules = append(rules, findRules(base, f, o.FontSize, radius, inputRadius, focusWidth)...)
	c := completionColors{Background: background(menu), Foreground: foreground(menu), DetailForeground: muted, Border: nrgba(menuBorder), SelectedBackground: selected,
		SelectedForeground: foreground(menu), SelectedDetailForeground: muted, MatchForeground: accent, SelectedMatchForeground: accent}
	rules = append(rules, completionRules(c, base, o.FontSize, radius, shadow)...)
	return rules
}
