package sourceview

import (
	"image/color"
	"maps"
	"math"
	"slices"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

const sourceStyleID = "github.com/golang-gui/goui-sourceview"

// The ScrollView host owns the outer frame. Copy the official theme's parts
// explicitly; the code body and its syntax colors remain independent.
func editorScrollRules(base style.StyleSheet) []style.Rule {
	var rules []style.Rule
	for _, part := range []string{"frame", "trough", "thumb"} {
		name := "scroll-view"
		if part == "frame" {
			name = "text-view"
		}
		for _, state := range []style.State{style.Normal, style.Focused, style.FocusVisible, style.Disabled, style.Hovered, style.Pressed} {
			rule := style.Rule{Sel: style.Sel{Name: "source-view", Part: part, State: state},
				Style: base.Resolve(style.Sel{Name: name, Part: part, State: state})}
			if part != "frame" {
				rule = rule.BorderWidth(0)
			}
			rules = append(rules, rule)
		}
	}
	return rules
}

var tokenKinds = []language.Kind{language.KindHeading, language.KindCode, language.KindEmphasis, language.KindLink, language.KindFence, language.KindMarker, language.KindPlain, language.KindProperty, language.KindString, language.KindEscape, language.KindNumber, language.KindKeyword, language.KindDelimiter, language.KindTag, language.KindAttribute, language.KindEntity, language.KindComment, language.KindMeta, language.KindInvalid, language.KindVariable}

var tokenParts = func() map[language.Kind]string {
	m := make(map[language.Kind]string)
	for _, k := range tokenKinds {
		m[k] = k.String()
	}
	return m
}()

func styleKinds(tokens map[language.Kind]color.NRGBA) []language.Kind {
	kinds := slices.Clone(tokenKinds)
	var extra []language.Kind
	for k := range tokens {
		if _, ok := tokenParts[k]; !ok {
			extra = append(extra, k)
		}
	}
	slices.Sort(extra)
	return append(kinds, extra...)
}

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
type styleBox struct {
	background, border graphics.Color
	width, radius      float32
	shadow             graphics.BoxShadow
}

func boxOf(s style.Style) styleBox {
	bg, _ := s.BackgroundColor()
	border, _ := s.BorderColor()
	width, _ := s.BorderWidth()
	radius, _ := s.Radius()
	shadow, _ := s.Shadow()
	return styleBox{graphics.ColorOf(nrgba(bg)), graphics.ColorOf(nrgba(border)), max(0, width), max(0, radius), graphics.BoxShadow{
		Color: graphics.ColorOf(nrgba(shadow.Color)), Offset: shadow.Offset, BlurRadius: max(0, shadow.BlurRadius), SpreadRadius: shadow.SpreadRadius,
	}}
}

func (b styleBox) paint(p gui.Painter, r geometry.Rectangle) {
	if r.Width <= 0 || r.Height <= 0 {
		return
	}
	if b.shadow.Color.A > 0 {
		p.DrawBoxShadow(r, b.radius, b.shadow)
	}
	if b.background.A > 0 {
		if b.radius == 0 {
			p.FillRect(r, b.background)
		} else {
			p.FillRoundRect(r, b.radius, b.background)
		}
	}
	if b.width > 0 && b.border.A > 0 {
		if b.radius == 0 {
			p.DrawRect(r.Inset(b.width/2), b.width, b.border)
		} else {
			p.DrawRoundRect(r.Inset(b.width/2), max(0, b.radius-b.width/2), b.width, b.border)
		}
	}
}

func (b styleBox) outset() float32 {
	if b.shadow.Color.A <= 0 {
		return 0
	}
	// DrawBoxShadow's documented sigma is blur/2, with a 3-sigma extent.
	return float32(math.Ceil(float64(max(0, b.shadow.BlurRadius*1.5+b.shadow.SpreadRadius) + float32(max(math.Abs(float64(b.shadow.Offset.X)), math.Abs(float64(b.shadow.Offset.Y)))))))
}

func codeRules(d scheme.Definition) []style.Rule {
	rules := []style.Rule{
		style.Name("source-view").Part("indent-guide").ForegroundColor(tint(d.GutterForeground, 65)),
		style.Name("source-view").Part("whitespace").ForegroundColor(tint(d.GutterForeground, 125)),
		style.Name("source-view").Part("selected-whitespace").ForegroundColor(tint(d.Foreground, 215)),
		style.Name("source-view").Part("ruler").ForegroundColor(tint(d.GutterForeground, 45)),
		style.Name("source-view").Part("occurrence").ForegroundColor(tint(d.SearchBackground, 55)),
		style.Name("source-view").FontFamily("monospace").FontSize(12).BackgroundColor(d.Background).ForegroundColor(d.Foreground),
		style.Name("source-view").Part("gutter").BackgroundColor(d.GutterBackground).ForegroundColor(d.GutterForeground),
		style.Name("source-view").Part("fold-toggle").ForegroundColor(d.GutterForeground),
		style.Name("source-view").Part("fold-placeholder").ForegroundColor(d.GutterForeground),
		style.Name("source-view").Part("current-line").BackgroundColor(d.CurrentLine),
		// An explicit zero foreground prevents the root foreground fallback;
		// SourceView treats zero alpha here as preserving syntax, not invisible text.
		style.Name("source-view").Part("selection").BackgroundColor(d.SelectionBackground).ForegroundColor(d.SelectionForeground),
		style.Name("source-view").Part("inactive-selection").BackgroundColor(d.InactiveSelectionBackground),
		style.Name("source-view").Part("search").BackgroundColor(d.SearchBackground),
		style.Name("source-view").Part("current-search").BackgroundColor(d.CurrentSearchBackground),
		style.Name("source-view").Part("bracket").BackgroundColor(d.BracketMatchBackground).BorderColor(d.BracketMatchBorder),
		style.Name("source-view").Part("caret").ForegroundColor(d.Caret),
		style.Name("source-view").Part("preedit").ForegroundColor(d.PreeditUnderline),
		style.Name("source-view").Part("border").BorderColor(d.Border),
	}
	for _, kind := range styleKinds(d.Tokens) {
		part := kind.String()
		c, ok := d.Tokens[kind]
		if kind == language.KindPlain && !ok {
			continue
		}
		if !ok {
			c = d.Foreground
		}
		rules = append(rules, style.Name("source-view").Part("token-"+part).ForegroundColor(c))
	}
	return rules
}

func sameCodeColors(a, b scheme.Definition) bool {
	return a.Background == b.Background && a.Foreground == b.Foreground && a.GutterBackground == b.GutterBackground && a.GutterForeground == b.GutterForeground &&
		a.CurrentLine == b.CurrentLine && a.SelectionBackground == b.SelectionBackground && a.SelectionForeground == b.SelectionForeground && a.InactiveSelectionBackground == b.InactiveSelectionBackground &&
		a.SearchBackground == b.SearchBackground && a.CurrentSearchBackground == b.CurrentSearchBackground && a.BracketMatchBackground == b.BracketMatchBackground && a.BracketMatchBorder == b.BracketMatchBorder &&
		a.Caret == b.Caret && a.PreeditUnderline == b.PreeditUnderline && a.Border == b.Border && maps.Equal(a.Tokens, b.Tokens)
}

func (v *SourceView) styledScheme() (*scheme.Scheme, error) {
	name := v.StyleName()
	if name == "" {
		name = "source-view"
	}
	resolve := func(part string) style.Style { return gui.ResolveWidgetStyle(v, name, part, style.Normal) }
	d := scheme.Light().Definition()
	d.Name = "SourceView stylesheet"
	setBG := func(part string, out *color.NRGBA) {
		if c, ok := resolve(part).BackgroundColor(); ok {
			*out = nrgba(c)
		}
	}
	setFG := func(part string, out *color.NRGBA) {
		if c, ok := resolve(part).ForegroundColor(); ok {
			*out = nrgba(c)
		}
	}
	setBG("", &d.Background)
	setFG("", &d.Foreground)
	setBG("gutter", &d.GutterBackground)
	setFG("gutter", &d.GutterForeground)
	setBG("current-line", &d.CurrentLine)
	setBG("selection", &d.SelectionBackground)
	setFG("selection", &d.SelectionForeground)
	if d.SelectionForeground.A == 0 {
		d.SelectionForeground = color.NRGBA{}
	}
	setBG("inactive-selection", &d.InactiveSelectionBackground)
	setBG("search", &d.SearchBackground)
	setBG("current-search", &d.CurrentSearchBackground)
	setBG("bracket", &d.BracketMatchBackground)
	if c, ok := resolve("bracket").BorderColor(); ok {
		d.BracketMatchBorder = nrgba(c)
	}
	setFG("caret", &d.Caret)
	setFG("preedit", &d.PreeditUnderline)
	if c, ok := resolve("border").BorderColor(); ok {
		d.Border = nrgba(c)
	}
	for _, kind := range v.document.definition.Styles() {
		if _, ok := d.Tokens[kind]; !ok {
			d.Tokens[kind] = d.Foreground
		}
	}
	for _, kind := range styleKinds(d.Tokens) {
		c, ok := resolve("token-" + kind.String()).ForegroundColor()
		if ok {
			d.Tokens[kind] = nrgba(c)
		}
	}
	if v.themeScheme != nil && sameCodeColors(d, v.themeScheme.Definition()) {
		return v.themeScheme, nil
	}
	s, err := scheme.New(d)
	if err == nil {
		v.themeScheme = s
	}
	return s, err
}

func (v *SourceView) styledFont() typography.FontInfo {
	name := v.StyleName()
	if name == "" {
		name = "source-view"
	}
	state := style.Normal
	if !gui.IsEnabled(v) {
		state = style.Disabled
	}
	resolved := gui.ResolveWidgetStyle(v, name, "", state)
	font := typography.FontInfo{Family: "monospace", Size: 12}
	if family, ok := resolved.FontFamily(); ok {
		font.Family = family
	}
	if size, ok := resolved.FontSize(); ok {
		font.Size = size
	}
	return font
}

func (v *SourceView) StyleChanged() {
	v.updateScheme()
	if v.Destroyed() {
		return
	}
	// Hover and other unchanged style notifications must preserve a popup.
	// Its own UI appearance can change even with an explicit code scheme.
	if s := v.completion; s != nil && s.list.readAppearance() {
		v.closeCompletion()
		if v.Destroyed() {
			return
		}
	}
	if !gui.IsEnabled(v) {
		v.closeCompletion()
		if v.Destroyed() {
			return
		}
		v.cancelPreedit()
		if v.Destroyed() {
			return
		}
		v.cancelPaste()
		v.hideMenu()
		if v.Destroyed() {
			return
		}
	}
	font := v.styledFont()
	if v.font == font {
		return
	}
	v.closeCompletion()
	if v.Destroyed() {
		return
	}
	// Teardown callbacks may replace the stylesheet or move the view.
	font = v.styledFont()
	if v.font == font {
		return
	}
	v.font = font
	v.metricsReady = false
	v.contentWidth = 0
	if v.anchor.valid {
		v.pendingAnchor = true
	}
	v.dropLayouts()
	v.resetHeights()
}

func init() {
	basic.Register(sourceStyleID, basicRules)
	modern.Register(sourceStyleID, modernRules)
}

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

func basicRules(base style.StyleSheet) []style.Rule {
	rules := codeRules(scheme.Light().Definition())
	rules = append(rules, editorScrollRules(base)...)
	c := completionColors{
		Background: rgb(0xf8fafc), Foreground: rgb(0x24292f), DetailForeground: rgb(0x626d7a), Border: rgb(0xb9c3cf),
		SelectedBackground: rgb(0xe3edf9), SelectedForeground: rgb(0x172b44), SelectedDetailForeground: rgb(0x4e627b),
		MatchForeground: rgb(0x005fb8), SelectedMatchForeground: rgb(0x0055a5),
	}
	f := findColors{Background: rgb(0xf3f3f3), Foreground: rgb(0x333333), MutedForeground: rgb(0x6c6c6c), Border: rgb(0xd0d0d0), InputBackground: rgb(0xffffff),
		InputBorder: rgb(0xcecece), FocusBorder: rgb(0x0078d4), HoverBackground: rgb(0xe1e1e1), OptionBackground: rgb(0xcce5ff), OptionBorder: rgb(0x0078d4), ErrorForeground: rgb(0xb42318), Shadow: color.NRGBA{A: 42}}
	rules = append(rules, findRules(base, f, 9.75, 4, 2, 1)...)
	shadow, _ := base.Resolve(style.Sel{Name: "menu"}).Shadow()
	rules = append(rules, completionRules(c, base, 12, 4, 2, shadow)...)
	return rules
}

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
	rules = append(rules, editorScrollRules(base)...)
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
	selectedRadius, _ := base.Resolve(style.Sel{Name: "menu-item"}).Radius()
	rules = append(rules, completionRules(c, base, o.FontSize, radius, selectedRadius, shadow)...)
	return rules
}

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
