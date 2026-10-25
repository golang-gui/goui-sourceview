// 共享文档订阅、输入法复位、确定性排版和绘制记录辅助设施，不自动创建窗口。
// 原生部分提供调用计数、度量副本及光栅范围；Context 工厂保留平台文件，调用方管理线程和资源。
// 排版与绘制替身不能替代真实原生排版及像素验证。
package sourceview

import (
	"image"
	"image/color"
	"math"
	"slices"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
)

// Bind only the pure document observer; native mount is covered by tests/window.
func observe(t *testing.T, v *SourceView) {
	t.Helper()
	v.mounted = true
	v.document.acquire(v)
	t.Cleanup(func() {
		if v.mounted {
			v.mounted = false
			v.document.release(v)
		}
	})
}

type resetContext struct {
	gui.IMContext
	reset func()
}

func (c *resetContext) Reset() { c.reset() }

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

type paintShape struct {
	rect  geometry.Rectangle
	width float32
	color graphics.Color
}

type recordingPainter struct {
	gui.Painter
	fills, borders []paintShape
	ellipses       []paintShape
	paths          []graphics.Path
	lines          int
	order          []string
}

func (p *recordingPainter) Save() {}

func (p *recordingPainter) Restore() {}

func (p *recordingPainter) PixelScale() float32 { return 1 }

func (p *recordingPainter) FillEllipse(c geometry.Point, rx, ry float32, b graphics.Brush) {
	r := geometry.Rect(c.X-rx, c.Y-ry, 2*rx, 2*ry)
	p.ellipses = append(p.ellipses, paintShape{rect: r, color: b.(graphics.Color)})
	p.FillRect(r, b)
}

func (p *recordingPainter) DrawEllipse(c geometry.Point, rx, ry, width float32, b graphics.Brush) {
	p.DrawRect(geometry.Rect(c.X-rx, c.Y-ry, 2*rx, 2*ry), width, b)
}

func (p *recordingPainter) FillPath(path graphics.Path, _ graphics.Brush) {
	p.paths = append(p.paths, path)
	p.order = append(p.order, "fill")
}

func (p *recordingPainter) FillRoundRect(r geometry.Rectangle, _ float32, b graphics.Brush) {
	p.FillRect(r, b)
}

func (p *recordingPainter) DrawRoundRect(r geometry.Rectangle, _ float32, w float32, b graphics.Brush) {
	p.DrawRect(r, w, b)
}

func (p *recordingPainter) SetClipRect(geometry.Rectangle) {}

func (p *recordingPainter) FillRect(r geometry.Rectangle, b graphics.Brush) {
	p.fills = append(p.fills, paintShape{rect: r, color: b.(graphics.Color)})
	p.order = append(p.order, "fill")
}

func (p *recordingPainter) DrawRect(r geometry.Rectangle, w float32, b graphics.Brush) {
	p.borders = append(p.borders, paintShape{rect: r, width: w, color: b.(graphics.Color)})
	p.order = append(p.order, "border")
}

func (p *recordingPainter) DrawLine(geometry.Point, geometry.Point, float32, graphics.Brush) {
	p.lines++
}

func (p *recordingPainter) DrawTextLayout(geometry.Point, typography.TextLayout) {
	p.order = append(p.order, "text")
}

// Color changes must preserve all native metric fields used by Geometry. The
// snapshot owns its slices: a backend must not make both sides change together.
func copyMetrics(l typography.TextLayout) ([]typography.TextLine, []typography.TextCluster) {
	lines, clusters := l.MeasureMetrics()
	lines = slices.Clone(lines)
	for i := range lines {
		lines[i].Clusters = slices.Clone(lines[i].Clusters)
	}
	return lines, slices.Clone(clusters)
}

func rasterRegion(layout typography.TextLayout, scale float32) image.Rectangle {
	w, h := layout.MeasureSize()
	return image.Rect(0, 0, int(math.Ceil(float64(w*scale))), int(math.Ceil(float64(h*scale))))
}

// Count native API entry, not a guessed amount of work from source length.
type countedNativeContext struct {
	typography.Context
	created, sizes, metrics, resizes, colors, destroyed int
}

func (c *countedNativeContext) NewTextLayout(text string, format typography.TextFormat, width, height float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(text, format, width, height)
	if err != nil {
		return nil, err
	}
	c.created++
	return &countedNativeLayout{TextLayout: l, owner: c}, nil
}

type countedNativeLayout struct {
	typography.TextLayout
	owner *countedNativeContext
}

func (l *countedNativeLayout) MeasureSize() (float32, float32) {
	l.owner.sizes++
	return l.TextLayout.MeasureSize()
}

func (l *countedNativeLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	l.owner.metrics++
	return l.TextLayout.MeasureMetrics()
}

func (l *countedNativeLayout) SetSize(width, height float32) {
	l.owner.resizes++
	l.TextLayout.SetSize(width, height)
}

func (l *countedNativeLayout) SetTextColor(start, length int, c color.Color) {
	l.owner.colors++
	l.TextLayout.SetTextColor(start, length, c)
}

func (l *countedNativeLayout) Destroy() {
	l.owner.destroyed++
	l.TextLayout.Destroy()
}
