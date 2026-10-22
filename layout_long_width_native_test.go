package sourceview

import (
	"image/color"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

// Exercise a >32-KiB paragraph through Context/TextLayout alone. Full native
// context, recent widths, color updates and snapshot lifetime remain supported
// after withdrawing the separate retained-glyph interfaces.
func TestNativeLongWidthsUseExistingLayoutContract(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	native, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	ctx := &widthCacheNativeContext{Context: native}
	v := NewSourceView()
	v.context = ctx
	defer v.dropLayouts()
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	text := "start \u202e" + strings.Repeat("abc 123 العربية עברית e\u0301 ffi 👩‍💻\t ", 600) + "END\u202c"
	if len(text) < 32768 {
		t.Fatal("fixture must cover the removed long-paragraph branch")
	}
	v.Document().Model().SetText(text)
	show := func(width, height float32) *paragraph {
		v.LayoutVisible(geometry.Size{Width: width, Height: height}, geometry.Point{})
		return v.paragraph(0)
	}
	a := show(240, 200)
	if a == nil || a.fragment.LineCount() < 100 {
		t.Fatal("missing measured long paragraph")
	}
	position := textedit.Position{Offset: strings.Index(text, "END")}
	wantCaret := a.geometry.Caret(position)
	fragment := a.fragment
	w, h := a.layout.Size()
	reference, err := native.NewTextLayout(text, a.layout.Format(), w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer reference.Destroy()
	wantLines, wantClusters := copyMetrics(reference)
	gotLines, gotClusters := fragment.Metrics()
	if !reflect.DeepEqual(gotLines, wantLines) || !reflect.DeepEqual(gotClusters, wantClusters) {
		t.Fatal("cached values differ from complete native context")
	}
	b := show(310, 200)
	if b == a || b.layout == a.layout {
		t.Fatal("different widths shared mutable native resources")
	}
	before := *ctx
	for range 3 {
		if show(240, 280) != a || show(310, 320) != b || *ctx != before {
			t.Fatal("restoring width or changing height repeated native work", before, *ctx)
		}
	}
	style := b.style
	style.selection = textedit.Range{Start: 10, End: 100}
	style.focused = true
	v.colors.SelectionForeground = color.NRGBA{R: 220, A: 255}
	if !v.colorParagraph(v.paragraphKey(0), b, style, nil, activeFocus(v)) {
		t.Fatal("foreground update failed")
	}
	if len(b.colors) == 0 || ctx.colors == before.colors || ctx.created != before.created || ctx.metrics != before.metrics {
		t.Fatal("foreground update did not reuse the measured paragraph")
	}
	v.dropLayouts()
	if ctx.created != ctx.destroyed {
		t.Fatal("native width resources leaked", ctx.created, ctx.destroyed)
	}
	g := fragment.Geometry(v.lineHeight)
	if g.Caret(position) != wantCaret {
		t.Fatal("ordinary snapshot changed after native cache release")
	}
}
