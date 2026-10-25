package sourceview

import (
	"image/color"
	"reflect"
	"sync"
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

func themedScope(child gui.Widget, sheet style.StyleSheet) *gui.StyleScope {
	s := gui.NewStyleScope()
	s.SetStyleSheet(sheet)
	s.SetChild(child)
	return s
}

func TestRegisteredStylesAndTokenParts(t *testing.T) {
	for _, sheet := range []style.StyleSheet{gui.DefaultStyleSheet(), basic.Sheet(), modern.Sheet(modern.Options{}), modern.Sheet(modern.Options{Dark: true})} {
		for _, name := range []string{"source-view", "sourceview.find", "sourceview.find.input", "sourceview.completion"} {
			if c, ok := sheet.Resolve(style.Sel{Name: name}).BackgroundColor(); !ok || c == nil {
				t.Fatalf("missing registered rules for %s", name)
			}
		}
	}
	for _, d := range []scheme.Definition{scheme.Light().Definition(), scheme.Dark().Definition()} {
		s := style.Sheet(codeRules(d)...)
		for kind, expected := range d.Tokens {
			if got := foreground(s.Resolve(style.Sel{Name: "source-view", Part: "token-" + tokenParts[kind]})); got != expected {
				t.Fatalf("token %v: %v, want %v", kind, got, expected)
			}
		}
		if tokenParts[language.KindComment] != "comment" || tokenParts[language.KindMeta] != "meta" {
			t.Fatal("token part indices do not match the public kinds")
		}
	}
	o := modern.Options{Dark: true, FontFamily: "test-family", FontSize: 17, AccentColor: rgb(0x427f50)}
	if !reflect.DeepEqual(modern.Rules(o), modern.Rules(o)) {
		t.Fatal("registered rules are not deterministic")
	}
}

func TestStyleChangedUpdatesCodeColorsWithoutReshaping(t *testing.T) {
	v := NewSourceView()
	scope := themedScope(v, modern.Sheet(modern.Options{}))
	v.StyleChanged()
	before, epoch, font := v.appliedScheme, v.layoutEpoch, v.font
	if v.colors.Background != rgb(0xffffff) {
		t.Fatal("light code surface", v.colors.Background)
	}
	scope.SetStyleSheet(modern.Sheet(modern.Options{Dark: true}))
	v.StyleChanged()
	if v.appliedScheme == before || v.colors.Background != rgb(0x28282d) || v.font != font || v.layoutEpoch != epoch {
		t.Fatal("color-only update retained colors or invalidated geometry")
	}
	before = v.appliedScheme
	v.StyleChanged()
	if before != v.appliedScheme {
		t.Fatal("identical styles created a new code scheme")
	}
	v.SetScheme(scheme.Light())
	scope.SetStyleSheet(modern.Sheet(modern.Options{Dark: true, FontSize: 18}))
	v.StyleChanged()
	if v.appliedScheme != scheme.Light() || v.font != font {
		t.Fatal("explicit code scheme lost, or UI font affected code font")
	}
	v.SetScheme(nil)
	if v.colors.Background != rgb(0x28282d) {
		t.Fatal("nil scheme did not return to the local stylesheet")
	}
}

func TestConcurrentThemeConstructionKeepsSnapshots(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o := modern.Options{Dark: i%2 == 0, FontSize: float32(10 + i)}
			s := modern.Sheet(o)
			if got, _ := s.Resolve(style.Sel{Name: "sourceview.completion"}).FontSize(); got != o.FontSize {
				t.Errorf("concurrent theme font: %g, want %g", got, o.FontSize)
			}
			want := rgb(0xffffff)
			if o.Dark {
				want = rgb(0x28282d)
			}
			if got := background(s.Resolve(style.Sel{Name: "sourceview.find"})); got != want {
				t.Errorf("concurrent theme palette: %v, want %v", got, want)
			}
		}()
	}
	wg.Wait()
}

func TestThemeChangeReadsCurrentStateAfterPopupTeardown(t *testing.T) {
	for _, change := range []string{"scheme", "scope", "document"} {
		t.Run(change, func(t *testing.T) {
			v := NewSourceView()
			owner := themedScope(v, modern.Sheet(modern.Options{}))
			v.StyleChanged()
			s := &completionSession{list: &completionList{}}
			s.popup = closingPopup{onClose: func() {
				switch change {
				case "scheme":
					v.SetScheme(scheme.Light())
				case "scope":
					owner.SetStyleSheet(modern.Sheet(modern.Options{}))
				case "document":
					v.SetDocument(NewDocument("replacement"))
				}
			}}
			v.completion = s
			if change == "scope" {
				owner.SetStyleSheet(modern.Sheet(modern.Options{Dark: true}))
				v.updateScheme()
			} else {
				v.SetScheme(scheme.Dark())
			}
			want := rgb(0xffffff)
			if change == "document" {
				want = scheme.Dark().Definition().Background
			}
			if v.colors.Background != want {
				t.Fatal("popup callback left stale code colors", v.colors.Background, want)
			}
		})
	}
}

func TestCodeFontAndPlainForegroundOverrides(t *testing.T) {
	rules := modern.Rules(modern.Options{Dark: true})
	rules = append(rules,
		style.Name("source-view").ForegroundColor(rgb(0xe1f1e1)).FontFamily("code-family").FontSize(16),
		style.Name("source-view").Part("token-string").ForegroundColor(rgb(0x99dd99)),
	)
	v := NewSourceView()
	themedScope(v, style.Sheet(rules...))
	v.StyleChanged()
	if v.font.Family != "code-family" || v.font.Size != 16 || v.appliedScheme.Color(language.KindPlain) != rgb(0xe1f1e1) || v.appliedScheme.Color(language.KindString) != rgb(0x99dd99) {
		t.Fatal("code font or foreground overrides were lost")
	}
}

func TestCustomStyleKeyFollowsDefinitionAndDocument(t *testing.T) {
	v := NewSourceView()
	color := scheme.Dark().Definition().Foreground
	scope := themedScope(v, style.Sheet(style.Name("source-view").Part("token-custom.heading").ForegroundColor(color)))
	_ = scope
	definition := language.Definition{Info: language.Info{ID: "custom", Name: "Custom"}}
	definition.Styles = []language.Kind{"custom.heading"}
	l, err := language.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	d := NewDocument("heading")
	d.SetLanguageDefinition(l)
	v.SetDocument(d)
	if got := v.colors.Tokens["custom.heading"]; got != color {
		t.Fatalf("custom color %v, want %v", got, color)
	}
	d.SetLanguageDefinition(nil)
	if _, ok := v.colors.Tokens["custom.heading"]; ok {
		t.Fatal("retained previous definition style")
	}
}

func TestEditorFrameAndScrollBarThemeParts(t *testing.T) {
	for _, sheet := range []style.StyleSheet{basic.Sheet(), modern.Sheet(modern.Options{}), modern.Sheet(modern.Options{Dark: true})} {
		for _, state := range []style.State{style.Normal, style.Focused, style.FocusVisible, style.Disabled} {
			frame := sheet.Resolve(style.Sel{Name: "source-view", Part: "frame", State: state})
			want := sheet.Resolve(style.Sel{Name: "text-view", Part: "frame", State: state})
			if !reflect.DeepEqual(frame, want) {
				t.Fatalf("editor frame differs from official theme: state=%v", state)
			}
			if width, _ := frame.BorderWidth(); width <= 0 {
				t.Fatalf("missing editor frame in state %v", state)
			}
		}
		for _, part := range []string{"trough", "thumb"} {
			for _, state := range []style.State{style.Normal, style.Hovered, style.Pressed} {
				got := sheet.Resolve(style.Sel{Name: "source-view", Part: part, State: state})
				want := sheet.Resolve(style.Sel{Name: "scroll-view", Part: part, State: state})
				gotRadius, _ := got.Radius()
				wantRadius, _ := want.Radius()
				if background(got) != background(want) || gotRadius != wantRadius {
					t.Fatalf("scrollbar theme mismatch: part=%s state=%v", part, state)
				}
				if width, _ := got.BorderWidth(); width != 0 {
					t.Fatal("editor frame leaked into scrollbar")
				}
			}
		}
		body := sheet.Resolve(style.Sel{Name: "source-view"})
		if width, _ := body.BorderWidth(); width != 0 {
			t.Fatal("code body must not draw a second frame")
		}
	}
}

// 官方主题及局部覆盖分别提供普通/选中标记颜色，不触发重新排版。
func TestWhitespaceStyleParts(t *testing.T) {
	for _, tc := range []struct {
		sheet style.StyleSheet
		want  color.NRGBA
	}{
		{basic.Sheet(), color.NRGBA{0x33, 0x33, 0x33, 0x33}},
		{modern.Sheet(modern.Options{}), color.NRGBA{0x33, 0x33, 0x33, 0x33}},
		{modern.Sheet(modern.Options{Dark: true}), color.NRGBA{0xe3, 0xe4, 0xe2, 0x29}},
	} {
		normal := foreground(tc.sheet.Resolve(style.Sel{Name: "source-view", Part: "whitespace"}))
		selected := foreground(tc.sheet.Resolve(style.Sel{Name: "source-view", Part: "selected-whitespace"}))
		if normal != tc.want || selected != tc.want {
			t.Fatal("whitespace theme parts", normal, selected, tc.want)
		}
	}
	v := NewSourceView()
	v.Document().Model().SetText(" ")
	observe(t, v)
	v.context = &shapedContext{}
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 200, Height: 100}, geometry.Point{})
	p := v.paragraph(0)
	normal, selected := rgb(0x116688), rgb(0xec22aa)
	themedScope(v, style.Sheet(style.Name("source-view").Part("whitespace").ForegroundColor(normal), style.Name("source-view").Part("selected-whitespace").ForegroundColor(selected)))
	_ = v.SetWhitespaceMode(WhitespaceAll)
	out := &recordingPainter{}
	v.paintWhitespace(out, 0, p, geometry.Point{X: v.gutter + padding, Y: padding})
	if len(out.fills) != 1 || out.fills[0].color != graphics.ColorOf(normal) {
		t.Fatal("normal marker style", out.fills)
	}
	v.SetSelection(textedit.Selection{Caret: 1})
	out = &recordingPainter{}
	v.paintWhitespace(out, 0, p, geometry.Point{X: v.gutter + padding, Y: padding})
	if len(out.fills) != 1 || out.fills[0].color != graphics.ColorOf(selected) || p != v.paragraph(0) {
		t.Fatal("selected marker style", out.fills)
	}
}

// 显式代码 Scheme 的装饰默认色依据代码背景，不能落回 GUI 主题或强正文色。
func TestWhitespaceExplicitSchemeDefaults(t *testing.T) {
	for _, tc := range []struct {
		scheme *scheme.Scheme
		want   color.NRGBA
	}{
		{scheme.Light(), color.NRGBA{0x33, 0x33, 0x33, 0x33}},
		{scheme.Dark(), color.NRGBA{0xe3, 0xe4, 0xe2, 0x29}},
	} {
		v := NewSourceView()
		v.Document().Model().SetText(" ")
		observe(t, v)
		v.SetScheme(tc.scheme)
		ctx := &shapedContext{}
		v.context = ctx
		t.Cleanup(v.dropLayouts)
		v.LayoutVisible(geometry.Size{Width: 200, Height: 100}, geometry.Point{})
		p := v.paragraph(0)
		created := ctx.created
		_ = v.SetWhitespaceMode(WhitespaceAll)
		for _, selection := range []textedit.Selection{{}, {Caret: 1}} {
			v.SetSelection(selection)
			out := &recordingPainter{}
			v.paintWhitespace(out, 0, p, geometry.Point{X: v.gutter + padding, Y: padding})
			if len(out.fills) != 1 || out.fills[0].color != graphics.ColorOf(tc.want) || ctx.created != created || v.paragraph(0) != p {
				t.Fatal("explicit scheme marker color/layout", out.fills, tc.want)
			}
		}
	}
}
