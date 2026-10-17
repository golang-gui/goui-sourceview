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
	selection := base.Resolve(style.Sel{Name: "text-view", Part: "selection"})
	d.SelectionBackground, d.SelectionForeground = background(selection), foreground(selection)
	rules := codeRules(d)
	menu := base.Resolve(style.Sel{Name: "menu"})
	input := base.Resolve(style.Sel{Name: "text-input"})
	focus := base.Resolve(style.Sel{Name: "text-input", State: style.Focused})
	border, _ := input.BorderColor()
	menuBorder, _ := menu.BorderColor()
	focusBorder, _ := focus.BorderColor()
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
		SelectedForeground: foreground(menu), SelectedDetailForeground: muted, MatchForeground: accent, SelectedMatchForeground: accent,
		ScrollThumb: background(base.Resolve(style.Sel{Name: "scroll-view", Part: "thumb"}))}
	rules = append(rules, completionRules(c, base, o.FontSize)...)
	return rules
}
