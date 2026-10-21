package main

import (
	"image"
	"image/color"
	"time"

	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/typography"
)

// Optional passive diagnostics. Input still comes from the desktop; these
// observations identify synchronous work, not end-to-end input latency.
type profileApp struct {
	gui.Application
	context typography.Context
}

func (a profileApp) Typography() typography.Context { return a.context }

type profileContext struct{ typography.Context }

func nativeSample(start time.Time, operation string, length int) {
	if elapsed := time.Since(start); elapsed > 3*time.Millisecond {
		emit(map[string]any{"event": "native", "operation": operation, "bytes": length, "at_ns": time.Now().UnixNano(), "ms": float64(elapsed) / 1e6})
	}
}

func (c profileContext) NewTextLayout(text string, format typography.TextFormat, width, height float32) (typography.TextLayout, error) {
	start := time.Now()
	l, err := c.Context.NewTextLayout(text, format, width, height)
	nativeSample(start, "create", len(text))
	if err != nil {
		return l, err
	}
	return profileLayout{l}, nil
}

type profileLayout struct{ typography.TextLayout }

func (l profileLayout) MeasureSize() (float32, float32) {
	start := time.Now()
	w, h := l.TextLayout.MeasureSize()
	nativeSample(start, "size", len(l.Text()))
	return w, h
}
func (l profileLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	start := time.Now()
	lines, clusters := l.TextLayout.MeasureMetrics()
	nativeSample(start, "metrics", len(l.Text()))
	return lines, clusters
}
func (l profileLayout) SetSize(width, height float32) {
	start := time.Now()
	l.TextLayout.SetSize(width, height)
	nativeSample(start, "set-size", len(l.Text()))
}
func (l profileLayout) SetTextColor(start, length int, c color.Color) {
	now := time.Now()
	l.TextLayout.SetTextColor(start, length, c)
	nativeSample(now, "color", len(l.Text()))
}
func (l profileLayout) Rasterize(scale float32, region image.Rectangle, buf []byte) (typography.TextBitmap, error) {
	start := time.Now()
	b, err := l.TextLayout.Rasterize(scale, region, buf)
	nativeSample(start, "raster", len(l.Text()))
	return b, err
}
