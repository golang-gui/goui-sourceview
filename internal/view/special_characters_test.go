package view

import (
	"strings"
	"testing"
	"time"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

func TestSpecialCharacterClassificationAndSafeNames(t *testing.T) {
	for _, r := range []rune{0, 8, 11, 27, 31, 127, 128, 159, 0xad, 0x34f, 0x61c, 0x200b, 0x200d, 0x202e, 0x2066, 0x2069, 0xfeff} {
		id := specialCharacterID(r)
		if id < 0 || id >= 128 {
			t.Fatalf("special U+%04X not classified", r)
		}
		label := specialCharacterLabel(id)
		if !strings.HasPrefix(label, "U+") || strings.ContainsRune(label, r) {
			t.Fatalf("unsafe label for U+%04X: %q", r, label)
		}
	}
	for _, r := range []rune{'\t', '\r', '\n', ' ', 'A', '中', 'א', 0x301, 0xfe0f, 0x1f600} {
		if specialCharacterID(r) >= 0 {
			t.Fatalf("ordinary U+%04X classified", r)
		}
	}
	if specialCharacterLabel(specialCharacterID(0x200b)) != "U+200B ZERO WIDTH SPACE" {
		t.Fatal("incorrect Unicode label")
	}
}

func TestNativeLayoutTextKeepsOffsetsAndSourceValue(t *testing.T) {
	original := "A\x00中\x00B\u200b"
	display := NativeLayoutText(original)
	if display != "A 中 B\u200b" || len(display) != len(original) || original != "A\x00中\x00B\u200b" {
		t.Fatal("NUL display adaptation changed offsets or original text", display)
	}
	if NativeLayoutText("中\u200bB") != "中\u200bB" {
		t.Fatal("NUL-free text changed")
	}
}

func TestSpecialLayoutZeroWidthGroupingClippingAndBudget(t *testing.T) {
	text := "A\u200b\u200eB\x00C"
	clusters := []typography.TextCluster{
		{Start: 0, Length: 1, X: 0, Width: 10},
		{Start: 1, Length: 3, X: 10, Width: 0},
		{Start: 4, Length: 3, X: 10, Width: 0},
		{Start: 7, Length: 1, X: 10, Width: 10},
		{Start: 8, Length: 1, X: 20, Width: 0},
		{Start: 9, Length: 1, X: 20, Width: 10},
	}
	lines := []typography.TextLine{{Start: 0, Length: len(text), Width: 30, Height: 20, Clusters: clusters}}
	g := textcore.NewGeometry(lines, clusters, len(text), 20)
	s := NewSpecialLayout(text, 0, g, geometry.Rect(0, 0, 100, 100))
	s.Advance(time.Now().Add(-time.Second))
	if !s.Pending() || len(s.markers) != 0 {
		t.Fatal("expired budget advanced work")
	}
	for s.Pending() {
		s.Advance(time.Time{})
	}
	if len(s.markers) != 2 {
		t.Fatal("zero-width marks were dropped or not grouped", s.markers)
	}
	labels := s.Tooltip(geometry.Point{X: 10, Y: 10})
	if labels != "U+200B ZERO WIDTH SPACE\nU+200E LEFT-TO-RIGHT MARK" {
		t.Fatal("coincident tooltip", labels)
	}
	if s.Tooltip(geometry.Point{X: 20, Y: 10}) != "U+0000 NUL" {
		t.Fatal("NUL marker missing")
	}
	if s.positions != nil {
		t.Fatal("completed index retained construction map")
	}
	clipped := NewSpecialLayout(text, 0, g, geometry.Rect(18, 0, 10, 20))
	for clipped.Pending() {
		clipped.Advance(time.Time{})
	}
	if len(clipped.markers) != 1 || clipped.markers[0].Cell.X != 20 {
		t.Fatal("offscreen markers retained")
	}
	if clipped.Tooltip(geometry.Point{X: 17, Y: 10}) != "" || clipped.Tooltip(geometry.Point{X: 20, Y: 20}) != "" {
		t.Fatal("tooltip escaped the viewport clip")
	}
	long := NewSpecialLayout(strings.Repeat("x", 5000), 0, g, geometry.Rect(0, 0, 100, 100))
	long.Advance(time.Time{})
	if !long.Pending() || long.at != 4096 {
		t.Fatal("unbounded scan", long.at)
	}
	long.Advance(time.Time{})
	if long.Pending() {
		t.Fatal("scan did not converge")
	}
}
