// 共享的确定性排版替身与着色样本；不能替代真实原生排版验证。
package sourceview

import (
	"image/color"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
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

func (l *shapedLayout) Text() string { return l.text }

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

func (l *shapedLayout) SetUnderline(int, int, bool) {}

func (l *shapedLayout) MeasureSize() (float32, float32) { return l.width, 20 }

func (l *shapedLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	if l.dead {
		panic("measured dead layout")
	}
	return []typography.TextLine{{Length: len(l.text), Width: l.width, Height: 20}}, l.clusters
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
