package view

import "testing"

func TestCoordinateBoundsAtFloat32PrecisionLimits(t *testing.T) {
	for _, tc := range []struct {
		value       float64
		floor, ceil float32
	}{
		{0, 0, 0},
		{1.5, 1.5, 1.5},
		{16777216, 16777216, 16777216},
		{16777217, 16777216, 16777218},
		{16777219, 16777218, 16777220},
		{-16777217, -16777218, -16777216},
		{33554435, 33554432, 33554436},
	} {
		if got := FloorCoordinate(tc.value); got != tc.floor {
			t.Errorf("floor(%v) = %v, want %v", tc.value, got, tc.floor)
		}
		if got := CeilCoordinate(tc.value); got != tc.ceil {
			t.Errorf("ceil(%v) = %v, want %v", tc.value, got, tc.ceil)
		}
	}
}
