package sourceview

import (
	"fmt"
	"image/color"
	"runtime"

	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/style"
)

func (v *SourceView) partColor(part string, fallback color.NRGBA) color.NRGBA {
	if v.explicitScheme != nil {
		return fallback
	}
	name := v.StyleName()
	if name == "" {
		name = "source-view"
	}
	s := gui.ResolveWidgetStyle(v, name, part, style.Normal)
	if c, ok := s.ForegroundColor(); ok {
		return nrgba(c)
	}
	return fallback
}
func (v *SourceView) foldButtonRect(row int) geometry.Rectangle {
	return geometry.Rect(v.gutter-v.foldColumn, padding+v.paragraphTop(row)-v.offset.Y, v.foldColumn, v.lineHeight)
}
func (v *SourceView) foldBadgeRect(row int, p *paragraph) geometry.Rectangle {
	if i := v.foldHeader(row); i < 0 || !v.folds[i].Collapsed || v.projection != nil {
		return geometry.Rectangle{}
	}
	edge := p.geometry.Caret(textedit.Position{Offset: len(p.text), Upstream: true})
	r := geometry.Rect(v.gutter+padding-v.offset.X+edge.X+5, padding+v.paragraphTop(row)-v.offset.Y+edge.Y, 16, edge.Height)
	if v.showLineEndings {
		r.X += 12
	}
	if v.wrap != WrapNone && r.X+r.Width > v.viewport.Width-padding {
		return geometry.Rectangle{}
	}
	return r
}
func (v *SourceView) paintFoldButton(p gui.Painter, row int) {
	i := v.foldHeader(row)
	if v.foldColumn == 0 || i < 0 || v.projection != nil {
		return
	}
	r := v.foldButtonRect(row)
	x, y := r.X+r.Width/2, r.Y+r.Height/2
	c := graphics.ColorOf(v.partColor("fold-toggle", v.colors.GutterForeground))
	if v.folds[i].Collapsed {
		p.DrawLine(geometry.Point{X: x - 2, Y: y - 4}, geometry.Point{X: x + 2, Y: y}, 1, c)
		p.DrawLine(geometry.Point{X: x + 2, Y: y}, geometry.Point{X: x - 2, Y: y + 4}, 1, c)
	} else {
		p.DrawLine(geometry.Point{X: x - 4, Y: y - 2}, geometry.Point{X: x, Y: y + 2}, 1, c)
		p.DrawLine(geometry.Point{X: x, Y: y + 2}, geometry.Point{X: x + 4, Y: y - 2}, 1, c)
	}
}
func (v *SourceView) paintFoldBadge(p gui.Painter, row int, item *paragraph) {
	r := v.foldBadgeRect(row, item)
	if r.Width == 0 {
		return
	}
	if s := v.Selection().Range(); s.Start <= v.folds[v.foldHeader(row)].start && s.End > v.folds[v.foldHeader(row)].end {
		p.FillRect(r, graphics.ColorOf(v.colors.SelectionBackground))
	}
	c := graphics.ColorOf(v.partColor("fold-placeholder", v.colors.GutterForeground))
	for n := range 3 {
		p.FillRect(geometry.Rect(r.X+float32(n)*4+2, r.Y+r.Height*.62, 2, 2), c)
	}
}
func (v *SourceView) foldHit(point geometry.Point) (int, bool) {
	if !v.canFold() || v.foldColumn == 0 {
		return 0, false
	}
	row := v.paragraphAt(max(0, point.Y+v.offset.Y-padding))
	if v.foldHeader(row) < 0 {
		return 0, false
	}
	if containsPoint(v.foldButtonRect(row), point) {
		return row, true
	}
	if p := v.paragraph(row); p != nil && containsPoint(v.foldBadgeRect(row, p), point) {
		return row, true
	}
	return 0, false
}
func (v *SourceView) foldAtCaret(collapsed bool) bool {
	line := v.document.model.LineAt(v.Selection().Caret)
	for i := len(v.folds) - 1; i >= 0; i-- {
		r := v.folds[i]
		if line >= r.StartLine && line <= r.EndLine && r.Collapsed != collapsed {
			return v.setFolded(r.StartLine, collapsed)
		}
	}
	return false
}
func (v *SourceView) foldingKey(e events.KeyEvent) bool {
	if e.Key != events.KeyBracketLeft && e.Key != events.KeyBracketRight {
		return false
	}
	mods, _ := (gui.ModPrimary | gui.ModShift).Resolve()
	if runtime.GOOS == "darwin" {
		mods, _ = (gui.ModPrimary | gui.ModAlt).Resolve()
	}
	if e.Modifiers != mods {
		return false
	}
	v.foldAtCaret(e.Key == events.KeyBracketLeft)
	return true
}
func (v *SourceView) snapshotFolding(info *gui.WidgetInfo, first, last int) {
	meta := info.Attributes["goui.sourceview"].(map[string]any)
	meta["foldingEnabled"] = v.foldingEnabled
	meta["foldingPending"] = v.document.definition.Folding() != nil && (!v.document.foldReady || v.document.folding != nil && v.document.folding.busy)
	meta["foldRegionCount"] = len(v.folds)
	if v.projection != nil {
		if len(v.hiddenLines) > 0 {
			at := v.projection.Replacement().Start
			info.TextEditing.VisibleRange = textedit.Range{Start: at, End: at}
			info.TextEditing.VisibleText = ""
		}
		return
	}
	budget := 64 * 1024
	var spans []map[string]any
	for row := first; row <= last && budget > 0 && len(spans) < 256; row = v.nextDisplayRow(row) {
		r, _ := v.document.model.LineRange(row)
		r.End = min(r.End, r.Start+budget)
		for r.End > r.Start && !v.document.model.IsRuneBoundary(r.End) {
			r.End--
		}
		text, _ := v.document.model.Slice(r)
		budget -= len(text)
		spans = append(spans, map[string]any{"range": r, "text": text})
		if len(v.hiddenLines) > 0 && len(spans) == 1 {
			info.TextEditing.VisibleRange = r
			info.TextEditing.VisibleText = text
		}
		if i := v.foldHeader(row); i >= 0 && v.foldColumn > 0 {
			label := resource.Fold()
			if v.folds[i].Collapsed {
				label = resource.Unfold()
			}
			r := v.foldButtonRect(row).Translate(geometry.Point{X: info.Bounds.X, Y: info.Bounds.Y})
			child := gui.WidgetInfo{ID: fmt.Sprintf("%s.fold.%d", info.ID, row), Role: gui.RoleButton, Name: label, Bounds: r, Visible: true, Enabled: gui.IsEnabled(v), Actions: []gui.Action{gui.ActionClick}}
			child.SetAttribute("goui.sourceview.fold", map[string]any{"startLine": row, "endLine": v.folds[i].EndLine, "collapsed": v.folds[i].Collapsed})
			info.Children = append(info.Children, child)
		}
	}
	if len(v.hiddenLines) > 0 {
		meta["visibleSpans"] = spans
	}
}
