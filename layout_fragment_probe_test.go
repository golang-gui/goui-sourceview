package sourceview

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
)

// This is a finite-input experiment, not an enabled control path. The caller
// must establish context equivalence before publishing independently measured
// windows. The long-RLO guard in layout_fragments_native_test.go demonstrates
// why successful ASCII samples alone cannot establish that equivalence.
type boundedFragmentProbe struct {
	pieces         []textedit.Fragment
	maxInput       int
	first, longest time.Duration
}

func probeBoundedFragments(ctx typography.Context, text string, format typography.TextFormat, width float32, limit int) (boundedFragmentProbe, error) {
	var result boundedFragmentProbe
	for start := 0; start < len(text); {
		end := min(len(text), start+limit)
		for end > start && end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		if end == start {
			return result, fmt.Errorf("probe: input limit cannot contain next rune")
		}
		began := time.Now()
		native, err := ctx.NewTextLayout(text[start:end], format, width, 1<<24)
		if err != nil {
			return result, err
		}
		w, h := native.MeasureSize()
		lines, clusters := native.MeasureMetrics()
		fragment, err := textedit.NewFragment(textedit.Range{Start: start, End: end}, geometry.Size{Width: w, Height: h}, lines, clusters)
		native.Destroy()
		if err != nil {
			return result, err
		}
		if end < len(text) {
			if len(lines) < 2 {
				return result, fmt.Errorf("probe: no complete line within bounded input")
			}
			fragment, err = fragment.LineRange(0, len(lines)-1)
			if err != nil {
				return result, err
			}
		}
		if fragment.Range().End <= start {
			return result, fmt.Errorf("probe: no progress")
		}
		result.pieces = append(result.pieces, fragment)
		result.maxInput = max(result.maxInput, end-start)
		elapsed := time.Since(began)
		if len(result.pieces) == 1 {
			result.first = elapsed
		}
		result.longest = max(result.longest, elapsed)
		start = fragment.Range().End // preserve the incomplete tail for the next step
	}
	return result, nil
}

func TestBoundedFragmentProbeMatchesSimpleParagraphs(t *testing.T) {
	for _, sample := range []struct{ name, text string }{
		{"word", strings.Repeat("a", 8192)},
		{"json", strings.Repeat(`{"title":"hello world","value":1234},`, 150)},
	} {
		t.Run(sample.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ctx, err := nativeContext()
			if err != nil {
				t.Fatal(err)
			}
			defer ctx.Destroy()
			format := typography.TextFormat{Font: typography.FontInfo{Family: "monospace", Size: 12}, WrapMode: typography.WrapWordChar, TabWidth: 40}
			whole, err := ctx.NewTextLayout(sample.text, format, 220, 1<<24)
			if err != nil {
				t.Fatal(err)
			}
			wantLines, wantClusters := copyMetrics(whole)
			whole.Destroy()
			for _, limit := range []int{512, 2048} {
				result, err := probeBoundedFragments(ctx, sample.text, format, 220, limit)
				if err != nil {
					t.Fatal(err)
				}
				if result.maxInput > limit || len(result.pieces) < 2 {
					t.Fatal("input was not bounded", result.maxInput, len(result.pieces))
				}
				lineAt, clusterAt := 0, 0
				y := float64(0)
				for _, piece := range result.pieces {
					lines, clusters := piece.Metrics()
					firstLine := lineAt
					for _, line := range lines {
						if lineAt >= len(wantLines) {
							t.Fatal("extra fragment line")
						}
						want := wantLines[lineAt]
						if line.Start+piece.Range().Start != want.Start || line.Length != want.Length || line.X != want.X || line.Width != want.Width || line.Height != want.Height || float32(float64(line.Y)+y) != want.Y || float32(float64(line.Baseline)+y) != want.Baseline {
							t.Fatalf("limit %d line %d: %+v y=%g want %+v", limit, lineAt, line, y, want)
						}
						lineAt++
					}
					for _, cluster := range clusters {
						if clusterAt >= len(wantClusters) {
							t.Fatal("extra fragment cluster")
						}
						want := wantClusters[clusterAt]
						if cluster.Start+piece.Range().Start != want.Start || cluster.Length != want.Length || cluster.X != want.X || cluster.Width != want.Width || cluster.Height != want.Height || cluster.Direction != want.Direction || cluster.LineIndex+firstLine != want.LineIndex || float32(float64(cluster.Y)+y) != want.Y {
							t.Fatalf("limit %d cluster %d differs: %+v y=%g want %+v", limit, clusterAt, cluster, y, want)
						}
						clusterAt++
					}
					y += float64(piece.Size().Height)
				}
				if lineAt != len(wantLines) || clusterAt != len(wantClusters) {
					t.Fatalf("incomplete output: %d/%d lines %d/%d clusters", lineAt, len(wantLines), clusterAt, len(wantClusters))
				}
			}
		})
	}
}

// Native timing probe only. Windows are not a general Unicode layout algorithm.
// No event-loop waits or painting are included; max-step-ms includes creation,
// measurement, metric export, snapshot construction and native destruction.
func BenchmarkBoundedFragmentProbe(b *testing.B) {
	text := strings.Repeat("a", 32768)
	for _, limit := range []int{0, 512, 2048} {
		b.Run(fmt.Sprintf("input-%d", limit), func(b *testing.B) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ctx, err := nativeContext()
			if err != nil {
				b.Fatal(err)
			}
			defer ctx.Destroy()
			format := typography.TextFormat{Font: typography.FontInfo{Family: "DejaVu Sans", Size: 12}, WrapMode: typography.WrapWordChar}
			warm, err := ctx.NewTextLayout("warm", format, 520, 1<<24)
			if err != nil {
				b.Fatal(err)
			}
			warm.MeasureMetrics()
			warm.Destroy()
			b.ResetTimer()
			var longest, first time.Duration
			for i := 0; i < b.N; i++ {
				if limit == 0 {
					start := time.Now()
					native, err := ctx.NewTextLayout(text, format, 520, 1<<24)
					if err != nil {
						b.Fatal(err)
					}
					w, h := native.MeasureSize()
					lines, clusters := native.MeasureMetrics()
					_, err = textedit.NewFragment(textedit.Range{End: len(text)}, geometry.Size{Width: w, Height: h}, lines, clusters)
					native.Destroy()
					if err != nil {
						b.Fatal(err)
					}
					elapsed := time.Since(start)
					first, longest = max(first, elapsed), max(longest, elapsed)
				} else {
					result, err := probeBoundedFragments(ctx, text, format, 520, limit)
					if err != nil {
						b.Fatal(err)
					}
					first, longest = max(first, result.first), max(longest, result.longest)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(first)/float64(time.Millisecond), "first-step-ms")
			b.ReportMetric(float64(longest)/float64(time.Millisecond), "max-step-ms")
		})
	}
}
