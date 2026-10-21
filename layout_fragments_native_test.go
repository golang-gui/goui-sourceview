package sourceview

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

// Ordinary snapshots must retain native Unicode geometry without retaining a
// native layout. This is a metric/cache regression, not a claim that separately
// shaped substrings have inherited a paragraph's complete context.
func TestNativeFragmentsSurviveLayoutRelease(t *testing.T) {
	for _, sample := range []struct{ name, text string }{
		{"json", strings.Repeat(`{"name":"hello world","value":1234},`, 130)},
		{"unicode", strings.Repeat("中文 e\u0301 ffi 👩‍💻 العربية עברית\t ", 130)},
		{"override", "start \u202e" + strings.Repeat("abcd 1234 ", 410) + "XYZ\u202c end"},
		{"isolate", strings.Repeat("a \u2067עברית 123 العربية\u2069 b ", 150)},
	} {
		t.Run(sample.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ctx, err := nativeContext()
			if err != nil {
				t.Fatal(err)
			}
			defer ctx.Destroy()
			format := typography.TextFormat{Font: typography.FontInfo{Family: "DejaVu Sans", Size: 12}, WrapMode: typography.WrapWordChar, TabWidth: 40}
			layout, err := ctx.NewTextLayout(sample.text, format, 220, 1<<24)
			if err != nil {
				t.Fatal(err)
			}
			width, height := layout.MeasureSize()
			lines, clusters := copyMetrics(layout)
			f, err := textedit.NewFragment(textedit.Range{End: len(sample.text)}, geometry.Size{Width: width, Height: height}, lines, clusters)
			layout.Destroy()
			if err != nil {
				t.Fatal(err)
			}
			if f.LineCount() <= 64 {
				t.Fatalf("fixture did not cover pages: %d lines", f.LineCount())
			}
			got, want := f.Geometry(20), textedit.NewGeometry(lines, clusters, len(sample.text), 20)
			for i, line := range lines {
				for _, x := range []float32{-5, 0, 30, 100, 225} {
					point := geometry.Point{X: x, Y: line.Y + line.Height/2}
					if got.Hit(point) != want.Hit(point) {
						t.Fatalf("hit at line %d x=%g: %+v want %+v", i, x, got.Hit(point), want.Hit(point))
					}
				}
				for _, pos := range []textedit.Position{{Offset: line.Start}, {Offset: line.Start, Upstream: true}, {Offset: line.Start + line.Length, Upstream: true}} {
					if got.Caret(pos) != want.Caret(pos) || got.LineIndex(pos) != want.LineIndex(pos) {
						t.Fatalf("caret at %+v differs: %v want %v", pos, got.Caret(pos), want.Caret(pos))
					}
					for _, forward := range []bool{false, true} {
						if got.Snap(pos.Offset, forward) != want.Snap(pos.Offset, forward) || got.Adjacent(pos.Offset, forward) != want.Adjacent(pos.Offset, forward) {
							t.Fatalf("boundary at %+v forward=%v", pos, forward)
						}
					}
				}
			}
			for first := 0; first < len(lines); first += 31 {
				end := min(first+35, len(lines))
				rng := textedit.Range{Start: lines[first].Start, End: lines[end-1].Start + lines[end-1].Length}
				if !reflect.DeepEqual(got.SelectionLines(rng, first, end), want.SelectionLines(rng, first, end)) {
					t.Fatalf("selection [%d,%d) differs", first, end)
				}
			}
			// Keep the independent semantic assertion, not just path agreement.
			if sample.name == "override" {
				offset := strings.Index(sample.text, "XYZ")
				_, cs := f.Metrics()
				covered := 0
				for _, c := range cs {
					if c.Start >= offset && c.Start < offset+3 {
						if c.Direction != typography.TextRightToLeft {
							t.Fatal("RLO context lost in snapshot")
						}
						covered += c.Length
					}
				}
				if covered != 3 {
					t.Fatal("RLO target coverage", covered)
				}
			}
		})
	}
}

func TestSourceViewRetainsFragmentAfterNativeCacheRelease(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	v := NewSourceView()
	v.context = ctx
	defer v.dropLayouts()
	v.Document().Model().SetText(strings.Repeat("one two three four five ", 300))
	v.SetShowLineNumbers(false)
	v.SetWrapMode(WrapWordChar)
	v.LayoutVisible(geometry.Size{Width: 220, Height: 240}, geometry.Point{})
	p := v.paragraph(0)
	if p == nil || p.fragment.LineCount() < 64 {
		t.Fatal("missing measured fragment")
	}
	f := p.fragment
	want := p.geometry.Caret(textedit.Position{Offset: 500})
	v.dropLayouts()
	g := f.Geometry(v.lineHeight)
	if got := g.Caret(textedit.Position{Offset: 500}); got != want {
		t.Fatal("native cache release lost fragment values", got, want)
	}
}

// A copied window owns its values but cannot manufacture the context omitted
// from its input. Keep this explicit guard against enabling independent-window
// shaping as a universally correct editor path.
func TestFragmentSnapshotDoesNotInventMissingParagraphContext(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, err := nativeContext()
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Destroy()
	text := "start \u202e" + strings.Repeat("abcd 1234 ", 410) + "XYZ\u202c end"
	format := typography.TextFormat{Font: typography.FontInfo{Family: "DejaVu Sans", Size: 12}, WrapMode: typography.WrapWordChar}
	whole, err := ctx.NewTextLayout(text, format, 220, 1<<24)
	if err != nil {
		t.Fatal(err)
	}
	_, full := whole.MeasureMetrics()
	whole.Destroy()
	at := strings.Index(text, "XYZ")
	window, err := ctx.NewTextLayout(text[at:], format, 220, 1<<24)
	if err != nil {
		t.Fatal(err)
	}
	w, h := window.MeasureSize()
	ls, cs := window.MeasureMetrics()
	f, err := textedit.NewFragment(textedit.Range{Start: at, End: len(text)}, geometry.Size{Width: w, Height: h}, ls, cs)
	window.Destroy()
	if err != nil {
		t.Fatal(err)
	}
	_, partial := f.Metrics()
	seenFull, seenPartial := false, false
	for _, c := range full {
		if c.Start == at {
			seenFull = true
			if c.Direction != typography.TextRightToLeft {
				t.Fatal("full paragraph lost RLO")
			}
		}
	}
	for _, c := range partial {
		if c.Start == 0 {
			seenPartial = true
			if c.Direction != typography.TextLeftToRight {
				t.Fatal("fixture no longer demonstrates missing RLO context")
			}
		}
	}
	if !seenFull || !seenPartial {
		t.Fatal("missing fixture target")
	}
}
