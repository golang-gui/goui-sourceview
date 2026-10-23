package sourceview

import (
	"bytes"
	"image/color"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

func TestNativeRecolorMatchesFreshLayout(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	v.Document().Model().SetText("true")
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.mounted = true
	v.Document().acquire(v)
	defer func() { v.dropLayouts(); v.Document().release(v); v.mounted = false }()
	v.context = ctx
	v.Document().engine.Advance(time.Time{})
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	p := v.paragraph(0)
	for _, plain := range []bool{false, true} {
		v.SetScheme(scheme.Dark())
		if plain {
			if err := v.Document().SetLanguage(language.Plain); err != nil {
				t.Fatal(err)
			}
		}
		if got := v.paragraph(0); got != p {
			t.Fatal("native recolor replaced the paragraph")
		}
		foreground := v.colors.Tokens[language.KindKeyword]
		if plain {
			foreground = v.colors.Foreground
		}
		fresh, err := ctx.NewTextLayout("true", typography.TextFormat{Font: v.font, TextColor: foreground, TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}, 1<<20, 1<<24)
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.layout.Rasterize(1, rasterRegion(p.layout, 1), nil)
		if err != nil {
			fresh.Destroy()
			t.Fatal(err)
		}
		// Keep an independent pixel expectation across later rasterizations.
		pixels := bytes.Clone(got.Pixels)
		want, err := fresh.Rasterize(1, rasterRegion(fresh, 1), nil)
		if err != nil {
			fresh.Destroy()
			t.Fatal(err)
		}
		if got.Width != want.Width || got.Height != want.Height || got.Stride != want.Stride || !bytes.Equal(pixels, want.Pixels) {
			fresh.Destroy()
			t.Fatalf("%s plain=%v: cached recolor differs from independent base-color layout", ctx.Name(), plain)
		}
		fresh.Destroy()
	}
}

func TestNativeForegroundMetrics(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	text := "ffi e\u0301 中文 👩‍💻\tשלום xyz"
	for _, family := range []string{"monospace", "serif"} {
		for _, wrap := range []WrapMode{WrapNone, WrapWordChar} {
			l, err := ctx.NewTextLayout(text, typography.TextFormat{Font: typography.FontInfo{Family: family, Size: 12}, WrapMode: wrap, TabWidth: 64}, 160, 800)
			if err != nil {
				t.Fatal(err)
			}
			lines, clusters := copyMetrics(l)
			width, height := l.MeasureSize()
			for _, span := range []struct{ start, end int }{{0, len(text)}, {1, 2}, {4, 7}, {0, len(text)}} {
				l.SetTextColor(span.start, span.end-span.start, color.NRGBA{R: 125, G: 60, B: 210, A: 255})
				gotLines, gotClusters := copyMetrics(l)
				w, h := l.MeasureSize()
				if !reflect.DeepEqual(lines, gotLines) || !reflect.DeepEqual(clusters, gotClusters) || w != width || h != height {
					l.Destroy()
					t.Fatalf("%s %s wrap=%v color [%d,%d) changes metrics: size %g/%g -> %g/%g; lines %v -> %v; clusters %v -> %v", ctx.Name(), family, wrap, span.start, span.end, width, height, w, h, lines, gotLines, clusters, gotClusters)
				}
			}
			l.Destroy()
		}
	}
}

func TestNativeRecolorViewIsolation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	d := NewDocument("true")
	if err := d.SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	a, b := NewSourceView(), NewSourceView()
	for _, v := range []*SourceView{a, b} {
		v.SetDocument(d)
		v.SetScheme(scheme.Light())
		v.mounted = true
		d.acquire(v)
		v.context = ctx
		defer func(v *SourceView) { v.dropLayouts(); d.release(v); v.mounted = false }(v)
	}
	d.engine.Advance(time.Time{})
	for _, v := range []*SourceView{a, b} {
		v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	}
	peer := b.paragraph(0)
	before, err := peer.layout.Rasterize(1, rasterRegion(peer.layout, 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	pixels := bytes.Clone(before.Pixels)
	a.SetScheme(scheme.Dark())
	a.paragraph(0)
	if b.paragraph(0) != peer {
		t.Fatal("another view's recolor invalidated peer layout")
	}
	after, err := peer.layout.Rasterize(1, rasterRegion(peer.layout, 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if before.Width != after.Width || before.Height != after.Height || !bytes.Equal(pixels, after.Pixels) {
		t.Fatal("recolor contaminated another view's native state")
	}
}

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
				got, err := p.layout.Rasterize(1, rasterRegion(p.layout, 1), nil)
				if err != nil {
					t.Fatal(err)
				}
				pixels := bytes.Clone(got.Pixels)
				want, err := fresh.Rasterize(1, rasterRegion(fresh, 1), nil)
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
