package sourceview

import (
	"image/color"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/style"
)

func findRules(base style.StyleSheet, c findColors, size, radius, inputRadius, focusWidth float32) []style.Rule {
	family, _ := base.Resolve(style.Sel{Name: "label"}).FontFamily()
	selection := base.Resolve(style.Sel{Name: "text-input", Part: "selection"})
	rules := []style.Rule{
		style.Name("sourceview.find").BackgroundColor(c.Background).ForegroundColor(c.Foreground).BorderColor(c.Border).BorderWidth(1).Radius(radius).
			Shadow(style.Shadow{Color: c.Shadow, Offset: geometry.Point{Y: 2}, BlurRadius: 4}),
		style.Name("sourceview.find.input").FontFamily(family).FontSize(size).ForegroundColor(c.Foreground).BackgroundColor(transparent).BorderWidth(0),
		style.Name("sourceview.find.input").State(style.Focused).BorderWidth(0),
		style.Name("sourceview.find.input").State(style.Disabled).ForegroundColor(c.MutedForeground),
		{Sel: style.Sel{Name: "sourceview.find.input", Part: "selection"}, Style: selection},
		style.Name("sourceview.find.input").Part("caret").ForegroundColor(c.Foreground),
		style.Name("sourceview.find.input").Part("placeholder").ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.input").Part("frame").BackgroundColor(c.InputBackground).BorderColor(c.InputBorder).BorderWidth(1).Radius(inputRadius),
		style.Name("sourceview.find.input").Part("frame").State(style.Focused).BorderColor(c.FocusBorder).BorderWidth(focusWidth),
		style.Name("sourceview.find.input").Part("error-frame").BackgroundColor(c.InputBackground).BorderColor(c.ErrorForeground).BorderWidth(focusWidth).Radius(inputRadius),
		style.Name("sourceview.find.input").Part("frame").State(style.Disabled).BorderColor(c.InputBorder).BorderWidth(1),
		style.Name("sourceview.find.action").BackgroundColor(transparent).BorderWidth(0).Radius(inputRadius),
		style.Name("sourceview.find.action").State(style.Hovered).BackgroundColor(c.HoverBackground),
		style.Name("sourceview.find.action").State(style.Pressed).BackgroundColor(c.OptionBackground),
		style.Name("sourceview.find.action").Part("focus").State(style.Focused).BackgroundColor(transparent).BorderWidth(0),
		style.Name("sourceview.find.action").Part("focus").State(style.FocusVisible).BorderColor(c.FocusBorder).BorderWidth(1).Radius(inputRadius),
		style.Name("sourceview.find.icon").ForegroundColor(c.Foreground), style.Name("sourceview.find.icon").State(style.Disabled).ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.option-text").FontFamily(family).FontSize(size * .92).ForegroundColor(c.Foreground),
		style.Name("sourceview.find.option-text").State(style.Disabled).ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.status").FontFamily(family).FontSize(size * .92).ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.error").FontFamily(family).FontSize(size * .92).ForegroundColor(c.ErrorForeground),
	}
	for _, name := range []string{"sourceview.find.option", "sourceview.find.expand"} {
		for _, part := range []string{"button", "button-checked"} {
			bg, border := transparent, transparent
			if part == "button-checked" && name != "sourceview.find.expand" {
				bg = c.OptionBackground
				border = c.OptionBorder
			}
			rules = append(rules, style.Name(name).Part(part).BackgroundColor(bg).BorderColor(border).BorderWidth(1).Radius(inputRadius).FontFamily(family).FontSize(size),
				style.Name(name).Part(part).State(style.Hovered).BackgroundColor(c.HoverBackground), style.Name(name).Part(part).State(style.Pressed).BackgroundColor(c.OptionBackground))
		}
		rules = append(rules, style.Name(name).Part("focus").State(style.FocusVisible).BorderColor(c.FocusBorder).BorderWidth(1).Radius(inputRadius))
	}
	return rules
}

func completionRules(c completionColors, base style.StyleSheet, size, radius, selectedRadius float32, shadow style.Shadow) []style.Rule {
	family, _ := base.Resolve(style.Sel{Name: "label"}).FontFamily()
	return []style.Rule{
		style.Name("sourceview.completion").BackgroundColor(c.Background).ForegroundColor(c.Foreground).BorderColor(c.Border).BorderWidth(1).Radius(radius).Shadow(shadow).FontFamily(family).FontSize(size),
		style.Name("sourceview.completion").Part("detail").ForegroundColor(c.DetailForeground).FontSize(size * .9),
		style.Name("sourceview.completion").Part("selected").BackgroundColor(c.SelectedBackground).ForegroundColor(c.SelectedForeground).Radius(selectedRadius),
		style.Name("sourceview.completion").Part("selected-detail").ForegroundColor(c.SelectedDetailForeground),
		style.Name("sourceview.completion").Part("match").ForegroundColor(c.MatchForeground),
		style.Name("sourceview.completion").Part("selected-match").ForegroundColor(c.SelectedMatchForeground),
	}
}

var transparent = color.NRGBA{}

type completionColors struct {
	Background, Foreground, DetailForeground color.NRGBA
	Border                                   color.NRGBA
	SelectedBackground, SelectedForeground   color.NRGBA
	SelectedDetailForeground                 color.NRGBA
	MatchForeground, SelectedMatchForeground color.NRGBA
}

type findColors struct {
	Background, Foreground, MutedForeground           color.NRGBA
	Border, InputBackground, InputBorder, FocusBorder color.NRGBA
	HoverBackground, OptionBackground, OptionBorder   color.NRGBA
	ErrorForeground, Shadow                           color.NRGBA
}
