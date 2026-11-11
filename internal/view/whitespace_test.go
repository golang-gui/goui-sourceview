package view

import (
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

func TestWhitespaceModesAndSpecialSpaces(t *testing.T) {
	text := "  a b  c\t\u00a0\u202f\u3000 \t"
	w := NewWhitespaceLayout(text, nil, nil)
	for w.Pending(true) {
		w.Advance(time.Time{}, true)
	}
	want := map[WhitespaceMode][]int{
		WhitespaceNone: {}, WhitespaceAll: {0, 1, 3, 5, 6, 8, 9, 11, 14, 17, 18},
		WhitespaceBoundary: {0, 1, 5, 6, 8, 9, 11, 14, 17, 18},
		WhitespaceTrailing: {8, 9, 11, 14, 17, 18}, WhitespaceSelection: {5, 6},
	}
	for mode, expected := range want {
		var got []int
		for at, r := range text {
			kind := WhitespaceKind(r)
			if kind != 0 && w.Show(mode, at, at+len(string(r)), kind, at >= 5 && at < 7) {
				got = append(got, at)
			}
		}
		if !slices.Equal(got, expected) {
			t.Fatalf("mode %d: %v, want %v", mode, got, expected)
		}
	}
	blank := NewWhitespaceLayout(" \t\u00a0\u3000", nil, nil)
	blank.Advance(time.Time{}, true)
	if blank.Trailing != 0 || blank.Leading != len(blank.Text) {
		t.Fatal("all-blank logical line", blank)
	}
}

func TestWhitespaceIndexBudgetAndViewport(t *testing.T) {
	const count = 131072
	clusters := make([]typography.TextCluster, count)
	for i := range clusters {
		clusters[i] = typography.TextCluster{Start: i, Length: 1, X: float32(i) * 10, Width: 10}
	}
	w := NewWhitespaceLayout(strings.Repeat(" ", count), []typography.TextLine{{Height: 20}}, clusters)
	w.Advance(time.Now().Add(-time.Second), true)
	if len(w.nodes) != 0 || w.Leading != 0 {
		t.Fatal("expired deadline did work")
	}
	w.Advance(time.Time{}, true)
	if !w.Pending(true) {
		t.Fatal("one step consumed the full input")
	}
	steps := 1
	for w.Pending(true) {
		w.Advance(time.Time{}, true)
		steps++
	}
	if steps < 2 || w.Leading != count || w.Trailing != 0 {
		t.Fatal("unfinished boundaries", steps, w.Leading, w.Trailing)
	}
	marked := 0
	examined := w.Visit(geometry.Rect(600000, 0, 100, 20), func(c typography.TextCluster, r geometry.Rectangle) {
		marked++
		if c.Start < 60000 || c.Start >= 60010 {
			t.Fatal("offscreen marker", c)
		}
	})
	if marked != 10 || examined > 2*whitespaceClusterBlock {
		t.Fatal("horizontal culling", marked, examined)
	}
	marked = 0
	if n := w.Visit(geometry.Rect(0, 40, 100, 20), func(typography.TextCluster, geometry.Rectangle) { marked++ }); n != 0 || marked != 0 {
		t.Fatal("vertical culling", n, marked)
	}
	// 第二屏位于前面的逻辑块：索引不能假定字节顺序与视觉坐标顺序相同。
	clusters[0].X = 600010
	w = NewWhitespaceLayout(strings.Repeat(" ", count), []typography.TextLine{{Height: 20}}, clusters)
	for w.Pending(false) {
		w.Advance(time.Time{}, false)
	}
	marked = 0
	w.Visit(geometry.Rect(600000, 0, 100, 20), func(typography.TextCluster, geometry.Rectangle) { marked++ })
	if marked != 11 {
		t.Fatal("unordered visual spans", marked)
	}
}

func TestWhitespaceNativeMetrics(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	for _, text := range []string{"  a b  c\t\u00a0\u202f\u3000  ", "אבג \tד ", strings.Repeat("abc ", 80)} {
		for _, width := range []float32{85, 500} {
			native, err := ctx.NewTextLayout(text, typography.TextFormat{Font: typography.FontInfo{Family: "sans", Size: 12}, WrapMode: typography.WrapWordChar, TabWidth: 48}, width, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			lines, clusters := native.MeasureMetrics()
			w := NewWhitespaceLayout(text, lines, clusters)
			for w.Pending(true) {
				w.Advance(time.Time{}, true)
			}
			g := textcore.NewGeometry(lines, clusters, len(text), 20)
			seen := 0
			w.Visit(geometry.Rect(-10000, -10000, 20000, 20000), func(c typography.TextCluster, r geometry.Rectangle) {
				if WhitespaceKind([]rune(text[c.Start : c.Start+c.Length])[0]) == 0 {
					return
				}
				seen++
				rects := g.Selection(textcore.Range{Start: c.Start, End: c.Start + c.Length})
				if len(rects) != 1 || rects[0] != r {
					t.Fatalf("native blank geometry %q: %v vs %v", text[c.Start:c.Start+c.Length], r, rects)
				}
			})
			if seen == 0 || !w.EndReady {
				t.Fatal("missing native whitespace or end caret")
			}
			native.Destroy()
		}
	}
}

func TestWhitespaceFragmentUsesLogicalBoundaries(t *testing.T) {
	text := "\t \t" + strings.Repeat(" ", 20000) + "a b  c" + strings.Repeat(" ", 20000)
	check := func(start, end int) {
		w := NewWhitespaceLayout(text[start:end], nil, nil)
		w.TextStart, w.Length, w.Trailing = start, len(text), len(text)
		w.Read = func(at int, back bool) string {
			if back {
				return text[max(0, at-512):at]
			}
			return text[at:min(len(text), at+512)]
		}
		for w.Pending(true) {
			w.Advance(time.Time{}, true)
		}
		if w.Leading != 20003 || w.Trailing != 20009 {
			t.Fatal("fragment lost logical boundaries", w.Leading, w.Trailing)
		}
		for at, r := range w.Text {
			k := WhitespaceKind(r)
			if k == 0 {
				continue
			}
			global := start + at
			wantTrailing := global >= 20009
			wantBoundary := k != 1 || global < 20003 || wantTrailing || global > 0 && text[global-1] == ' ' || global+1 < len(text) && text[global+1] == ' '
			if w.Show(WhitespaceTrailing, at, at+1, k, false) != wantTrailing || w.Show(WhitespaceBoundary, at, at+1, k, false) != wantBoundary {
				t.Fatal("fragment boundary", global)
			}
		}
	}
	check(1, 2)
	check(20002, 20008)
	check(20005, 20006)
	check(20008, 20012)
	check(25000, 25020)
}
