package view

import (
	"image/color"
	"unicode/utf8"

	"github.com/golang-gui/goui/core/geometry"
)

// MinimapCells samples a bounded source fragment; colorAt is called only for visible ink.
func MinimapCells(text string, columns, tab int, colorAt func(int) color.NRGBA) []color.NRGBA {
	pixels := make([]color.NRGBA, columns)
	column := 0
	for at := 0; at < len(text) && column < columns; {
		ch, size := utf8.DecodeRuneInString(text[at:])
		if ch == '\t' {
			column += tab - column%tab
		} else {
			if ch != ' ' && ch != '\r' && ch != '\n' {
				c := colorAt(at)
				// Ordinary sRGB NRGBA; alpha makes the overview quieter than source text.
				c.A = uint8(uint16(c.A) * 180 / 255)
				pixels[column] = c
			}
			column++
		}
		at += size
	}
	return pixels
}
func MinimapThumb(r geometry.Rectangle, total, extent, viewportHeight, offset, dragThumb float32, dragging bool) geometry.Rectangle {
	h := min(extent, max(8, extent*viewportHeight/max(1, total)))
	if dragging {
		h = dragThumb
	}
	y := float32(0)
	if total > viewportHeight {
		y = (extent - h) * max(0, min(1, offset/(total-viewportHeight)))
	}
	return geometry.Rect(r.X, y, r.Width, h)
}
