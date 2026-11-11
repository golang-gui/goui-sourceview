package view

import (
	"strings"
	"testing"
	"unicode/utf8"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

// Fixed 10 DIP rune advances and 20 DIP lines give an independent size oracle.
// Embedding the native contract makes unexpected calls fail rather than silently
// accepting operations that the paragraph tests did not arrange.
type paragraphFixtureContext struct{ typography.Context }

type paragraphFixtureLayout struct {
	typography.TextLayout
	lines         []typography.TextLine
	clusters      []typography.TextCluster
	width, height float32
	destroyed     bool
}

func (*paragraphFixtureContext) NewTextLayout(text string, format typography.TextFormat, width, _ float32) (typography.TextLayout, error) {
	p := &paragraphFixtureLayout{}
	line := typography.TextLine{Height: 20, Baseline: 15}
	x, y := float32(0), float32(0)
	for at, r := range text {
		if x+10 > width && x > 0 && format.WrapMode != typography.WrapNone {
			line.Length, line.Width = at-line.Start, x
			p.lines = append(p.lines, line)
			x, y = 0, y+20
			line = typography.TextLine{Start: at, Y: y, Height: 20, Baseline: y + 15}
		}
		p.clusters = append(p.clusters, typography.TextCluster{Start: at, Length: utf8.RuneLen(r), X: x, Y: y, Width: 10, Height: 20, LineIndex: len(p.lines)})
		x += 10
		p.width = max(p.width, x)
	}
	line.Length, line.Width = len(text)-line.Start, x
	p.lines = append(p.lines, line)
	p.height = y + 20
	return p, nil
}

func (p *paragraphFixtureLayout) MeasureSize() (float32, float32) { return p.width, p.height }
func (p *paragraphFixtureLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	return p.lines, p.clusters
}
func (p *paragraphFixtureLayout) Destroy() { p.destroyed = true }

func TestParagraphLayoutViewIsolationAndRelease(t *testing.T) {
	const size = 40 * paragraphBlockBytes
	text := strings.Repeat("a", size)
	read := func(r textcore.Range) (string, error) { return text[r.Start:r.End], nil }
	format := typography.TextFormat{WrapMode: typography.WrapWordChar}
	a := NewParagraphLayout(&paragraphFixtureContext{}, format, 100, size, read)
	b := NewParagraphLayout(&paragraphFixtureContext{}, format, 200, size, read)
	t.Cleanup(a.Destroy)
	t.Cleanup(b.Destroy)
	viewport := geometry.Rect(0, 0, 100, 100)
	if !a.Prepare(viewport, true) || !b.Prepare(viewport, true) {
		t.Fatal("initial pages unavailable", a.Err(), b.Err())
	}
	first, second := a.pages[0].layout.(*paragraphFixtureLayout), b.pages[0].layout.(*paragraphFixtureLayout)
	if first == second || len(first.lines) <= len(second.lines) {
		t.Fatal("views shared mutable layout or lost their widths")
	}
	a.Destroy()
	if !first.destroyed || second.destroyed || b.CachedGeometryAt(0) == nil {
		t.Fatal("destroying one view affected the other")
	}
	b.SetWidth(80)
	if !second.destroyed || b.CachedGeometryAt(0) != nil {
		t.Fatal("width change retained stale native resources")
	}
	if !b.Prepare(viewport, true) {
		t.Fatal("resized view unavailable", b.Err())
	}
	resized := b.pages[0].layout.(*paragraphFixtureLayout)
	b.Change(size+1, []textcore.Edit{{Range: textcore.Range{}, Text: "x"}})
	if !resized.destroyed || b.CachedGeometryAt(0) != nil {
		t.Fatal("edit retained stale native resources")
	}
}
