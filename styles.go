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
