package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
)

func TestDotShapeRequiresCircularDrawAndIndependentPixelFootprint(t *testing.T) {
	before := image.NewRGBA(image.Rect(0, 0, 30, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 30; x++ {
			before.Set(x, y, color.White)
		}
	}
	makePixels := func(bounds image.Rectangle) image.Image {
		after := image.NewRGBA(before.Bounds())
		copy(after.Pix, before.Pix)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				after.Set(x, y, color.Black)
			}
		}
		return after
	}
	// A subpixel circle may cover two columns and one row. Geometry is checked
	// independently, so the same pixels produced by a rectangle must fail.
	cell := geometry.Rect(0, 0, 3.5, 16)
	circle := circleRecord{center: geometry.Point{X: 1.75, Y: 8}, rx: .5, ry: .5}
	pixels := makePixels(image.Rect(1, 7, 3, 8))
	if err := dotShapeError(before, pixels, cell, 3.5, 1, []circleRecord{circle}); err != nil {
		t.Fatal("valid subpixel footprint", err)
	}
	for _, circles := range [][]circleRecord{nil, {{center: circle.center, rx: .5, ry: .25}}, {{center: geometry.Point{X: 2.75, Y: 8}, rx: .5, ry: .5}}} {
		if dotShapeError(before, pixels, cell, 3.5, 1, circles) == nil {
			t.Fatal("noncircular or displaced draw accepted")
		}
	}
	large := geometry.Rect(0, 0, 14, 20)
	circle = circleRecord{center: geometry.Point{X: 7, Y: 10}, rx: 2, ry: 2}
	if dotShapeError(before, makePixels(image.Rect(5, 9, 9, 10)), large, 14, 1, []circleRecord{circle}) == nil {
		t.Fatal("4x1 raster strip accepted despite circular draw")
	}
	if dotShapeError(before, before, large, 14, 1, []circleRecord{circle}) == nil {
		t.Fatal("invisible circle accepted")
	}
}
