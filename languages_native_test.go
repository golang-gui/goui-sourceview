package sourceview

import (
	"bytes"
	"runtime"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

// Compare glyph pixels against a separately constructed, single-color layout.
// Each fixture's visible glyphs have an independently specified lexical color.
// Same backend/font/scale on both sides; borrowed bitmap pixels are copied.
func TestNativeConfigurationAndShellColors(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	for _, tc := range []struct {
		id         language.ID
		text, line string
		row        int
		kind       language.Kind
	}{
		{language.TOML, "text = '''\nbody\n'''", "body", 1, language.KindString},
		{language.YAML, "text: |\n  body\nnext: true", "  body", 1, language.KindString},
		{language.Shell, "$HOME", "$HOME", 0, language.KindVariable},
		{language.Shell, "cat <<END\n${HOME}\nEND", "${HOME}", 1, language.KindVariable},
	} {
		for _, colors := range []*scheme.Scheme{scheme.Light(), scheme.Dark()} {
			func() {
				v := NewSourceView()
				v.SetScheme(colors)
				v.Document().Model().SetText(tc.text)
				if err := v.Document().SetLanguage(tc.id); err != nil {
					t.Fatal(err)
				}
				v.mounted = true
				v.Document().acquire(v)
				defer func() { v.dropLayouts(); v.Document().release(v); v.mounted = false }()
				v.context = ctx
				v.Document().engine.Advance(time.Time{})
				v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
				p := v.paragraph(tc.row)
				fresh, err := ctx.NewTextLayout(tc.line, typography.TextFormat{Font: v.font, TextColor: colors.Color(tc.kind), TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}, 1<<20, 1<<24)
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Destroy()
				got, err := p.layout.Rasterize(1, nil)
				if err != nil {
					t.Fatal(err)
				}
				pixels := bytes.Clone(got.Pixels)
				want, err := fresh.Rasterize(1, nil)
				if err != nil {
					t.Fatal(err)
				}
				if got.Width != want.Width || got.Height != want.Height || got.Stride != want.Stride || !bytes.Equal(pixels, want.Pixels) {
					t.Fatalf("%s %s %s: glyph color differs from %s", ctx.Name(), tc.id, colors.Definition().Name, tc.kind)
				}
			}()
		}
	}
}
