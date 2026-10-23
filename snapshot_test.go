package sourceview

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

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
