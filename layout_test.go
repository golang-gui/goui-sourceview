package sourceview

import (
	"fmt"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"image/color"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

type shapedContext struct {
	typography.Context
	created, destroyed int
}

func (c *shapedContext) NewTextLayout(text string, format typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	c.created++
	clusters := []typography.TextCluster{}
	x := float32(0)
	for at := 0; at < len(text); {
		_, n := utf8.DecodeRuneInString(text[at:])
		if text[at:] != "" && len(text)-at >= 3 && text[at:at+3] == "e\u0301" {
			n = 3
		}
		clusters = append(clusters, typography.TextCluster{Start: at, Length: n, X: x, Width: 10, Height: 20})
		x += 10
		at += n
	}
	colors := make([]color.NRGBA, len(text))
	base := format.TextColor
	if base == nil {
		base = color.Black
	}
	for i := range colors {
		colors[i] = color.NRGBAModel.Convert(base).(color.NRGBA)
	}
	return &shapedLayout{context: c, text: text, format: format, width: x, clusters: clusters, colors: colors}, nil
}

type shapedLayout struct {
	typography.TextLayout
	context    *shapedContext
	text       string
	format     typography.TextFormat
	width      float32
	clusters   []typography.TextCluster
	dead       bool
	colors     []color.NRGBA
	colorCalls int
	onColor    func()
}

func (l *shapedLayout) Destroy() {
	if l.dead {
		panic("layout destroyed twice")
	}
	l.dead = true
	l.context.destroyed++
}
func (l *shapedLayout) Text() string                  { return l.text }
func (l *shapedLayout) Format() typography.TextFormat { return l.format }
func (l *shapedLayout) SetTextColor(start, length int, c color.Color) {
	if l.dead {
		panic("colored dead layout")
	}
	l.colorCalls++
	for i := start; i < start+length; i++ {
		l.colors[i] = color.NRGBAModel.Convert(c).(color.NRGBA)
	}
	if l.onColor != nil {
		l.onColor()
	}
}
func (l *shapedLayout) SetUnderline(int, int, bool)     {}
func (l *shapedLayout) MeasureSize() (float32, float32) { return l.width, 20 }
func (l *shapedLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	if l.dead {
		panic("measured dead layout")
	}
	return []typography.TextLine{{Length: len(l.text), Width: l.width, Height: 20}}, l.clusters
}
func TestSearchKeepsRawMatchSeparateFromClusterSelection(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("e\u0301 z")
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	v.SetSearch(SearchQuery{Text: "\u0301"})
	raw, ok := v.FindNext(false)
	if !ok || raw != (textedit.Range{Start: 1, End: 3}) || v.Selection() != (textedit.Selection{Caret: 3}) {
		t.Fatalf("raw %v selection %v", raw, v.Selection())
	}
	if ok, err := v.ReplaceCurrent("X"); !ok || err != nil || v.Document().Model().Text() != "eX z" {
		t.Fatal("visual selection replaced the base character", ok, err, v.Document().Model().Text())
	}
	v.dropLayouts()
	// Every native object, including measurement probes, has one view owner.
	if ctx.created != ctx.destroyed {
		t.Fatalf("native layout leak %d/%d", ctx.created, ctx.destroyed)
	}
}
func TestFindReentryDoesNotRestoreAnObsoleteRawMatch(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("abc abc")
	observe(t, v)
	v.SetSearch(SearchQuery{Text: "abc"})
	once := true
	v.ConnectSelection(func(textedit.Selection) {
		if once {
			once = false
			v.SetSelection(textedit.Selection{Anchor: 4, Caret: 4})
		}
	})
	_, _ = v.FindNext(false)
	if ok, err := v.ReplaceCurrent("X"); ok || err != nil || v.Document().Model().Text() != "abc abc" {
		t.Fatal("reentrant selection ignored", ok, err)
	}
}

func colorFixture(t *testing.T) (*SourceView, *shapedContext) {
	t.Helper()
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	v.Document().Model().SetText(`{"name":"value"}`)
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	v.Document().engine.Advance(time.Time{})
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	return v, ctx
}

func TestColorChangesReuseParagraph(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	original := p.layout.(*shapedLayout)
	created := ctx.created
	height, width := v.heights.Total(), v.contentWidth
	for i := 0; i < 20; i++ {
		v.SetSelection(textedit.Selection{Anchor: i % 4, Caret: 8})
		v.SetScheme([]*scheme.Scheme{scheme.Dark(), scheme.Light()}[i%2])
		if got := v.paragraph(0); got != p || got.layout != original || ctx.created != created {
			t.Fatal("color-only change recreated native paragraph", ctx.created, created)
		}
		if v.heights.Total() != height || v.contentWidth != width {
			t.Fatal("color-only change invalidated size estimates")
		}
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("native layout leak", ctx.created, ctx.destroyed)
	}
}

func TestLanguageChangePreservesTextLayoutAndClearsTokens(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	if native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("fixture lacks property color", native.colors[2])
	}
	created := ctx.created
	id := v.Document().engine.Line(0).ID
	if err := v.Document().SetLanguage(language.Plain); err != nil {
		t.Fatal(err)
	}
	v.Document().engine.Advance(time.Time{})
	if v.Document().engine.Line(0).ID != id {
		t.Fatal("language changed text-line identity")
	}
	if got := v.paragraph(0); got != p || ctx.created != created {
		t.Fatal("language change recreated paragraph")
	}
	for _, c := range native.colors {
		if c != v.colors.Foreground {
			t.Fatal("obsolete token color remained", c)
		}
	}
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	v.Document().engine.Advance(time.Time{})
	if got := v.paragraph(0); got != p || ctx.created != created || native.colors[2] != v.colors.Tokens[language.KindProperty] {
		t.Fatal("language recolor failed")
	}
}

func TestUnchangedColorStateDoesNoNativeWork(t *testing.T) {
	v, _ := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	calls := native.colorCalls
	for i := 0; i < 10; i++ {
		if v.paragraph(0) != p {
			t.Fatal("cache miss")
		}
	}
	if native.colorCalls != calls {
		t.Fatal("unchanged paragraph recolored")
	}
}

func BenchmarkParagraphSelection(b *testing.B) {
	for _, size := range []int{80, 4096} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			v := NewSourceView()
			v.Document().Model().SetText(fmt.Sprintf("%*s", size, "x"))
			ctx := &shapedContext{}
			v.context = ctx
			defer v.dropLayouts()
			v.LayoutVisible(geometry.Size{Width: 800, Height: 400}, geometry.Point{})
			created := ctx.created
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				v.SetSelection(textedit.Selection{Caret: i % size})
				v.paragraph(0)
			}
			b.StopTimer()
			b.ReportMetric(float64(ctx.created-created)/float64(b.N), "layouts/op")
		})
	}
}

func TestColdIndexAdoptsPlainLayout(t *testing.T) {
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	v.Document().Model().SetText("true")
	if err := v.Document().SetLanguage(language.JSON); err != nil {
		t.Fatal(err)
	}
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	p := v.paragraph(0)
	created := ctx.created
	v.Document().engine.Advance(time.Time{})
	if got := v.paragraph(0); got != p || ctx.created != created {
		t.Fatal("initial index completion reshaped unchanged text")
	}
	native := p.layout.(*shapedLayout)
	for _, c := range native.colors {
		if c != v.colors.Tokens[language.KindKeyword] {
			t.Fatal("first lexical colors were not applied")
		}
	}
}

func TestRecolorReentryStopsUsingReleasedLayout(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	native := p.layout.(*shapedLayout)
	native.onColor = func() { native.onColor = nil; v.SetDocument(NewDocument("replacement")) }
	v.SetScheme(scheme.Dark())
	if got := v.paragraph(0); got != nil {
		t.Fatal("reentrant document switch returned stale paragraph")
	}
	if !native.dead || v.Document().Model().Text() != "replacement" {
		t.Fatal("reentry was not completed")
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("layout ownership leak", ctx.created, ctx.destroyed)
	}
}

func TestGutterRecolorKeepsNativeNumber(t *testing.T) {
	v, ctx := colorFixture(t)
	p := v.paragraph(0)
	number, err := ctx.NewTextLayout("1", typography.TextFormat{TextColor: v.colors.GutterForeground}, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	p.number, p.numberText = number, "1"
	v.SetScheme(scheme.Dark())
	if v.paragraph(0) != p || p.number != number {
		t.Fatal("gutter color rebuilt native number")
	}
	if got := number.(*shapedLayout).colors[0]; got != v.colors.GutterForeground {
		t.Fatal("gutter retained old scheme", got)
	}
}
