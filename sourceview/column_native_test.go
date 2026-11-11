package sourceview

import (
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

func TestColumnWrappedNativeReferenceAndResize(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	v := NewSourceView()
	observe(t, v)
	v.context = ctx
	defer v.dropLayouts()
	v.font = typography.FontInfo{Family: "monospace", Size: 12}
	v.SetWrapMode(WrapWordChar)
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false)
	text := strings.Repeat("aa\t中e\u0301 bb ", 30)
	v.Document().SetText(text)
	v.LayoutVisible(geometry.Size{Width: 180, Height: 300}, geometry.Point{})
	p := v.paragraph(0)
	if p == nil {
		t.Fatal("layout")
	}
	// A standalone complete native paragraph supplies the display-line reference.
	native, err := ctx.NewTextLayout(text, typography.TextFormat{Font: v.font, WrapMode: v.wrap, TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}, v.paragraphWidth(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer native.Destroy()
	lines, clusters := native.MeasureMetrics()
	if len(lines) < 5 {
		t.Fatal("sample too short")
	}
	reference := textcore.NewGeometry(lines, clusters, len(text), v.lineHeight)
	var expected []TextSelection
	for i := 1; i <= 4; i++ {
		s, err := view.ColumnRange(&reference, i, 10, 45)
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, s.Selection)
	}
	expected = append([]TextSelection{expected[3]}, expected[:3]...)
	c := &columnSelection{state: v.operationState(), layout: v.layoutEpoch, anchor: textcore.Position{Offset: lines[1].Start}, target: textcore.Position{Offset: lines[4].Start}, left: 10, right: 45, before: v.cursors(), pending: true}
	v.columnSelection = c
	for i := 0; i < 100 && c.pending; i++ {
		_, err = v.advanceColumn(c, time.Time{})
		if err != nil && err != errSelectionGeometry {
			t.Fatal(err)
		}
	}
	if c.pending || !slices.Equal(v.Selections(), expected) {
		t.Fatal(v.Selections(), expected)
	}
	v.LayoutVisible(geometry.Size{Width: 260, Height: 300}, geometry.Point{})
	v.columnStep(time.Time{})
	if !slices.Equal(v.Selections(), expected) {
		t.Fatal("completed column moved after resize", v.Selections())
	}
}
