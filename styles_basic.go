package sourceview

import (
	"image/color"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
)

func init() { basic.Register(sourceStyleID, basicRules) }
func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}
func basicRules(base style.StyleSheet) []style.Rule {
	rules := codeRules(scheme.Light().Definition())
	c := completionColors{
		Background: rgb(0xf8fafc), Foreground: rgb(0x24292f), DetailForeground: rgb(0x626d7a), Border: rgb(0xb9c3cf),
		SelectedBackground: rgb(0xe3edf9), SelectedForeground: rgb(0x172b44), SelectedDetailForeground: rgb(0x4e627b),
		MatchForeground: rgb(0x005fb8), SelectedMatchForeground: rgb(0x0055a5),
	}
	f := findColors{Background: rgb(0xf3f3f3), Foreground: rgb(0x333333), MutedForeground: rgb(0x6c6c6c), Border: rgb(0xd0d0d0), InputBackground: rgb(0xffffff),
		InputBorder: rgb(0xcecece), FocusBorder: rgb(0x0078d4), HoverBackground: rgb(0xe1e1e1), OptionBackground: rgb(0xcce5ff), OptionBorder: rgb(0x0078d4), ErrorForeground: rgb(0xb42318), Shadow: color.NRGBA{A: 42}}
	rules = append(rules, findRules(base, f, 9.75, 4, 2, 1)...)
	shadow, _ := base.Resolve(style.Sel{Name: "menu"}).Shadow()
	rules = append(rules, completionRules(c, base, 12, 4, shadow)...)
	return rules
}
