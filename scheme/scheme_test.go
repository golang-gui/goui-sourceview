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
