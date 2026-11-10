package controls

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/modern"
	"image/color"
	"math"
)

func nrgba(c color.Color) color.NRGBA {
	if c == nil {
		return color.NRGBA{}
	}
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

func foreground(s style.Style) color.NRGBA { c, _ := s.ForegroundColor(); return nrgba(c) }

func background(s style.Style) color.NRGBA { c, _ := s.BackgroundColor(); return nrgba(c) }

func styleFont(s style.Style) typography.FontInfo {
	family, _ := s.FontFamily()
	size, _ := s.FontSize()
	return typography.FontInfo{Family: family, Size: size}
}

// Box values are copied to ordinary comparable data; native resources stay on
// the consuming widget. An explicit transparent fill/border/shadow is retained.
type Box struct {
	Background, Border graphics.Color
	Width              float32
	Radius             geometry.CornerRadius
	Sides              style.BorderSides
	Shadow             graphics.BoxShadow
}

// Keep cached radii comparable even when an application supplies NaN/Inf.
// Geometry normalization against the allocated size remains GOUI's job.
func cleanCornerRadius(r geometry.CornerRadius) geometry.CornerRadius {
	clean := func(v float32) float32 {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return 0
		}
		return max(0, v)
	}
	return geometry.CornerRadius{TopLeft: clean(r.TopLeft), TopRight: clean(r.TopRight), BottomRight: clean(r.BottomRight), BottomLeft: clean(r.BottomLeft)}
}

func BoxOf(s style.Style) Box {
	bg, _ := s.BackgroundColor()
	border, _ := s.BorderColor()
	width, _ := s.BorderWidth()
	radius, _ := s.CornerRadius()
	sides, set := s.BorderSides()
	if !set {
		sides = style.BorderAll
	}
	shadow, _ := s.Shadow()
	return Box{graphics.ColorOf(nrgba(bg)), graphics.ColorOf(nrgba(border)), max(0, width), cleanCornerRadius(radius), sides, graphics.BoxShadow{
		Color: graphics.ColorOf(nrgba(shadow.Color)), Offset: shadow.Offset, BlurRadius: max(0, shadow.BlurRadius), SpreadRadius: shadow.SpreadRadius,
	}}
}

func (b Box) Paint(p gui.Painter, r geometry.Rectangle) {
	if r.Width <= 0 || r.Height <= 0 {
		return
	}
	if b.Shadow.Color.A > 0 {
		p.DrawBoxShadow(r, b.Radius, b.Shadow)
	}
	gui.PaintStyledBox(p, r, style.Rule{}.BackgroundColor(b.Background).BorderColor(b.Border).
		BorderWidth(b.Width).BorderSides(b.Sides).CornerRadius(b.Radius).Style)
}

func (b Box) Outset() float32 {
	if b.Shadow.Color.A <= 0 {
		return 0
	}
	// DrawBoxShadow's documented sigma is blur/2, with a 3-sigma extent.
	return float32(math.Ceil(float64(max(0, b.Shadow.BlurRadius*1.5+b.Shadow.SpreadRadius) + float32(max(math.Abs(float64(b.Shadow.Offset.X)), math.Abs(float64(b.Shadow.Offset.Y)))))))
}

func FindRules(base style.StyleSheet, c FindColors, size float32, radius, inputRadius geometry.CornerRadius, focusWidth float32) []style.Rule {
	family, _ := base.Resolve(style.Sel{Name: "label"}).FontFamily()
	selection := base.Resolve(style.Sel{Name: "text-input", Part: "selection"})
	rules := []style.Rule{
		style.Name("sourceview.find").BackgroundColor(c.Background).ForegroundColor(c.Foreground).BorderColor(c.Border).BorderWidth(1).CornerRadius(radius).
			Shadow(style.Shadow{Color: c.Shadow, Offset: geometry.Point{Y: 2}, BlurRadius: 4}),
		style.Name("sourceview.find.input").FontFamily(family).FontSize(size).ForegroundColor(c.Foreground).BackgroundColor(transparent).BorderWidth(0),
		style.Name("sourceview.find.input").State(style.Focused).BorderWidth(0),
		style.Name("sourceview.find.input").State(style.Disabled).ForegroundColor(c.MutedForeground),
		{Sel: style.Sel{Name: "sourceview.find.input", Part: "selection"}, Style: selection},
		style.Name("sourceview.find.input").Part("caret").ForegroundColor(c.Foreground),
		style.Name("sourceview.find.input").Part("placeholder").ForegroundColor(c.MutedForeground),
		style.Name("sourceview.find.input").Part("frame").BackgroundColor(c.InputBackground).BorderColor(c.InputBorder).BorderWidth(1).CornerRadius(inputRadius),
		style.Name("sourceview.find.input").Part("frame").State(style.Focused).BorderColor(c.FocusBorder).BorderWidth(focusWidth),
		style.Name("sourceview.find.input").Part("error-frame").BackgroundColor(c.InputBackground).BorderColor(c.ErrorForeground).BorderWidth(focusWidth).CornerRadius(inputRadius),
		style.Name("sourceview.find.input").Part("frame").State(style.Disabled).BorderColor(c.InputBorder).BorderWidth(1),
		style.Name("sourceview.find.action").BackgroundColor(transparent).BorderWidth(0).CornerRadius(inputRadius),
		style.Name("sourceview.find.action").State(style.Hovered).BackgroundColor(c.HoverBackground),
		style.Name("sourceview.find.action").State(style.Pressed).BackgroundColor(c.OptionBackground),
		style.Name("sourceview.find.action").Part("focus").State(style.Focused).BackgroundColor(transparent).BorderWidth(0),
		style.Name("sourceview.find.action").Part("focus").State(style.FocusVisible).BorderColor(c.FocusBorder).BorderWidth(1).CornerRadius(inputRadius),
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
			rules = append(rules, style.Name(name).Part(part).BackgroundColor(bg).BorderColor(border).BorderWidth(1).CornerRadius(inputRadius).FontFamily(family).FontSize(size),
				style.Name(name).Part(part).State(style.Hovered).BackgroundColor(c.HoverBackground), style.Name(name).Part(part).State(style.Pressed).BackgroundColor(c.OptionBackground))
		}
		rules = append(rules, style.Name(name).Part("focus").State(style.FocusVisible).BorderColor(c.FocusBorder).BorderWidth(1).CornerRadius(inputRadius))
	}
	return rules
}

func CompletionRules(c CompletionColors, base style.StyleSheet, size float32, radius, selectedRadius geometry.CornerRadius, shadow style.Shadow) []style.Rule {
	family, _ := base.Resolve(style.Sel{Name: "label"}).FontFamily()
	return []style.Rule{
		style.Name("sourceview.completion").BackgroundColor(c.Background).ForegroundColor(c.Foreground).BorderColor(c.Border).BorderWidth(1).CornerRadius(radius).Shadow(shadow).FontFamily(family).FontSize(size),
		style.Name("sourceview.completion").Part("detail").ForegroundColor(c.DetailForeground).FontSize(size * .9),
		style.Name("sourceview.completion").Part("selected").BackgroundColor(c.SelectedBackground).ForegroundColor(c.SelectedForeground).CornerRadius(selectedRadius),
		style.Name("sourceview.completion").Part("selected-detail").ForegroundColor(c.SelectedDetailForeground),
		style.Name("sourceview.completion").Part("match").ForegroundColor(c.MatchForeground),
		style.Name("sourceview.completion").Part("selected-match").ForegroundColor(c.SelectedMatchForeground),
	}
}

var transparent = color.NRGBA{}

type CompletionColors struct {
	Background, Foreground, DetailForeground color.NRGBA
	Border                                   color.NRGBA
	SelectedBackground, SelectedForeground   color.NRGBA
	SelectedDetailForeground                 color.NRGBA
	MatchForeground, SelectedMatchForeground color.NRGBA
}

type FindColors struct {
	Background, Foreground, MutedForeground           color.NRGBA
	Border, InputBackground, InputBorder, FocusBorder color.NRGBA
	HoverBackground, OptionBackground, OptionBorder   color.NRGBA
	ErrorForeground, Shadow                           color.NRGBA
}

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}
func BasicRules(base style.StyleSheet) []style.Rule {
	var rules []style.Rule
	c := CompletionColors{
		Background: rgb(0xf8fafc), Foreground: rgb(0x24292f), DetailForeground: rgb(0x626d7a), Border: rgb(0xb9c3cf),
		SelectedBackground: rgb(0xe3edf9), SelectedForeground: rgb(0x172b44), SelectedDetailForeground: rgb(0x4e627b),
		MatchForeground: rgb(0x005fb8), SelectedMatchForeground: rgb(0x0055a5),
	}
	f := FindColors{Background: rgb(0xf3f3f3), Foreground: rgb(0x333333), MutedForeground: rgb(0x6c6c6c), Border: rgb(0xd0d0d0), InputBackground: rgb(0xffffff),
		InputBorder: rgb(0xcecece), FocusBorder: rgb(0x0078d4), HoverBackground: rgb(0xe1e1e1), OptionBackground: rgb(0xcce5ff), OptionBorder: rgb(0x0078d4), ErrorForeground: rgb(0xb42318), Shadow: color.NRGBA{A: 42}}
	rules = append(rules, FindRules(base, f, 9.75, geometry.UniformCornerRadius(4), geometry.UniformCornerRadius(2), 1)...)
	shadow, _ := base.Resolve(style.Sel{Name: "menu"}).Shadow()
	rules = append(rules, CompletionRules(c, base, 12, geometry.UniformCornerRadius(4), geometry.UniformCornerRadius(2), shadow)...)
	return rules
}
func ModernRules(o modern.Options, base style.StyleSheet) []style.Rule {
	var rules []style.Rule
	focus := base.Resolve(style.Sel{Name: "text-input", State: style.Focused})
	focusBorder, _ := focus.BorderColor()
	menu := base.Resolve(style.Sel{Name: "menu"})
	input := base.Resolve(style.Sel{Name: "text-input"})
	border, _ := input.BorderColor()
	menuBorder, _ := menu.BorderColor()
	shadow, _ := menu.Shadow()
	muted := foreground(base.Resolve(style.Sel{Name: modern.MutedText}))
	accent := foreground(base.Resolve(style.Sel{Name: modern.AccentIcon}))
	hover := background(base.Resolve(style.Sel{Name: "menu-item", State: style.Hovered}))
	selected := background(base.Resolve(style.Sel{Name: "text-input", Part: "selection"}))
	f := FindColors{Background: background(menu), Foreground: foreground(input), MutedForeground: muted, Border: nrgba(menuBorder), InputBackground: background(input), InputBorder: nrgba(border), FocusBorder: nrgba(focusBorder),
		HoverBackground: hover, OptionBackground: selected, OptionBorder: nrgba(focusBorder), ErrorForeground: foreground(base.Resolve(style.Sel{Name: modern.Error})), Shadow: nrgba(shadow.Color)}
	radius, _ := menu.CornerRadius()
	inputRadius, _ := input.CornerRadius()
	focusWidth, _ := focus.BorderWidth()
	rules = append(rules, FindRules(base, f, o.FontSize, radius, inputRadius, focusWidth)...)
	c := CompletionColors{Background: background(menu), Foreground: foreground(menu), DetailForeground: muted, Border: nrgba(menuBorder), SelectedBackground: selected,
		SelectedForeground: foreground(menu), SelectedDetailForeground: muted, MatchForeground: accent, SelectedMatchForeground: accent}
	selectedRadius, _ := base.Resolve(style.Sel{Name: "menu-item"}).CornerRadius()
	rules = append(rules, CompletionRules(c, base, o.FontSize, radius, selectedRadius, shadow)...)
	return rules
}
