// 原生调用计数、度量副本与光栅范围；Context 由各平台文件提供，调用方管理线程和资源。
package sourceview

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/golang-gui/goui/platform/typography"
)

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
