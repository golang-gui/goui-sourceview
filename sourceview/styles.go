package sourceview

import (
	"github.com/golang-gui/goui-sourceview/internal/controls"
	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

const sourceStyleID = "github.com/golang-gui/goui-sourceview"

func (v *SourceView) styledScheme() (*scheme.Scheme, error) {
	name := v.StyleName()
	if name == "" {
		name = "source-view"
	}
	resolve := func(part string) style.Style { return gui.ResolveWidgetStyle(v, name, part, style.Normal) }
	s, err := view.SchemeFromStyles(resolve,
		func() []language.Kind { return v.document.definition.Styles() },
		func() *scheme.Scheme { return v.themeScheme })
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
	defer func() { v.report(v.prepareInlineSuggestion()) }()
	v.updateScheme()
	if v.Destroyed() {
		return
	}
	// Hover and other unchanged style notifications must preserve a popup.
	// Its own UI appearance can change even with an explicit code scheme.
	if s := v.completion; s != nil && completionAppearanceChanged(s.list) {
		v.closeCompletion()
		if v.Destroyed() {
			return
		}
	}
	if !gui.IsEnabled(v) {
		v.ClearInlineSuggestion()
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

func basicRules(base style.StyleSheet) []style.Rule {
	rules := view.CodeRules(scheme.Light().Definition())
	rules = append(rules, view.EditorScrollRules(base)...)
	rules = append(rules, controls.BasicRules(base)...)
	return rules
}

func modernRules(o modern.Options, base style.StyleSheet) []style.Rule {
	s := scheme.Light()
	if o.Dark {
		s = scheme.Dark()
	}
	d := s.Definition()
	root := base.Resolve(style.Sel{Name: "text-view"})
	d.Background, d.Foreground = view.Background(root), view.Foreground(root)
	// Source selection preserves syntax foregrounds and blends a real tint under
	// the text. TextView's resolved selection inherits its ordinary foreground
	// and uses a preblended, opaque background, which is a different contract.
	focus := base.Resolve(style.Sel{Name: "text-input", State: style.Focused})
	focusBorder, _ := focus.BorderColor()
	d.SelectionBackground = view.NRGBA(focusBorder)
	d.SelectionBackground.A = 64
	d.InactiveSelectionBackground = d.SelectionBackground
	d.InactiveSelectionBackground.A = 32
	rules := view.CodeRules(d)
	rules = append(rules, view.EditorScrollRules(base)...)
	rules = append(rules, controls.ModernRules(o, base)...)
	return rules
}

func completionAppearanceChanged(list *controls.CompletionList) bool {
	before := list.Appearance()
	list.ReadAppearance()
	return before != list.Appearance()
}
