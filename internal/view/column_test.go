package view

import (
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

func TestColumnNativeClustersAndBidi(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	for _, text := range []string{"a\t中e\u0301X", "abc אבג xyz"} {
		l, err := ctx.NewTextLayout(text, typography.TextFormat{Font: typography.FontInfo{Family: "monospace", Size: 12}, TabWidth: 32}, 800, 100)
		if err != nil {
			t.Fatal(err)
		}
		lines, clusters := l.MeasureMetrics()
		g := textcore.NewGeometry(lines, clusters, len(text), 20)
		if text[0:3] != "abc" {
			for _, c := range clusters {
				s, err := ColumnRange(&g, 0, c.X, c.X+c.Width)
				if err != nil {
					t.Fatal(err)
				}
				if s.Range() != (textcore.Range{Start: c.Start, End: c.Start + c.Length}) {
					t.Fatal("cluster split", c, s)
				}
			}
			s, err := ColumnRange(&g, 0, 10000, 11000)
			if err != nil || s.Anchor != len(text) || s.Caret != len(text) {
				t.Fatal("short line", s, err)
			}
		} else {
			rejected := false
			for a := float32(0); a < lines[0].Width; a += 3 {
				for b := a + 3; b < lines[0].Width; b += 3 {
					_, err := ColumnRange(&g, 0, a, b)
					if errors.Is(err, ErrColumnDirection) {
						rejected = true
					}
				}
			}
			if !rejected {
				t.Fatal("ambiguous bidi rectangle accepted")
			}
		}
		l.Destroy()
	}
}
func TestColumnGeometryKeepsPaintViewportAndBounds(t *testing.T) {
	text := strings.Repeat("a", 30*paragraphBlockBytes)
	p := NewParagraphLayout(&paragraphFixtureContext{}, typography.TextFormat{WrapMode: typography.WrapNone}, 800, len(text), func(r textcore.Range) (string, error) { return text[r.Start:r.End], nil })
	defer p.Destroy()
	viewport := geometry.Rect(0, 0, 800, 100)
	for i := 0; i < 4; i++ {
		p.Prepare(viewport, true)
	}
	active := slices.Clone(p.active)
	left := float64(20*paragraphBlockBytes*10 + 100)
	var frame ParagraphGeometry
	var ready bool
	var err error
	for i := 0; i < 10; i++ {
		frame, ready, err = p.ColumnGeometry(left, left+100)
		if err != nil {
			t.Fatal(err)
		}
		if p.viewport != viewport || !slices.Equal(active, p.active) {
			t.Fatal("painting viewport changed")
		}
		if ready {
			break
		}
	}
	if !ready {
		t.Fatal("distant column did not converge")
	}
	s, err := ColumnRange(&frame.Geometry, 0, float32(left-frame.X), float32(left+100-frame.X))
	if err != nil || s.Range() != (textcore.Range{Start: 20*paragraphBlockBytes + 10, End: 20*paragraphBlockBytes + 20}) {
		t.Fatal(s, err)
	}
	if _, _, err = p.ColumnGeometry(0, float64(20*paragraphBlockBytes*10)); !errors.Is(err, ErrColumnWindow) {
		t.Fatal("missing bound", err)
	}
	if len(p.pages) > paragraphPageLimit {
		t.Fatal("native cache exceeded", len(p.pages))
	}
}
