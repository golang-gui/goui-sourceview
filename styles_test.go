package sourceview

import (
	"reflect"
	"sync"
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/gui"
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
