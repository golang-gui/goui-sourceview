// 分段上下文实验及反例；默认测试继续验证有限上下文方案的适用边界。
package sourceview

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-gui/goui/platform/typography"
	"golang.org/x/text/unicode/bidi"
)

// This is deliberately a test-only hypothesis: replaying explicit controls is
// not a Unicode bidi resolver. In particular it cannot resolve arbitrarily
// distant neutral characters, paired brackets, or first-strong isolates. Do
// not publish its results in a control based on the successful fixtures below.
type explicitContextProbe struct {
	text     string
	at       int
	base     rune
	controls []rune
}

func newExplicitContextProbe(text string) *explicitContextProbe {
	p := &explicitContextProbe{text: text, base: '\u200e'}
	isolateDepth := 0
	for _, r := range text {
		prop, _ := bidi.LookupRune(r)
		switch prop.Class() {
		case bidi.LRI, bidi.RLI, bidi.FSI:
			isolateDepth++
		case bidi.PDI:
			isolateDepth = max(0, isolateDepth-1)
		case bidi.L, bidi.R, bidi.AL:
			if isolateDepth == 0 {
				if prop.Class() != bidi.L {
					p.base = '\u200f'
				}
				return p
			}
		}
	}
	return p
}

func (p *explicitContextProbe) prefix(at int) (string, error) {
	if at < p.at || at > len(p.text) {
		return "", fmt.Errorf("context probe: nonmonotonic input")
	}
	for _, r := range p.text[p.at:at] {
		switch r {
		case '\u202a', '\u202b', '\u202d', '\u202e', '\u2066', '\u2067':
			// Overflow counters are intentionally not implemented by this
			// hypothesis. Refuse instead of silently dropping a control.
			if len(p.controls) == 60 {
				return "", fmt.Errorf("context probe: explicit depth not supported")
			}
			p.controls = append(p.controls, r)
		case '\u2068':
			return "", fmt.Errorf("context probe: first-strong isolate not resolved")
		case '\u202c':
			if n := len(p.controls); n > 0 && p.controls[n-1] != '\u2066' && p.controls[n-1] != '\u2067' {
				p.controls = p.controls[:n-1]
			}
		case '\u2069':
			for i := len(p.controls) - 1; i >= 0; i-- {
				if p.controls[i] == '\u2066' || p.controls[i] == '\u2067' {
					p.controls = p.controls[:i]
					break
				}
			}
		}
	}
	p.at = at
	return string(p.base) + string(p.controls), nil
}

// Map a synthetic, zero-width prefix out of native metrics. A prefix that
// acquires width, crosses a shaping cluster, or occupies a separate line is
// not safely removable. Do not discard genuine document controls/characters.
func removeProbePrefix(lines []typography.TextLine, clusters []typography.TextCluster, prefix int) ([]typography.TextLine, []typography.TextCluster, error) {
	if prefix == 0 {
		return lines, clusters, nil
	}
	if len(lines) == 0 || lines[0].Start != 0 || lines[0].Length < prefix || len(lines) > 1 && lines[1].Start <= prefix {
		return nil, nil, fmt.Errorf("context probe: prefix is not contained in first line")
	}
	mappedLines := make([]typography.TextLine, len(lines))
	for i, line := range lines {
		end := line.Start + line.Length
		line.Start = max(prefix, line.Start) - prefix
		line.Length = end - prefix - line.Start
		line.Clusters = nil
		mappedLines[i] = line
	}
	mappedClusters := make([]typography.TextCluster, 0, len(clusters))
	for _, cluster := range clusters {
		if cluster.Start < prefix {
			if cluster.Start+cluster.Length > prefix || cluster.Width != 0 {
				return nil, nil, fmt.Errorf("context probe: prefix changes a document cluster")
			}
			continue
		}
		cluster.Start -= prefix
		mappedClusters = append(mappedClusters, cluster)
	}
	return mappedLines, mappedClusters, nil
}

func fragmentProbeDifference(pieces []probeFragment, wantLines []typography.TextLine, wantClusters []typography.TextCluster) error {
	lineAt, clusterAt := 0, 0
	for _, piece := range pieces {
		y := piece.y
		lines, clusters := piece.Metrics()
		firstLine := lineAt
		for _, line := range lines {
			if lineAt >= len(wantLines) {
				return fmt.Errorf("extra fragment line")
			}
			line.Start += piece.Range().Start
			line.Y = float32(float64(line.Y) + y)
			line.Baseline = float32(float64(line.Baseline) + y)
			want := wantLines[lineAt]
			if line.Start != want.Start || line.Length != want.Length || line.X != want.X || line.Y != want.Y || line.Width != want.Width || line.Height != want.Height || line.Baseline != want.Baseline {
				return fmt.Errorf("line %d: %+v want %+v", lineAt, line, want)
			}
			lineAt++
		}
		for _, cluster := range clusters {
			if clusterAt >= len(wantClusters) {
				return fmt.Errorf("extra fragment cluster")
			}
			cluster.Start += piece.Range().Start
			cluster.Y = float32(float64(cluster.Y) + y)
			cluster.LineIndex += firstLine
			if cluster != wantClusters[clusterAt] {
				return fmt.Errorf("cluster %d: %+v want %+v", clusterAt, cluster, wantClusters[clusterAt])
			}
			clusterAt++
		}
	}
	if lineAt != len(wantLines) || clusterAt != len(wantClusters) {
		return fmt.Errorf("incomplete metrics: %d/%d lines, %d/%d clusters", lineAt, len(wantLines), clusterAt, len(wantClusters))
	}
	return nil
}

func TestExplicitContextFragmentProbe(t *testing.T) {
	for _, sample := range []struct{ name, text string }{
		{"longRLO", "start \u202e" + strings.Repeat("abcd 1234 ", 410) + "XYZ\u202c end"},
		{"longLRO", "אבג \u202d" + strings.Repeat("abcd 1234 ", 410) + "XYZ\u202c סוף"},
		{"longRLI", "start \u2067" + strings.Repeat("עברית العربية 123 ", 180) + "XYZ\u2069 end"},
		{"unicode", strings.Repeat("中文 e\u0301 ffi 👩‍💻 العربية עברית\t ", 130)},
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
			whole, err := ctx.NewTextLayout(sample.text, format, 220, 1<<24)
			if err != nil {
				t.Fatal(err)
			}
			ls, cs := copyMetrics(whole)
			whole.Destroy()
			for _, limit := range []int{512, 2048} {
				context := newExplicitContextProbe(sample.text)
				result, err := probeBoundedFragmentsWithPrefix(ctx, sample.text, format, 220, limit, context.prefix)
				if err != nil {
					t.Fatal(err)
				}
				if result.maxInput > limit || len(result.pieces) < 2 {
					t.Fatal("context replay exceeded native input limit or did not split")
				}
				if err := fragmentProbeDifference(result.pieces, ls, cs); err != nil {
					t.Fatalf("limit %d: %v", limit, err)
				}
			}
		})
	}
}

func TestExplicitContextProbeKeepsNestedScopes(t *testing.T) {
	text := "a \u202eX\u2066Y\u202cZ\u2069W\u202cQ"
	p := newExplicitContextProbe(text)
	for _, step := range []struct{ through, want string }{
		{"X", "\u200e\u202e"},
		{"Y", "\u200e\u202e\u2066"},
		{"Z", "\u200e\u202e\u2066"}, // PDF cannot pop across the isolate.
		{"W", "\u200e\u202e"},
		{"Q", "\u200e"},
	} {
		at := strings.Index(text, step.through) + len(step.through)
		got, err := p.prefix(at)
		if err != nil || got != step.want {
			t.Fatalf("after %s: %q / %v, want %q", step.through, got, err, step.want)
		}
	}
}

func TestProbePrefixMappingPreservesDocumentControls(t *testing.T) {
	// Three injected bytes precede a genuine RLO and X in the document. Only
	// the injection may disappear; both document clusters retain their bytes.
	ls := []typography.TextLine{{Length: 7, Width: 10, Height: 20, Baseline: 15}}
	cs := []typography.TextCluster{
		{Start: 0, Length: 3, Height: 20},
		{Start: 3, Length: 3, X: 10, Height: 20, Direction: typography.TextRightToLeft},
		{Start: 6, Length: 1, Width: 10, Height: 20, Direction: typography.TextRightToLeft},
	}
	lines, clusters, err := removeProbePrefix(ls, cs, 3)
	if err != nil || len(clusters) != 2 || clusters[0].Start != 0 || clusters[0].Length != 3 || clusters[1].Start != 3 || lines[0].Length != 4 {
		t.Fatal("prefix mapping lost document bytes", lines, clusters, err)
	}
	if ls[0].Length != 7 || cs[1].Start != 3 {
		t.Fatal("prefix mapping mutated its native input")
	}
	for _, bad := range []struct {
		name string
		cs   []typography.TextCluster
	}{
		{"width", []typography.TextCluster{{Length: 3, Width: 1}}},
		{"crossing", []typography.TextCluster{{Length: 4}}},
	} {
		t.Run(bad.name, func(t *testing.T) {
			if _, _, err := removeProbePrefix(ls, bad.cs, 3); err == nil {
				t.Fatal("accepted a prefix that changes document geometry")
			}
		})
	}
}

// Passing explicit-control samples must not conceal implicit bidi context.
// These fixtures intentionally require more than the active-control prefix.
func TestExplicitContextProbeDoesNotResolveImplicitContext(t *testing.T) {
	for _, sample := range []struct{ name, text string }{
		{"neutral", "start אבג " + strings.Repeat(" . ", 1500) + "ד end"},
		{"numbers", "start אבג " + strings.Repeat("1234 ", 900) + "tail"},
		{"brackets", "start אבג (" + strings.Repeat(" . ", 1500) + "ד) end"},
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
			whole, err := ctx.NewTextLayout(sample.text, format, 220, 1<<24)
			if err != nil {
				t.Fatal(err)
			}
			ls, cs := copyMetrics(whole)
			whole.Destroy()
			context := newExplicitContextProbe(sample.text)
			result, err := probeBoundedFragmentsWithPrefix(ctx, sample.text, format, 220, 512, context.prefix)
			if err != nil {
				t.Fatal(err)
			}
			if err := fragmentProbeDifference(result.pieces, ls, cs); err == nil {
				t.Fatal("fixture no longer exposes implicit context dependency")
			} else {
				t.Log(err)
			}
		})
	}
}
