package sourceview

import (
	"image/color"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
)

// Use the public focus event path instead of accessing WidgetBase state.
type bracketHost struct{ widget, focused gui.Widget }

func (h *bracketHost) Widget() gui.Widget { return h.widget }

func (h *bracketHost) FocusedWidget() gui.Widget { return h.focused }

func (h *bracketHost) SetFocusedWidget(w gui.Widget) bool { h.focused = w; return true }

func focusBracketFixture(t *testing.T, v *SourceView) {
	t.Helper()
	var d gui.EventDispatcher
	if err := d.DispatchEvent(&bracketHost{widget: v, focused: v}, events.FocusEvent{Focused: true}); err != nil {
		t.Fatal(err)
	}
	if !v.Focused() {
		t.Fatal("focus event did not reach editor")
	}
	v.caretOn = false
}

func bracketFills(p *recordingPainter, c color.NRGBA) []paintShape {
	return slices.DeleteFunc(slices.Clone(p.fills), func(s paintShape) bool { return s.color != graphics.ColorOf(c) })
}

func TestBracketDecorationUsesQuietFillAndInsetBorder(t *testing.T) {
	for _, s := range []*scheme.Scheme{scheme.Light(), scheme.Dark()} {
		t.Run(s.Definition().Name, func(t *testing.T) {
			v, ctx := colorFixture(t)
			v.SetShowLineNumbers(false)
			v.SetFoldingEnabled(false) // Isolate bracket decoration from the independent folding gutter.
			v.SetScheme(s)
			v.Arrange(geometry.Rect(0, 0, 400, 200))
			focusBracketFixture(t, v)
			item := v.paragraph(0)
			native := item.layout.(*shapedLayout)
			foreground := slices.Clone(native.colors)
			created := ctx.created
			p := &recordingPainter{}
			v.Paint(p)
			fills := bracketFills(p, s.Definition().BracketMatchBackground)
			if len(fills) != 2 || len(p.borders) != 2 || p.lines != 0 {
				t.Fatalf("fills=%d borders=%d underlines=%d", len(fills), len(p.borders), p.lines)
			}
			// Mock clusters are 10x20 DIP. The outer JSON braces are bytes 0/15,
			// with 6-DIP content padding and a 1-DIP vertical inset.
			want := []geometry.Rectangle{geometry.Rect(6, 7, 10, 18), geometry.Rect(156, 7, 10, 18)}
			for i, r := range want {
				if fills[i].rect != r || p.borders[i].rect != geometry.Rect(r.X+.5, r.Y+.5, r.Width-1, r.Height-1) || p.borders[i].width != 1 || p.borders[i].color != graphics.ColorOf(s.Definition().BracketMatchBorder) {
					t.Fatal("decoration escaped its character cell", fills, p.borders)
				}
			}
			if slices.Index(p.order, "border") > slices.Index(p.order, "text") {
				t.Fatal("border covered glyphs")
			}
			if v.paragraph(0) != item || ctx.created != created || !slices.Equal(native.colors, foreground) {
				t.Fatal("decoration reshaped or recolored syntax")
			}
		})
	}
}

func TestBracketDecorationSuppressionAndLayering(t *testing.T) {
	v, _ := colorFixture(t)
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false) // Isolate bracket decoration from the independent folding gutter.
	v.Arrange(geometry.Rect(0, 0, 400, 200))
	focusBracketFixture(t, v)
	paint := func() *recordingPainter { p := &recordingPainter{}; v.Paint(p); return p }
	v.SetSearch(SearchQuery{Text: "{"})
	p := paint()
	fills := bracketFills(p, v.colors.BracketMatchBackground)
	if len(fills) != 2 || len(p.borders) != 2 {
		t.Fatal("search suppressed confirmed pair")
	}
	backgroundIndex, searchIndex := -1, -1
	for i, shape := range p.fills {
		if shape.color == graphics.ColorOf(v.colors.BracketMatchBackground) && backgroundIndex < 0 {
			backgroundIndex = i
		}
		if shape.color == graphics.ColorOf(v.colors.SearchBackground) {
			searchIndex = i
		}
	}
	if backgroundIndex < 0 || searchIndex <= backgroundIndex {
		t.Fatal("bracket fill covered search", p.fills)
	}
	for _, tt := range []struct {
		name, text string
		selection  textedit.Selection
	}{
		{"unmatched", "{[}]", textedit.Selection{}},
		{"selected", "{}", textedit.Selection{Caret: 2}},
		{"string", `{"x":"[]"}`, textedit.Selection{Anchor: 6, Caret: 6}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v.document.model.SetText(tt.text)
			v.document.engine.Advance(time.Time{})
			v.SetSelection(tt.selection)
			v.caretOn = false
			p := paint()
			if len(p.borders) != 0 || len(bracketFills(p, v.colors.BracketMatchBackground)) != 0 || p.lines != 0 {
				t.Fatal("unconfirmed pair decorated", p)
			}
		})
	}
	v.document.model.SetText("{}")
	v.SetSelection(textedit.Selection{})
	v.document.engine.Reset(language.JSON)
	p = paint()
	if len(p.borders) != 0 {
		t.Fatal("unresolved lexical state decorated")
	}
	v.document.engine.Advance(time.Time{})
	base := v.colors
	for _, borderOnly := range []bool{true, false} {
		d := base
		if borderOnly {
			d.BracketMatchBackground.A = 0
		} else {
			d.BracketMatchBorder.A = 0
		}
		s, err := scheme.New(d)
		if err != nil {
			t.Fatal(err)
		}
		v.SetScheme(s)
		p = paint()
		if borderOnly && (len(p.borders) != 2 || len(bracketFills(p, base.BracketMatchBackground)) != 0) {
			t.Fatal("border-only style", p)
		}
		if !borderOnly && (len(p.borders) != 0 || len(bracketFills(p, base.BracketMatchBackground)) != 2) {
			t.Fatal("background-only style", p)
		}
	}
	d := v.colors
	d.BracketMatchBackground.A = 0
	d.BracketMatchBorder.A = 0
	s, err := scheme.New(d)
	if err != nil {
		t.Fatal(err)
	}
	v.SetScheme(s)
	p = paint()
	if len(p.borders) != 0 || len(bracketFills(p, d.BracketMatchBackground)) != 0 {
		t.Fatal("transparent scheme did not disable decoration")
	}
}

func TestBracketRectanglesFollowWrappedAndBidiGeometry(t *testing.T) {
	lines := []typography.TextLine{{Start: 0, Length: 1, Height: 20}, {Start: 1, Length: 1, Y: 20, Height: 20}}
	clusters := []typography.TextCluster{{Start: 0, Length: 1, X: 0, Width: 10, Height: 20}, {Start: 1, Length: 1, X: 30, Y: 20, Width: 10, Height: 20, LineIndex: 1, Direction: typography.TextRightToLeft}}
	p := &paragraph{geometry: textedit.NewGeometry(lines, clusters, 2, 20)}
	pair := []textedit.Range{{Start: 10, End: 11}, {Start: 11, End: 12}}
	got := bracketRectangles(p, textedit.Range{Start: 10, End: 12}, geometry.Point{X: -5, Y: 6}, pair)
	want := []geometry.Rectangle{geometry.Rect(-5, 7, 10, 18), geometry.Rect(25, 27, 10, 18)}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	if got := bracketRectangles(p, textedit.Range{Start: 10, End: 12}, geometry.Point{}, pair[:1]); len(got) != 0 {
		t.Fatal("lone candidate decorated", got)
	}
	p.geometry = textedit.NewGeometry([]typography.TextLine{{Length: 2, Height: 20}}, []typography.TextCluster{{Length: 2, Width: 20, Height: 20}}, 2, 20)
	got = bracketRectangles(p, textedit.Range{Start: 10, End: 12}, geometry.Point{}, pair)
	if !reflect.DeepEqual(got, []geometry.Rectangle{geometry.Rect(0, 1, 20, 18)}) {
		t.Fatal("shared shaping cluster painted twice", got)
	}
}

func TestPaintRangeCullsWrappedLines(t *testing.T) {
	const count = 1000
	lines := make([]typography.TextLine, count)
	clusters := make([]typography.TextCluster, count)
	for i := range lines {
		lines[i] = typography.TextLine{Start: i, Length: 1, Y: float32(i * 20), Height: 20}
		clusters[i] = typography.TextCluster{Start: i, Length: 1, Width: 10, LineIndex: i}
	}
	item := &paragraph{geometry: textedit.NewGeometry(lines, clusters, count, 20)}
	v := NewSourceView()
	v.viewport = geometry.Size{Width: 200, Height: 38}
	p := &recordingPainter{}
	// A huge selection includes the whole logical paragraph, but only two
	// visual lines intersect the viewport. Keep full rectangle heights for
	// underlines and let the Painter perform the final pixel clipping.
	v.paintRange(p, item, textedit.Range{End: count}, geometry.Point{Y: -10001},
		textedit.Range{End: count}, color.NRGBA{A: 255})
	if len(p.fills) != 2 {
		t.Fatalf("painted %d selection rectangles, want 2", len(p.fills))
	}
	for i, fill := range p.fills {
		want := geometry.Rect(0, float32(i*20-1), 10, 20)
		if fill.rect != want {
			t.Fatalf("rectangle %d = %v, want %v", i, fill.rect, want)
		}
	}
}

func TestSnapshotLongLineFollowsViewportWithoutShaping(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText(strings.Repeat("界a", 20000)) // 80,000 bytes; 40,000 clusters.
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{X: 350000})
	created := ctx.created
	info := v.Snapshot().TextEditing
	// Fixture advances are 10 DIP/cluster: X=350000 is rune 35000, byte 70000.
	if info == nil || info.VisibleRange.Start <= 64*1024 || info.VisibleRange.Start > 70000 || info.VisibleRange.End < 70000 || len(info.VisibleText) > 64*1024 {
		t.Fatalf("snapshot stayed at line start: %+v", info)
	}
	want, err := v.Document().Model().Slice(textedit.Range{Start: info.VisibleRange.Start, End: info.VisibleRange.End})
	if err != nil || want != info.VisibleText || !v.Document().Model().IsRuneBoundary(info.VisibleRange.Start) || !v.Document().Model().IsRuneBoundary(info.VisibleRange.End) {
		t.Fatal("snapshot bytes or UTF-8 boundaries are invalid")
	}
	if v.Selection().Caret != 0 || ctx.created != created {
		t.Fatal("snapshot moved selection or created native layouts")
	}
	v.document.engine.Advance(time.Time{})
	v.paragraph(0) // Transfer the fallback cache entry to the stable line ID.
	if got := v.Snapshot().TextEditing; got == nil || got.VisibleRange != info.VisibleRange || ctx.created != created {
		t.Fatal("stable line identity lost the snapshot anchor or reshaped text", got)
	}
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	if got := v.Snapshot().TextEditing; got == nil || got.VisibleRange.Start != 0 {
		t.Fatal("snapshot retained a stale horizontal anchor", got)
	}
}

func TestSnapshotLongLinePreservesUTF8Start(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText(strings.Repeat("界ab", 20000))
	observe(t, v)
	v.context = &shapedContext{}
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{X: 450000})
	// Rune 45000 is byte 75000. The 1024-byte prefix lands two bytes inside
	// a three-byte rune; start must advance to 73978, never split that rune.
	info := v.Snapshot().TextEditing
	if info == nil || info.VisibleRange.Start != 73978 || !v.Document().Model().IsRuneBoundary(info.VisibleRange.End) {
		t.Fatalf("invalid UTF-8 excerpt boundary: %+v", info)
	}
}

func TestSnapshotLongLineWithMissingCacheDoesNotShape(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText(strings.Repeat("x", 80000))
	observe(t, v)
	ctx := &shapedContext{}
	v.context = ctx
	t.Cleanup(v.dropLayouts)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{X: 700000})
	v.dropLayouts()
	created := ctx.created
	info := v.Snapshot().TextEditing
	if info == nil || info.VisibleRange.Start != 0 || len(info.VisibleText) != 64*1024 || ctx.created != created {
		t.Fatal("snapshot created layouts for a missing cache")
	}
}
