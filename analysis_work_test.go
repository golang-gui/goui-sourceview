package sourceview

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

func TestAnalysisOnlyInvalidatesVisibleTokenChanges(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText(strings.Repeat("<p>value</p>\n", 1000))
	if err := v.Document().SetLanguage(language.XML); err != nil {
		t.Fatal(err)
	}
	v.SetFoldingEnabled(false)
	observe(t, v)
	v.Document().engine.Advance(time.Time{})
	ctx := &shapedContext{}
	v.context, v.frameLayout = ctx, true
	t.Cleanup(func() {
		v.dropLayouts()
		if ctx.created != ctx.destroyed {
			t.Fatal("analysis view leaked native layouts", ctx.created, ctx.destroyed)
		}
	})
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	v.layoutStep(time.Time{})
	created := ctx.created
	if v.analysisPaintNeeded() {
		t.Fatal("unchanged published colors request another paint")
	}
	edit := func(row int) {
		r, _ := v.Document().Model().LineRange(row)
		if err := v.Document().Model().ReplaceAtomic(textedit.Range{Start: r.Start + 3, End: r.Start + 8}, "other"); err != nil {
			t.Fatal(err)
		}
	}
	q := &frameQueue{views: []*SourceView{v}}
	edit(900)
	before := v.Document().engine.ScannedLines()
	if !q.advanceAnalysis(time.Now().Add(-time.Second)) || v.Document().engine.ScannedLines() != before {
		t.Fatal("expired analysis batch performed scanning")
	}
	if q.advanceAnalysis(time.Time{}) || v.analysisPaintNeeded() || ctx.created != created {
		t.Fatal("offscreen analysis repainted or shaped the viewport")
	}
	edit(3)
	q.advanceAnalysis(time.Time{})
	if !v.analysisPaintNeeded() || ctx.created != created {
		t.Fatal("visible token changes were missed or measured by pure analysis")
	}
	v.layoutStep(time.Time{}) // Only frame work may prepare changed text/geometry.
	v.LayoutVisible(v.viewport, v.offset)
	v.layoutStep(time.Time{})
	if v.analysisPaintNeeded() {
		t.Fatal("frame did not publish current visible colors")
	}
}

type analysisRoot struct {
	gui.Root
	paints int
}

func (r *analysisRoot) RequestPaint() error { r.paints++; return nil }

func TestAnalysisTimerFailureKeepsFrameFallback(t *testing.T) {
	root := &analysisRoot{}
	q := &frameQueue{root: root, analysisTimer: &gui.Timer{}, analysisHandle: signal.Handles(nil)}
	q.requestAnalysis()
	if q.analysisTimer != nil || root.paints != 1 {
		t.Fatal("unavailable timer left pending work without a frame", q.analysisTimer, root.paints)
	}
}
