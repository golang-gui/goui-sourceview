package sourceview

import (
	"image/color"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/platform/typography/pango"
)

type shapedSourceContext struct {
	typography.Context
	created, destroyed, colors int
}

func (c *shapedSourceContext) NewGlyphLayout(text string, f typography.TextFormat) (typography.GlyphLayout, error) {
	l, err := c.Context.(typography.GlyphContext).NewGlyphLayout(text, f)
	if err != nil {
		return nil, err
	}
	c.created++
	return &shapedSourceLayout{GlyphLayout: l, owner: c}, nil
}

type shapedSourceLayout struct {
	typography.GlyphLayout
	owner *shapedSourceContext
}

func (l *shapedSourceLayout) Destroy() { l.owner.destroyed++; l.GlyphLayout.Destroy() }
func (l *shapedSourceLayout) SetTextColor(start, length int, c color.Color) {
	l.owner.colors++
	l.GlyphLayout.SetTextColor(start, length, c)
}

func TestSourceShapedWidthsAndColorsKeepNativeFacts(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := pango.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	ctx := &shapedSourceContext{Context: native}
	v := NewSourceView()
	v.context = ctx
	defer v.dropLayouts()
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	text := "start \u202e" + strings.Repeat("abc 123 العربية עברית e\u0301 ffi 👩‍💻\t ", 1000) + "END\u202c"
	v.Document().Model().SetText(text)
	v.LayoutVisible(geometry.Size{Width: 240, Height: 200}, geometry.Point{})
	a := v.paragraph(0)
	if a == nil || a.shape == nil || a.fragment.LineCount() < 100 {
		t.Fatal("long Unicode input missed shaped path")
	}
	position := textedit.Position{Offset: strings.Index(text, "END")}
	want := a.geometry.Caret(position)
	for width := float32(250); width < 270; width++ {
		v.LayoutVisible(geometry.Size{Width: width, Height: 200}, geometry.Point{})
		b := v.paragraph(0)
		if b.shape != a.shape || b == a {
			t.Fatal("width change did not share immutable source")
		}
	}
	if ctx.created != 1 || ctx.destroyed != 0 {
		t.Fatal("width changes shaped/released source", ctx.created, ctx.destroyed)
	}
	b := v.paragraph(0)
	style := b.style
	style.selection = textedit.Range{Start: 10, End: 100}
	style.focused = true
	v.colors.SelectionForeground = color.NRGBA{R: 220, A: 255}
	if !v.colorParagraph(v.paragraphKey(0), b, style, nil, activeFocus(v)) {
		t.Fatal("paint recoloring failed")
	}
	if len(b.glyphColors) == 0 || ctx.colors != 0 || ctx.created != 1 {
		t.Fatal("foreground changes reshaped immutable source")
	}
	// A snapshot outlives eviction of its original width and the native source.
	fragment := a.fragment
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("native source leaked", ctx.created, ctx.destroyed)
	}
	snapshotGeometry := fragment.Geometry(v.lineHeight)
	if got := snapshotGeometry.Caret(position); got != want {
		t.Fatal("eviction changed immutable caret snapshot", got, want)
	}
}
