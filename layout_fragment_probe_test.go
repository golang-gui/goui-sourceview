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
	pieces         []probeFragment
	maxInput       int
	first, longest time.Duration
}

// 片段的占用范围不包含下一行的间距。单独保存由重叠行基线确定的放置坐标。
type probeFragment struct {
	textedit.Fragment
	y float64
}

func probeBoundedFragments(ctx typography.Context, text string, format typography.TextFormat, width float32, limit int) (boundedFragmentProbe, error) {
	return probeBoundedFragmentsWithPrefix(ctx, text, format, width, limit, nil)
}

func probeBoundedFragmentsWithPrefix(ctx typography.Context, text string, format typography.TextFormat, width float32, limit int, prefixAt func(int) (string, error)) (boundedFragmentProbe, error) {
	var result boundedFragmentProbe
	var nextBaseline float64
	for start := 0; start < len(text); {
		began := time.Now()
		prefix := ""
		if prefixAt != nil {
			var err error
			prefix, err = prefixAt(start)
			if err != nil {
				return result, err
			}
		}
		if len(prefix) >= limit {
			return result, fmt.Errorf("probe: context consumes input limit")
		}
		end := min(len(text), start+limit-len(prefix))
		for end > start && end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		if end == start {
			return result, fmt.Errorf("probe: input limit cannot contain next rune")
		}
		native, err := ctx.NewTextLayout(prefix+text[start:end], format, width, 1<<24)
		if err != nil {
			return result, err
		}
		w, h := native.MeasureSize()
		lines, clusters := native.MeasureMetrics()
		lines, clusters, err = removeProbePrefix(lines, clusters, len(prefix))
		if err != nil {
			native.Destroy()
			return result, err
		}
		fragment, err := textedit.NewFragment(textedit.Range{Start: start, End: end}, geometry.Size{Width: w, Height: h}, lines, clusters)
		native.Destroy()
		if err != nil {
			return result, err
		}
		if end < len(text) {
			if len(lines) < 3 {
				return result, fmt.Errorf("probe: no complete line within bounded input")
			}
			// 保留一条完整重叠行，再排除可能被输入上限截断的末行。
			fragment, err = fragment.LineRange(0, len(lines)-2)
			if err != nil {
				return result, err
			}
		}
		if fragment.Range().End <= start {
			return result, fmt.Errorf("probe: no progress")
		}
		y := float64(0)
		if len(result.pieces) > 0 {
			y = nextBaseline - float64(lines[0].Baseline)
		}
		result.pieces = append(result.pieces, probeFragment{Fragment: fragment, y: y})
		if end < len(text) {
			nextBaseline = y + float64(lines[len(lines)-2].Baseline)
		}
		result.maxInput = max(result.maxInput, len(prefix)+end-start)
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
				for _, piece := range result.pieces {
					y := piece.y
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
				}
				if lineAt != len(wantLines) || clusterAt != len(wantClusters) {
					t.Fatalf("incomplete output: %d/%d lines %d/%d clusters", lineAt, len(wantLines), clusterAt, len(wantClusters))
				}
			}
		})
	}
}

// 每行四字节、字形高 12、行距 15、首行顶部 2.75。占用高度与行推进不同，
// 不依赖宿主字体或原生库也能复现按片段 Size.Height 累加导致的位置漂移。
type probeGapContext struct{ typography.Context }
type probeGapLayout struct {
	typography.TextLayout
	text string
}

func (probeGapContext) NewTextLayout(text string, _ typography.TextFormat, _, _ float32) (typography.TextLayout, error) {
	return &probeGapLayout{text: text}, nil
}
func (p *probeGapLayout) Destroy() {}
func (p *probeGapLayout) MeasureSize() (float32, float32) {
	return 20, float32((len(p.text)+3)/4-1)*15 + 14.75
}
func (p *probeGapLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	var lines []typography.TextLine
	var clusters []typography.TextCluster
	for start := 0; start < len(p.text); start += 4 {
		index := len(lines)
		y, baseline := float32(index*15)+2.75, float32(index*15)+12
		length := min(4, len(p.text)-start)
		lines = append(lines, typography.TextLine{Start: start, Length: length, Y: y, Width: float32(length * 5), Height: 12, Baseline: baseline})
		for i := range length {
			clusters = append(clusters, typography.TextCluster{Start: start + i, Length: 1, X: float32(i * 5), Y: y, Width: 5, Height: 12, LineIndex: index})
		}
	}
	return lines, clusters
}

func TestBoundedProbeKeepsInterlineGaps(t *testing.T) {
	result, err := probeBoundedFragments(probeGapContext{}, strings.Repeat("a", 80), typography.TextFormat{}, 20, 32)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.pieces) < 2 || result.maxInput > 32 {
		t.Fatal("未保留有界的独立输入", len(result.pieces), result.maxInput)
	}
	next := 0
	for _, piece := range result.pieces {
		if piece.Range().Start != next {
			t.Fatal("片段遗漏或重复源文本", piece.Range(), next)
		}
		lines, clusters := piece.Metrics()
		for _, line := range lines {
			index := (piece.Range().Start + line.Start) / 4
			if float32(float64(line.Y)+piece.y) != float32(index*15)+2.75 || float32(float64(line.Baseline)+piece.y) != float32(index*15)+12 {
				t.Fatalf("行距丢失：byte=%d y=%g baseline=%g", piece.Range().Start+line.Start, float64(line.Y)+piece.y, float64(line.Baseline)+piece.y)
			}
		}
		for _, cluster := range clusters {
			index := (piece.Range().Start + cluster.Start) / 4
			if float32(float64(cluster.Y)+piece.y) != float32(index*15)+2.75 {
				t.Fatal("簇未使用与行相同的放置坐标", cluster)
			}
		}
		next = piece.Range().End
	}
	if next != 80 {
		t.Fatal("未覆盖完整文本", next)
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
