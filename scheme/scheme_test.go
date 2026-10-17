package scheme

import (
	"image/color"
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
)

func TestImmutableAndFallback(t *testing.T) {
	d := Light().Definition()
	original := Light().Color(language.KindString)
	d.Tokens[language.KindString] = color.NRGBA{R: 12, A: 255}
	s, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	d.Tokens[language.KindString] = color.NRGBA{}
	if Light().Color(language.KindString) != original || s.Color(language.KindString).R != 12 {
		t.Fatal("shared mutable map")
	}
	out := s.Definition()
	out.Tokens[language.KindString] = color.NRGBA{}
	if s.Color(language.KindString).R != 12 {
		t.Fatal("getter exposed map")
	}
	if s.Color(language.KindPlain) != d.Foreground {
		t.Fatal("missing fallback")
	}
	d.Background.A = 0
	if _, err := New(d); err == nil {
		t.Fatal("accepted transparent base")
	}
}

func TestBracketColorsCanBeIndependentlyTransparent(t *testing.T) {
	for _, base := range []*Scheme{Light(), Dark()} {
		for _, alpha := range []uint8{0, 64, 255} {
			d := base.Definition()
			d.BracketMatchBackground.A = alpha
			s, err := New(d)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Definition(); got.BracketMatchBackground.A != alpha || got.BracketMatchBorder != base.Definition().BracketMatchBorder {
				t.Fatal("background changed border")
			}
			d = base.Definition()
			d.BracketMatchBorder.A = alpha
			s, err = New(d)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.Definition(); got.BracketMatchBorder.A != alpha || got.BracketMatchBackground != base.Definition().BracketMatchBackground {
				t.Fatal("border changed background")
			}
		}
	}
}

func TestCompletionPaletteIsIndependentAndOpaque(t *testing.T) {
	for _, base := range []*Scheme{Light(), Dark()} {
		d := base.Definition()
		want := d.Completion
		d.SelectionBackground = color.NRGBA{R: 255, A: 100}
		d.SelectionForeground = color.NRGBA{G: 255, A: 255}
		s, err := New(d)
		if err != nil || s.Definition().Completion != want {
			t.Fatal("editor selection changed completion palette", err)
		}
		d.Completion = CompletionColors{}
		s, err = New(d)
		if err != nil || s.Definition().Completion.Background != d.Background || s.Definition().Completion.Foreground != d.Foreground {
			t.Fatal("missing custom palette did not inherit base colors", err)
		}
		d.Completion = want
		d.Completion.DetailForeground.A = 0
		if _, err := New(d); err == nil {
			t.Fatal("transparent detail could become unreadable")
		}
		d.Completion = want
		d.Completion.Border.A, d.Completion.ScrollThumb.A = 0, 80
		if s, err = New(d); err != nil || s.Definition().Completion != d.Completion {
			t.Fatal("decorations lost explicit alpha", err)
		}
	}
}

func TestFindPaletteFallbackAndIsolation(t *testing.T) {
	for _, base := range []*Scheme{Light(), Dark()} {
		d := base.Definition()
		want := d.Find
		d.SelectionBackground = color.NRGBA{R: 240, A: 255}
		s, err := New(d)
		if err != nil || s.Definition().Find != want {
			t.Fatal("selection leaked into find palette", err)
		}
		d.Find = FindColors{}
		s, err = New(d)
		if err != nil || s.Definition().Find.InputBackground != d.Background || s.Definition().Find.Background != d.Completion.Background {
			t.Fatal("missing palette fallback", err)
		}
		d.Find = want
		d.Find.InputBackground.A = 0
		if _, err = New(d); err == nil {
			t.Fatal("accepted transparent input surface")
		}
		d.Find = want
		d.Find.Shadow.A = 0
		d.Find.OptionBorder.A = 80
		s, err = New(d)
		if err != nil || s.Definition().Find != d.Find {
			t.Fatal("lost decorative alpha", err)
		}
	}
}
