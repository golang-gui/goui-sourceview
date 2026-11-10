package controls

import (
	"image/color"
	"math"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/style"
)

type shadowPainter struct {
	recordingPainter
	radius geometry.CornerRadius
	shadow graphics.BoxShadow
}

func (p *shadowPainter) DrawBoxShadow(_ geometry.Rectangle, radius geometry.CornerRadius, shadow graphics.BoxShadow) {
	p.radius, p.shadow = radius, shadow
}

func TestStyleBoxKeepsCornerShadowAndBorderSides(t *testing.T) {
	corners := geometry.CornerRadius{TopLeft: 18, BottomRight: 4}
	rule := style.Rule{}.BackgroundColor(rgb(0xffffff)).BorderColor(rgb(0xff0000)).BorderWidth(2).
		CornerRadius(corners).BorderSides(style.BorderBottom).
		Shadow(style.Shadow{Color: color.NRGBA{A: 64}, BlurRadius: 4, Offset: geometry.Point{Y: 2}})
	b := BoxOf(rule.Style)
	p := &shadowPainter{}
	b.Paint(p, geometry.Rect(0, 0, 100, 40))
	if b.Radius != corners || p.radius != corners || p.shadow != b.Shadow {
		t.Fatal("shadow and box must retain the complete corner outline", b, p.radius)
	}
	if b.Sides != style.BorderBottom || len(p.paths) < 2 || len(p.borders) != 0 {
		t.Fatal("single-sided border was painted as a full frame", b, p)
	}
	none := BoxOf(rule.BorderSides(0).Style)
	p = &shadowPainter{}
	none.Paint(p, geometry.Rect(0, 0, 100, 40))
	if len(p.paths) != 1 || len(p.borders) != 0 || b == none {
		t.Fatal("explicit border removal was lost in painting or caching", p)
	}
	if all := BoxOf(style.Rule{}.Style); all.Sides != style.BorderAll {
		t.Fatal("unset border sides must preserve the all-edge default", all)
	}
}

func TestStyleBoxCanonicalizesInvalidCornersForCaching(t *testing.T) {
	r := geometry.CornerRadius{TopLeft: float32(math.NaN()), TopRight: -1, BottomRight: 7, BottomLeft: float32(math.Inf(1))}
	rule := style.Rule{}.CornerRadius(r)
	a, b := BoxOf(rule.Style), BoxOf(rule.Style)
	if a != b || a.Radius != (geometry.CornerRadius{BottomRight: 7}) {
		t.Fatal("invalid corner values caused perpetual appearance invalidation", a.Radius, b.Radius)
	}
}
