package sourceview

import (
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
)

func TestSameIDWithoutTokenizerClearsNativeColors(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	if native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("fixture has no syntax color")
	}
	created := ctx.created
	id := v.document.engine.Line(0).ID
	uncolored, err := language.New(language.Definition{Info: language.Info{ID: language.JSON, Name: "Uncolored JSON"}})
	if err != nil {
		t.Fatal(err)
	}
	v.document.SetLanguageDefinition(uncolored)
	if got := v.paragraph(0); got != p || ctx.created != created || v.document.engine.Line(0).ID != id {
		t.Fatal("definition recreated text layout or line identity")
	}
	for _, c := range native.colors {
		if c != v.colors.Foreground {
			t.Fatal("same-ID definition retained obsolete syntax colors", c)
		}
	}
	if err := v.document.SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.document.engine.Advance(time.Time{})
	if got := v.paragraph(0); got != p || ctx.created != created || native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("same-ID tokenizer restore failed")
	}
}
