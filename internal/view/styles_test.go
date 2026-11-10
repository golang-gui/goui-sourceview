package view

import (
	"testing"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/style"
)

func TestSchemeReadersObserveStylesheetCallbacks(t *testing.T) {
	const annotation language.Kind = "annotation"
	marker := RGB(0x123456)
	d := scheme.Light().Definition()
	d.Tokens[annotation] = marker
	expected, err := scheme.New(d)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []language.Kind
	latest := scheme.Light()
	resolve := func(part string) style.Style {
		if part == "" {
			// A custom stylesheet may replace the language during root style
			// resolution; its new token kinds must be read afterward.
			kinds = []language.Kind{annotation}
		}
		if part == "token-annotation" {
			// The matching cache may also be replaced during token resolution.
			latest = expected
			return style.Name("source-view").ForegroundColor(marker).Style
		}
		return style.Style{}
	}
	got, err := SchemeFromStyles(resolve, func() []language.Kind { return kinds }, func() *scheme.Scheme { return latest })
	if err != nil || got != expected {
		t.Fatalf("callbacks must select the current language and cache: got %p, want %p, err %v", got, expected, err)
	}
	if got.Definition().Tokens[annotation] != marker {
		t.Fatal("language replaced during style resolution lost its token color")
	}
}
