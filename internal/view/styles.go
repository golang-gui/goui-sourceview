package view

import (
	"image/color"
	"maps"
	"slices"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/style"
)

// The ScrollView host owns the outer frame. Copy the official theme's parts
// explicitly; the code body and its syntax colors remain independent.
func EditorScrollRules(base style.StyleSheet) []style.Rule {
	var rules []style.Rule
	for _, part := range []string{"frame", "trough", "thumb"} {
		name := "scroll-view"
		states := []style.State{style.Normal, style.Focused, style.FocusVisible, style.Disabled}
		if part == "frame" {
			name = "text-view"
			// Resolved hover/press defaults would overwrite the focused frame.
			// Official editor frames have no separate pointer-state appearance.
		} else {
			states = append(states, style.Hovered, style.Pressed)
		}
		for _, state := range states {
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

var tokenKinds = []language.Kind{language.KindHeading, language.KindCode, language.KindEmphasis, language.KindLink, language.KindFence, language.KindMarker, language.KindPlain, language.KindProperty, language.KindString, language.KindEscape, language.KindNumber, language.KindKeyword, language.KindDelimiter, language.KindTag, language.KindAttribute, language.KindEntity, language.KindComment, language.KindMeta, language.KindInvalid, language.KindVariable, language.KindAdded, language.KindRemoved}

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

func NRGBA(c color.Color) color.NRGBA {
	if c == nil {
		return color.NRGBA{}
	}
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

func Foreground(s style.Style) color.NRGBA { c, _ := s.ForegroundColor(); return NRGBA(c) }

func Background(s style.Style) color.NRGBA { c, _ := s.BackgroundColor(); return NRGBA(c) }

// Defaults follow the actual code Background, including an explicit Scheme.
// Both states stay quiet; applications can override their style parts separately.
func WhitespaceColor(bg color.NRGBA) color.NRGBA {
	if uint32(bg.R)*299+uint32(bg.G)*587+uint32(bg.B)*114 < 128000 {
		return Tint(RGB(0xe3e4e2), 0x29)
	}
	return Tint(RGB(0x333333), 0x33)
}

func CodeRules(d scheme.Definition) []style.Rule {
	whitespace := WhitespaceColor(d.Background)
	rules := []style.Rule{
		style.Name("source-view").Part("indent-guide").ForegroundColor(Tint(d.GutterForeground, 65)),
		style.Name("source-view").Part("whitespace").ForegroundColor(whitespace),
		style.Name("source-view").Part("selected-whitespace").ForegroundColor(whitespace),
		style.Name("source-view").Part("ruler").ForegroundColor(Tint(d.GutterForeground, 45)),
		style.Name("source-view").Part("occurrence").ForegroundColor(Tint(d.SearchBackground, 55)),
		style.Name("source-view").FontFamily("monospace").FontSize(12).BackgroundColor(d.Background).ForegroundColor(d.Foreground),
		style.Name("source-view").Part("gutter").BackgroundColor(d.GutterBackground).ForegroundColor(d.GutterForeground),
		style.Name("source-view").Part("fold-toggle").ForegroundColor(d.GutterForeground),
		style.Name("source-view").Part("line-mark").ForegroundColor(RGB(0xd83b36)),
		style.Name("source-view").Part("line-mark-muted").ForegroundColor(d.GutterForeground),
		style.Name("source-view").Part("bookmark").ForegroundColor(RGB(0x3579c8)),
		style.Name("source-view").Part("execution-arrow").ForegroundColor(RGB(0xc39400)),
		style.Name("source-view").Part("minimap-background").ForegroundColor(d.Background),
		style.Name("source-view").Part("minimap-viewport").ForegroundColor(Tint(d.GutterForeground, 35)),
		style.Name("source-view").Part("minimap-border").ForegroundColor(Tint(d.GutterForeground, 90)),
		style.Name("source-view").Part("execution-line").ForegroundColor(Tint(RGB(0xe6b422), 35)),
		style.Name("source-view").Part("fold-placeholder").ForegroundColor(d.GutterForeground),
		style.Name("source-view").Part("inline-suggestion").ForegroundColor(d.GutterForeground),
		style.Name("source-view").Part("current-line").BackgroundColor(d.CurrentLine),
		// An explicit zero Foreground prevents the root Foreground fallback;
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

func RGB(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// SchemeFromStyles borrows readers so reentrant stylesheet callbacks retain
// their original lookup order. It owns neither language nor scheme state.
func SchemeFromStyles(resolve func(string) style.Style, kinds func() []language.Kind, previous func() *scheme.Scheme) (*scheme.Scheme, error) {
	d := scheme.Light().Definition()
	d.Name = "SourceView stylesheet"
	setBG := func(part string, out *color.NRGBA) {
		if c, ok := resolve(part).BackgroundColor(); ok {
			*out = NRGBA(c)
		}
	}
	setFG := func(part string, out *color.NRGBA) {
		if c, ok := resolve(part).ForegroundColor(); ok {
			*out = NRGBA(c)
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
		d.BracketMatchBorder = NRGBA(c)
	}
	setFG("caret", &d.Caret)
	setFG("preedit", &d.PreeditUnderline)
	if c, ok := resolve("border").BorderColor(); ok {
		d.Border = NRGBA(c)
	}
	for _, kind := range kinds() {
		if _, ok := d.Tokens[kind]; !ok {
			d.Tokens[kind] = d.Foreground
		}
	}
	for _, kind := range styleKinds(d.Tokens) {
		c, ok := resolve("token-" + kind.String()).ForegroundColor()
		if ok {
			d.Tokens[kind] = NRGBA(c)
		}
	}
	if cached := previous(); cached != nil && sameCodeColors(d, cached.Definition()) {
		return cached, nil
	}
	return scheme.New(d)
}

func Tint(c color.NRGBA, alpha uint8) color.NRGBA { c.A = alpha; return c }
