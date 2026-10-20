package sourceview

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

func completionFixture(t *testing.T, items []Completion) (*completionList, *shapedContext) {
	t.Helper()
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	ctx := &shapedContext{}
	v.context = ctx
	s := &completionSession{items: items, prefix: "tr"}
	l := newCompletionList(v, s)
	s.list = l
	v.completion = s
	t.Cleanup(func() {
		v.completion = nil
		l.release()
		if ctx.created != ctx.destroyed {
			t.Errorf("completion leaked layouts: %d/%d", ctx.created, ctx.destroyed)
		}
	})
	return l, ctx
}
func arrangeCompletion(l *completionList) {
	if l.session.panel == nil {
		l.session.panel = newCompletionPanel(l)
	}
	p := l.session.panel
	m := p.Measure(layout.Unbounded())
	p.Arrange(geometry.Rect(0, 0, m.Width, m.Height))
}

type completionDraw struct {
	origin geometry.Point
	text   string
	clip   geometry.Rectangle
}
type completionPainter struct {
	bracketPainter
	clip    geometry.Rectangle
	stack   []geometry.Rectangle
	text    []completionDraw
	rounded []completionRound
}
type completionRound struct {
	rect   geometry.Rectangle
	radius float32
	color  graphics.Color
}

func (p *completionPainter) Save() { p.stack = append(p.stack, p.clip) }
func (p *completionPainter) Restore() {
	p.clip = p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
}
func (p *completionPainter) SetClipRect(r geometry.Rectangle) { p.clip = r }
func (p *completionPainter) FillRoundRect(r geometry.Rectangle, radius float32, b graphics.Brush) {
	p.rounded = append(p.rounded, completionRound{r, radius, b.(graphics.Color)})
}
func (p *completionPainter) DrawTextLayout(at geometry.Point, l typography.TextLayout) {
	p.text = append(p.text, completionDraw{at, l.Text(), p.clip})
}

func TestCompletionPanelColumnsPaletteAndColorCaching(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			l, ctx := completionFixture(t, []Completion{{Label: "transform", Detail: "JSON"}, {Label: "true", Detail: "document"}})
			l.view.completion = nil
			scope := gui.NewStyleScope()
			scope.SetStyleSheet(modern.Sheet(modern.Options{Dark: dark}))
			scope.SetChild(l.view)
			l.readAppearance()
			l.view.completion = l.session
			arrangeCompletion(l)
			p := &completionPainter{}
			l.session.panel.Paint(p)
			l.Paint(p)
			if len(l.rows) != 2 || len(p.text) != 4 || len(p.borders) != 0 {
				t.Fatal("missing columns or content redrew the popup shell", p)
			}
			// Mock glyphs are 10x20 DIP: 28-DIP rows. Drawing coordinates
			// are content-local; the panel adds another 6 DIP on every side.
			want := []completionDraw{
				{geometry.Point{X: 6, Y: 4}, "transform", geometry.Rect(6, 0, 90, 28)},
				{geometry.Point{X: 182, Y: 4}, "JSON", geometry.Rect(112, 0, 110, 28)},
				{geometry.Point{X: 6, Y: 32}, "true", geometry.Rect(6, 28, 90, 28)},
				{geometry.Point{X: 142, Y: 32}, "document", geometry.Rect(112, 28, 110, 28)},
			}
			if !slices.Equal(p.text, want) {
				t.Fatalf("column geometry: got %+v", p.text)
			}
			c := l.colors
			if len(p.fills) != 0 || len(p.rounded) != 1 || p.rounded[0] != (completionRound{geometry.Rect(2, 1, 224, 26), 4, graphics.ColorOf(c.SelectedBackground)}) {
				t.Fatal("selection lost its inset, radius or popup palette", p.fills, p.rounded)
			}
			for index, row := range l.rows {
				label, detail := row.label.native.(*shapedLayout), row.detail.native.(*shapedLayout)
				base, match, secondary := c.Foreground, c.MatchForeground, c.DetailForeground
				if index == 0 {
					base, match, secondary = c.SelectedForeground, c.SelectedMatchForeground, c.SelectedDetailForeground
				}
				for i, got := range label.colors {
					expected := base
					if i < 2 {
						expected = match
					}
					if got != expected {
						t.Fatal("prefix color", index, i, got, expected)
					}
				}
				for _, got := range detail.colors {
					if got != secondary {
						t.Fatal("detail prominence", index, got, secondary)
					}
				}
				if label.format.Font != l.font || detail.format.Font != l.detailFont() {
					t.Fatal("column fonts")
				}
			}
			created := ctx.created
			calls := l.rows[0].label.native.(*shapedLayout).colorCalls
			l.Paint(&completionPainter{})
			if ctx.created != created || l.rows[0].label.native.(*shapedLayout).colorCalls != calls {
				t.Fatal("unchanged frame rebuilt or recolored")
			}
			l.session.index = 1
			l.refresh()
			l.Paint(&completionPainter{})
			if ctx.created != created {
				t.Fatal("selection reshaped rows")
			}
			if l.rows[0].label.native.(*shapedLayout).colors[0] != c.MatchForeground || l.rows[1].label.native.(*shapedLayout).colors[0] != c.SelectedMatchForeground {
				t.Fatal("selection did not restore match colors")
			}
		})
	}
}

func TestCompletionSelectionUsesStyledRadius(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sheet  style.StyleSheet
		radius float32
	}{
		{"basic", basic.Sheet(), 2},
		{"modern-light", modern.Sheet(modern.Options{}), 4},
		{"modern-dark", modern.Sheet(modern.Options{Dark: true}), 4},
		{"square-override", style.Sheet(append(modern.Rules(modern.Options{}), style.Name("sourceview.completion").Part("selected").Radius(0))...), 0},
		{"independent-override", style.Sheet(append(modern.Rules(modern.Options{}), style.Name("sourceview.completion").Radius(32), style.Name("sourceview.completion").Part("selected").Radius(9))...), 9},
		{"clamped-override", style.Sheet(append(modern.Rules(modern.Options{}), style.Name("sourceview.completion").Part("selected").Radius(1000))...), 13},
		{"negative-override", style.Sheet(append(modern.Rules(modern.Options{}), style.Name("sourceview.completion").Part("selected").Radius(-1))...), 0},
		{"nan-override", style.Sheet(append(modern.Rules(modern.Options{}), style.Name("sourceview.completion").Part("selected").Radius(float32(math.NaN())))...), 0},
		{"infinite-override", style.Sheet(append(modern.Rules(modern.Options{}), style.Name("sourceview.completion").Part("selected").Radius(float32(math.Inf(1))))...), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, _ := completionFixture(t, []Completion{{Label: "true", Detail: "JSON"}})
			l.view.completion = nil
			themedScope(l.view, tc.sheet)
			l.readAppearance()
			l.view.completion = l.session
			arrangeCompletion(l)
			p := &completionPainter{}
			l.Paint(p)
			// One 28-DIP row; custom shell padding is outside this content.
			r := geometry.Rect(2, 1, l.viewport.Width-4, 26)
			c := graphics.ColorOf(l.colors.SelectedBackground)
			if tc.radius == 0 {
				if len(p.rounded) != 0 || len(p.fills) != 1 || p.fills[0] != (paintShape{rect: r, color: c}) {
					t.Fatal("explicit square selection was not respected", p.fills, p.rounded)
				}
			} else if len(p.fills) != 0 || len(p.rounded) != 1 || p.rounded[0] != (completionRound{r, tc.radius, c}) {
				t.Fatal("selection did not use its independent style radius", p.fills, p.rounded)
			}
		})
	}
}

func TestCompletionSelectionPaintKeepsWholeScrolledRow(t *testing.T) {
	items := make([]Completion, 32)
	for i := range items {
		items[i] = Completion{Label: "true", Detail: "JSON"}
	}
	l, _ := completionFixture(t, items)
	arrangeCompletion(l)
	l.session.panel.scroll.SetScrollY(7)
	for _, tc := range []struct {
		index int
		y     float32
		hitY  float32
	}{
		{0, -6, 0}, {8, 218, 223},
	} {
		l.session.index = tc.index
		p := &completionPainter{}
		l.Paint(p)
		want := completionRound{geometry.Rect(2, tc.y, l.viewport.Width-4, 26), 2, graphics.ColorOf(l.colors.SelectedBackground)}
		if len(p.fills) != 0 || len(p.rounded) != 1 || p.rounded[0] != want {
			t.Fatal("partial selection was reshaped at the viewport edge", p.fills, p.rounded)
		}
		if index, ok := l.itemAt(geometry.Point{Y: tc.hitY}); !ok || index != tc.index {
			t.Fatal("visual inset shrank the row hit area", index, ok)
		}
	}
	l.session.index = 0
	l.LayoutVisible(geometry.Size{Width: 5, Height: 28}, geometry.Point{})
	p := &completionPainter{}
	l.Paint(p)
	if len(p.rounded) != 1 || p.rounded[0].rect != geometry.Rect(2, 1, 1, 26) || p.rounded[0].radius != .5 {
		t.Fatal("narrow viewport did not clamp selection geometry", p.rounded)
	}
}

func TestCompletionWidthScrollingAndConstrainedHitAreas(t *testing.T) {
	items := make([]Completion, 64)
	for i := range items {
		items[i] = Completion{Label: "true", Detail: "JSON"}
	}
	items[63] = Completion{Label: strings.Repeat("long", 100), Detail: strings.Repeat("explanation ", 40)}
	l, ctx := completionFixture(t, items)
	arrangeCompletion(l)
	l.Paint(&completionPainter{})
	if l.width != 520 || l.visible != 8 || ctx.created-ctx.destroyed != 16 {
		t.Fatal("unbounded realization", l.width, l.visible, ctx.created-ctx.destroyed)
	}
	size := l.Measure(layout.Unbounded()).Size
	for _, index := range []int{63, 20, 0} {
		l.session.index = index
		l.refresh()
		p := &completionPainter{}
		l.Paint(p)
		if l.Measure(layout.Unbounded()).Size != size || ctx.created-ctx.destroyed > 16 {
			t.Fatal("scroll resized panel or leaked rows")
		}
		if index < l.first || index >= l.first+l.visible {
			t.Fatal("selection scrolled outside list")
		}
		info := l.Snapshot()
		if info.VisibleStart != l.first || info.VisibleEnd != l.first+8 || info.ItemCount != 64 || len(info.Children) != 8 {
			t.Fatal("visible semantic range", info)
		}
		for slot, row := range info.Children {
			r := row.Bounds
			if got, ok := l.itemAt(geometry.Point{X: r.X - info.Bounds.X + .5, Y: r.Y - info.Bounds.Y + .5}); !ok || got != l.first+slot {
				t.Fatal("snapshot/click disagreement")
			}
			if row.Attributes["sourceview.completion.detail"] != items[l.first+slot].Detail {
				t.Fatal("lost full detail")
			}
		}
		for _, point := range []geometry.Point{{X: -1, Y: 7}, {X: 519, Y: 7}, {X: 20, Y: -1}, {X: 20, Y: 224}, {X: 510, Y: 20}} {
			if _, ok := l.itemAt(point); ok {
				t.Fatal("outside viewport or scrollbar activated a row", point)
			}
		}
		if l.session.panel.scroll.Snapshot().MaxScrollY != 56*28 {
			t.Fatal("scroll extent did not include all candidates")
		}
	}
	l.session.panel.Arrange(geometry.Rect(0, 0, 150, 70))
	l.Paint(&completionPainter{})
	if l.visible != 3 || len(l.rows) != 3 || ctx.created-ctx.destroyed > 6 {
		t.Fatal("short workarea lost partially visible rows")
	}
	if index, ok := l.itemAt(geometry.Point{X: 20, Y: 57}); !ok || index != l.first+2 {
		t.Fatal("visible fragment of bottom row was not interactive")
	}
	if _, ok := l.itemAt(geometry.Point{X: 20, Y: 58}); ok {
		t.Fatal("clipped bottom row was interactive")
	}
	l.session.panel.Arrange(geometry.Rect(0, 0, 10, 10))
	l.Paint(&completionPainter{})
	if len(l.Snapshot().Children) != 0 || len(l.rows) != 0 {
		t.Fatal("tiny workarea exposed rows")
	}
}

func TestCompletionPixelScrollingPreservesSelectionAndOverlappingLayouts(t *testing.T) {
	items := make([]Completion, 32)
	for i := range items {
		items[i] = Completion{Label: "true", Detail: "JSON"}
	}
	l, ctx := completionFixture(t, items)
	arrangeCompletion(l)
	l.Paint(&completionPainter{})
	created := ctx.created
	retained := l.rows[1].label.native
	l.session.panel.scroll.SetScrollY(7)
	l.Paint(&completionPainter{})
	if l.session.index != 0 || l.first != 0 || l.visible != 9 || ctx.created != created+2 || len(l.rows) != 9 {
		t.Fatal("pixel scroll changed selection, rebuilt retained rows, or lost partial rows")
	}
	if l.rows[1].label.native != retained {
		t.Fatal("overlapping row lost its layout")
	}
	info := l.Snapshot()
	if info.Children[0].Bounds.Y != info.Bounds.Y || info.Children[0].Bounds.Height != 21 || info.Children[8].Bounds.Height != 7 {
		t.Fatal("partial row bounds escaped the viewport", info.Children)
	}
	if index, ok := l.itemAt(geometry.Point{X: 1, Y: 223}); !ok || index != 8 {
		t.Fatal("partial row hit did not include pixel offset")
	}
	l.session.panel.scroll.SetScrollY(28)
	l.Paint(&completionPainter{})
	if l.first != 1 || l.visible != 8 || ctx.created != created+2 || ctx.created-ctx.destroyed != 16 || l.rows[1].label.native != retained {
		t.Fatal("scrolling a full row recreated layouts or retained offscreen resources")
	}
	l.session.index = 31
	l.refresh()
	if l.first != 24 || l.session.panel.scroll.ScrollY() != 24*28 {
		t.Fatal("keyboard reveal did not reach the final candidate")
	}
}

func TestCompletionEllipsisPreservesNativeClusters(t *testing.T) {
	l, _ := completionFixture(t, []Completion{{Label: "unused"}})
	for _, tt := range []struct {
		value string
		width float32
		want  string
		kept  int
	}{
		{"e\u0301ABC", 20, "e\u0301…", 3}, {"你好世界", 30, "你好…", 6}, {"abc", 30, "abc", 3}, {"abc", 10, "…", 0}, {"abc", 9, "", 0},
	} {
		b := l.build()
		text, err := b.fit(tt.value, l.font, l.colors.Foreground, tt.width)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if text != nil {
			got = text.native.Text()
			if text.width > tt.width || text.kept != tt.kept {
				t.Fatal("invalid fit", text)
			}
		}
		if got != tt.want {
			t.Fatalf("%q at %g: %q, want %q", tt.value, tt.width, got, tt.want)
		}
		b.destroy()
	}
	clusters := []typography.TextCluster{{Start: 0, Length: 11}, {Start: 0, Length: 4}, {Start: 11, Length: 1}}
	if cuts := completionCuts("👩‍💻x", clusters); !slices.Equal(cuts, []int{0, 11, 12}) {
		t.Fatal("overlapping emoji cluster split", cuts)
	}
}

func TestCompletionNativeResourcesStayBoundedWhileEliding(t *testing.T) {
	items := make([]Completion, 64)
	for i := range items {
		items[i] = Completion{Label: strings.Repeat("word", 100), Detail: strings.Repeat("description", 80)}
	}
	l, ctx := completionFixture(t, items)
	peak := 0
	hook := &completionHookContext{Context: ctx}
	hook.create = func(typography.TextLayout) { peak = max(peak, ctx.created-ctx.destroyed) }
	l.view.context = hook
	arrangeCompletion(l)
	for _, index := range []int{0, 20, 63} {
		l.session.index = index
		l.refresh()
		l.Paint(&completionPainter{})
		if ctx.created-ctx.destroyed != 16 || peak > 17 {
			t.Fatal("ellipsis accumulated native probes", peak, ctx.created, ctx.destroyed)
		}
	}
	if peak != 17 {
		t.Fatal("test did not exercise a full row set plus an ellipsis probe", peak)
	}
	l.session.panel.scroll.SetScrollY(7)
	l.Paint(&completionPainter{})
	if l.visible != 9 || ctx.created-ctx.destroyed != 18 || peak != 19 {
		t.Fatal("partial rows or ellipsis probes exceeded their independent bounds", peak, ctx.created, ctx.destroyed)
	}
}

type completionHookContext struct {
	typography.Context
	create        func(typography.TextLayout)
	failAt, calls int
}

func (c *completionHookContext) NewTextLayout(s string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	c.calls++
	if c.failAt == c.calls {
		return nil, errors.New("completion layout failed")
	}
	l, err := c.Context.NewTextLayout(s, f, w, h)
	if c.create != nil {
		c.create(l)
	}
	return l, err
}
func TestCompletionBuildCancellationAndFailureReleaseResources(t *testing.T) {
	t.Run("recursive measurement and painting", func(t *testing.T) {
		l, ctx := completionFixture(t, []Completion{{Label: "true", Detail: "JSON"}})
		hook := &completionHookContext{Context: ctx}
		hook.create = func(typography.TextLayout) {
			l.Measure(layout.Unbounded())
			l.Paint(&completionPainter{})
		}
		l.view.context = hook
		arrangeCompletion(l)
		l.Paint(&completionPainter{})
		if len(l.rows) != 1 || ctx.created-ctx.destroyed != 2 {
			t.Fatal("recursive native callback published duplicate rows", len(l.rows), ctx.created, ctx.destroyed)
		}
	})
	t.Run("creation cancellation", func(t *testing.T) {
		l, ctx := completionFixture(t, []Completion{{Label: "true", Detail: "JSON"}})
		hook := &completionHookContext{Context: ctx}
		hook.create = func(typography.TextLayout) { hook.create = nil; l.view.completion = nil; l.release() }
		l.view.context = hook
		l.Measure(layout.Unbounded())
		if l.measured || l.err != nil || ctx.created != ctx.destroyed {
			t.Fatal("published cancelled measurement or leaked a probe")
		}
	})
	t.Run("row failure", func(t *testing.T) {
		l, ctx := completionFixture(t, []Completion{{Label: "true", Detail: "JSON"}, {Label: "truncate", Detail: "word"}})
		arrangeCompletion(l)
		closed, reported := 0, 0
		l.session.shown = true
		l.session.popup = closingPopup{onClose: func() { closed++ }}
		l.view.ConnectError(func(error) { reported++ })
		l.view.context = &completionHookContext{Context: ctx, failAt: 3}
		l.Paint(&completionPainter{})
		if l.view.completion != nil || closed != 1 || reported != 1 || ctx.created != ctx.destroyed {
			t.Fatal("failure was not atomic", closed, reported, ctx.created, ctx.destroyed)
		}
	})
	t.Run("color cancellation", func(t *testing.T) {
		l, ctx := completionFixture(t, []Completion{{Label: "true", Detail: "JSON"}, {Label: "truncate"}})
		arrangeCompletion(l)
		l.ensureRows()
		l.rows[0].label.native.(*shapedLayout).onColor = func() { l.view.completion = nil; l.release() }
		l.Paint(&completionPainter{})
		if ctx.created != ctx.destroyed {
			t.Fatal("color cancellation leaked rows")
		}
	})
}

func TestCompletionUnicodePrefixAndWordAnchor(t *testing.T) {
	for _, tt := range []struct {
		label, prefix string
		end           int
	}{{"İtem", "i", 2}, {"Key", "k", 3}, {"中文", "中", 3}, {"true", "TR", 2}, {"true", "", 0}, {"true", "x", 0}} {
		if got := completionPrefixEnd(tt.label, tt.prefix); got != tt.end {
			t.Fatal(tt, got)
		}
	}
	v, _ := colorFixture(t)
	v.SetShowLineNumbers(false)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	v.SetSelection(textedit.Selection{Anchor: 5, Caret: 5})
	caret, ok := v.caretRect()
	if !ok {
		t.Fatal("missing caret")
	}
	c := CompletionContext{Range: textedit.Range{Start: 1, End: 5}}
	position := v.completionPosition(c, caret)
	if position != (geometry.Point{X: v.gutter + 4, Y: caret.Y + caret.Height + 2}) {
		t.Fatal("list text did not align with word start", position)
	}
	caret.Y += 20
	if p := v.completionPosition(c, caret); p.X != caret.X-12 {
		t.Fatal("wrapped start displaced popup", p)
	}
	v.offset.X = 30
	if p := v.completionPosition(c, caret); p.X != caret.X-12 {
		t.Fatal("hidden start displaced popup", p)
	}
	if v.Selection() != (textedit.Selection{Anchor: 5, Caret: 5}) {
		t.Fatal("anchoring changed selection")
	}
}
