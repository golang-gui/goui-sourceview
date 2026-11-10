package view

import "math"

// CeilCoordinate rounds toward positive infinity so content extents include
// their final caret even when large coordinates exceed float32 precision.
func CeilCoordinate(value float64) float32 {
	n := float32(value)
	if float64(n) < value {
		n = math.Nextafter32(n, float32(math.Inf(1)))
	}
	return n
}

// FloorCoordinate rounds toward negative infinity, retaining the leading edge
// of reveal rectangles when content coordinates are converted to float32.
func FloorCoordinate(value float64) float32 {
	n := float32(value)
	if float64(n) > value {
		n = math.Nextafter32(n, float32(math.Inf(-1)))
	}
	return n
}
