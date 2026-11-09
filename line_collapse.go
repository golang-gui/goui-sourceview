package sourceview

import (
	"fmt"
	"slices"

	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/graphics"
)

const lineCollapseBadgeWidth float32 = 22

// ParagraphLayout sets explicit page clips; Painter.SetClipRect replaces its
// caller's explicit clip. Keep those page clips inside the preview using only
// the public Painter contract, without changing the paragraph's geometry.
type linePreviewPainter struct {
	gui.Painter
	bounds geometry.Rectangle
}

func (p *linePreviewPainter) SetClipRect(rect geometry.Rectangle) {
	p.Painter.SetClipRect(rect.Intersect(p.bounds))
}

// CollapsedLines returns the zero-based logical lines manually shortened in
// this view. Intent survives resizing and temporarily disabling wrapping.
// Editing a shortened line, resetting text or switching Document clears it.
func (v *SourceView) CollapsedLines() []int {
	if v.collapsedRevision != v.document.model.Revision() {
		return nil
	}
	lines := make([]int, 0, len(v.collapsedLines))
	for line := range v.collapsedLines {
		lines = append(lines, line)
	}
	slices.Sort(lines)
	return lines
}

func (v *SourceView) lineCollapsed(row int) bool {
	_, ok := v.collapsedLineRange(row)
	return ok && v.collapsedRevision == v.document.model.Revision() && v.wrap != WrapNone
}

// Unchanged paragraphs retain their source identity during IME projection,
// even when preedit inserts/removes temporary display paragraphs before them.
func (v *SourceView) collapsedLineRange(row int) (textedit.Range, bool) {
	sourceRow, ok := v.highlightRow(row)
	if !ok {
		return textedit.Range{}, false
	}
	r, ok := v.collapsedLines[sourceRow]
	return r, ok
}

func (v *SourceView) canCollapseLine(row int) bool {
	if v.Destroyed() || !gui.IsEnabled(v) || v.wrap == WrapNone || v.projection != nil || row < 0 || row >= v.document.model.LineCount() || v.lineCollapsed(row) {
		return false
	}
	if _, visible := v.visibleLines.index(row); !visible {
		return false
	}
	p := v.cachedParagraph(row)
	return p != nil && p.height >= 8*v.lineHeight && v.textRight()-v.gutter-2*padding >= 64
}

// CollapseLine shows only the first wrapped display line and an expand badge.
// The source, revision and undo history are unchanged. Only already measured
// lines at least eight base line heights tall are eligible; no language rule
// or scan of the remainder of a paged paragraph is required.
func (v *SourceView) CollapseLine(line int) bool {
	if !v.canCollapseLine(line) {
		return false
	}
	state := v.operationState()
	v.closeCompletion()
	if !state.valid(v) || !v.canCollapseLine(line) {
		return false
	}
	p := v.paragraph(line)
	if p == nil || !state.valid(v) {
		return false
	}
	if p.paged != nil && p.paged.GeometryAt(0) == nil {
		return false
	}
	if !state.valid(v) || v.Destroyed() {
		return false
	}
	g, _ := v.linePreview(p)
	if g == nil {
		return false
	}
	r, _ := v.document.model.LineRange(line)
	if v.collapsedLines == nil || v.collapsedRevision != v.document.model.Revision() {
		v.collapsedLines = make(map[int]textedit.Range)
	}
	v.collapsedRevision = v.document.model.Revision()
	v.collapsedLines[line] = r
	selection := v.Selection()
	upstream := v.upstream
	hidden := func(at int) bool { return at >= r.Start && at <= r.End && v.collapsedOffsetHidden(line, p, at, false) }
	if hidden(selection.Anchor) || hidden(selection.Caret) {
		pos := g.HitLine(0, 0)
		selection = textedit.Selection{Anchor: r.Start + pos.Offset, Caret: r.Start + pos.Offset}
		upstream = pos.Upstream
	}
	v.refreshCollapsedLine(line)
	if v.anchor.valid && v.anchor.offset >= r.Start && v.anchor.offset <= r.End {
		v.anchor.offset, v.anchor.inset = r.Start, 0
	}
	if selection != v.Selection() {
		v.setSelection(selection, upstream, false)
	}
	if v.document == state.document && v.document.model.Revision() == state.revision && v.mountEpoch == state.mount && !v.Destroyed() {
		v.updateMenu()
		v.foldingSignal.Emit()
	}
	return true
}

// ExpandLine restores a manually shortened logical line in this view.
func (v *SourceView) ExpandLine(line int) bool {
	if v.Destroyed() || !gui.IsEnabled(v) || v.projection != nil || v.collapsedRevision != v.document.model.Revision() {
		return false
	}
	if _, ok := v.collapsedLines[line]; !ok {
		return false
	}
	delete(v.collapsedLines, line)
	v.refreshCollapsedLine(line)
	v.updateMenu()
	v.foldingSignal.Emit()
	return true
}

func (v *SourceView) refreshCollapsedLine(row int) {
	v.layoutEpoch++
	if p := v.cachedParagraph(row); p != nil {
		v.setParagraphHeight(row, p.height)
	}
	if v.anchor.valid {
		v.pendingAnchor = true
	}
	v.beginViewportInitialization()
	v.requestLayoutWork()
	v.RequestLayout()
	v.RequestPaint()
}

// linePreview borrows cached first-page geometry; painting/snapshots never
// prepare missing pages just to locate a badge or describe hidden text.
func (v *SourceView) linePreview(p *paragraph) (*textedit.Geometry, float32) {
	if p == nil {
		return nil, v.lineHeight
	}
	g := &p.geometry
	if p.paged != nil {
		frame := p.paged.CachedGeometryAt(0)
		if frame == nil {
			return nil, v.lineHeight
		}
		g = &frame.Geometry
	}
	if g.LineCount() == 0 {
		return nil, v.lineHeight
	}
	return g, max(1, g.LineMetrics(0).Height)
}

func (v *SourceView) linePreviewRect(row int, p *paragraph) geometry.Rectangle {
	g, height := v.linePreview(p)
	width := max(0, v.textRight()-v.gutter-2*padding-lineCollapseBadgeWidth-4)
	if g != nil && width > 0 {
		// End on a native cluster boundary rather than cutting a glyph in half.
		// This uses visual stops, including RTL runs, not byte-count estimates.
		pos := g.HitLine(0, width)
		x := g.Caret(pos).X
		if x > width {
			if previous, ok := g.Visual(pos, false); ok && g.LineIndex(previous) == 0 {
				x = g.Caret(previous).X
			}
		}
		width = min(width, max(0, x))
	}
	return geometry.Rect(v.gutter+padding, padding+v.paragraphTop(row)-v.offset.Y,
		width, height)
}

func (v *SourceView) lineCollapseBadge(row int, p *paragraph) geometry.Rectangle {
	if !v.lineCollapsed(row) || p == nil {
		return geometry.Rectangle{}
	}
	r := v.linePreviewRect(row, p)
	return geometry.Rect(r.X+r.Width+4, r.Y, min(lineCollapseBadgeWidth, max(0, v.textRight()-padding-r.X-r.Width-4)), r.Height)
}

func (v *SourceView) collapsedOffsetHidden(row int, p *paragraph, at int, upstream bool) bool {
	r, ok := v.collapsedLineRange(row)
	if !ok || !v.lineCollapsed(row) || at < r.Start || at > r.End {
		return false
	}
	g, _ := v.linePreview(p)
	if g == nil {
		return at != r.Start
	}
	pos := textedit.Position{Offset: at - r.Start, Upstream: upstream}
	caret := g.Caret(pos)
	return g.LineIndex(pos) != 0 || caret.X < 0 || caret.X > v.linePreviewRect(row, p).Width
}

func (v *SourceView) expandCollapsedRange(r textedit.Range) bool {
	changed := false
	for row, line := range v.collapsedLines {
		if !v.lineCollapsed(row) || r.End < line.Start || r.Start > line.End {
			continue
		}
		hidden := r.Start != r.End || v.collapsedOffsetHidden(row, v.cachedParagraph(row), r.Start, false)
		if hidden {
			delete(v.collapsedLines, row)
			v.refreshCollapsedLine(row)
			changed = true
		}
	}
	return changed
}

func (v *SourceView) mapCollapsedLines(c textedit.Change) bool {
	v.collapsedRevision = c.Revision
	if len(v.collapsedLines) == 0 {
		return false
	}
	if c.Reset {
		v.collapsedLines = nil
		return true
	}
	mapped := make(map[int]textedit.Range, len(v.collapsedLines))
	changed := false
	for oldRow, line := range v.collapsedLines {
		valid := true
		for _, e := range c.Edits {
			if e.Range.Start <= line.End && e.Range.End >= line.Start {
				valid = false
				break
			}
			line.Start = textedit.MapOffset(line.Start, e)
			line.End = textedit.MapOffset(line.End, e)
		}
		if valid {
			row := v.document.model.LineAt(line.Start)
			current, _ := v.document.model.LineRange(row)
			if current == line {
				mapped[row] = line
				changed = changed || row != oldRow
				continue
			}
		}
		changed = true
	}
	v.collapsedLines = mapped
	return changed
}

func (v *SourceView) collapsedLineHit(point geometry.Point) (int, bool) {
	if v.Destroyed() || !gui.IsEnabled(v) || v.projection != nil {
		return 0, false
	}
	row := v.paragraphAt(max(0, point.Y+v.offset.Y-padding))
	badge := v.lineCollapseBadge(row, v.cachedParagraph(row))
	return row, badge.Width > 0 && badge.Height > 0 && containsPoint(badge, point)
}

func (v *SourceView) paintLineCollapseBadge(draw gui.Painter, row int, p *paragraph) {
	r := v.lineCollapseBadge(row, p)
	if r.Width <= 0 {
		return
	}
	line, _ := v.collapsedLineRange(row)
	selection := v.Selection().Range()
	if selection.Start <= line.Start && selection.End >= line.End {
		background := v.colors.InactiveSelectionBackground
		if activeFocus(v) {
			background = v.colors.SelectionBackground
		}
		draw.FillRect(r, graphics.ColorOf(background))
	} else if v.projection == nil && v.pointer != nil && v.pointer.Hover() && containsPoint(r, v.pointerPoint) {
		draw.FillRoundRect(r, 3, graphics.ColorOf(tint(v.colors.Foreground, 16)))
	}
	c := graphics.ColorOf(v.partColor("fold-placeholder", v.colors.GutterForeground))
	for n := range 3 {
		draw.FillRect(geometry.Rect(r.X+r.Width/2-5+float32(n)*4, r.Y+r.Height*.6, 2, 2), c)
	}
}

func (v *SourceView) snapshotCollapsedLines(info *gui.WidgetInfo, first, last int) {
	meta := info.Attributes["goui.sourceview"].(map[string]any)
	meta["collapsedLines"] = v.CollapsedLines()
	if v.wrap == WrapNone || len(v.collapsedLines) == 0 {
		return
	}
	if projection := v.projection; projection != nil {
		// Composition is exposed separately; do not report hidden committed
		// suffixes as visible text. Unchanged previews remain in place.
		at := projection.Replacement().Start
		info.TextEditing.VisibleRange, info.TextEditing.VisibleText = textedit.Range{Start: at, End: at}, ""
		delta := projection.ParagraphCount() - (projection.LastLine() - projection.FirstLine() + 1)
		for sourceRow := range v.collapsedLines {
			row := sourceRow
			if row >= projection.FirstLine() {
				if row <= projection.LastLine() {
					continue
				}
				row += delta
			}
			v.snapshotLineCollapseButton(info, row, sourceRow, false)
		}
		return
	}
	budget := 64 * 1024
	var spans []map[string]any
	for row := first; row <= last && budget > 0 && len(spans) < 256; row = v.nextDisplayRow(row) {
		r, _ := v.document.model.LineRange(row)
		if v.lineCollapsed(row) {
			p := v.cachedParagraph(row)
			if g, _ := v.linePreview(p); g != nil {
				line := g.LineMetrics(0)
				r.End = min(r.End, r.Start+line.Start+line.Length)
			} else {
				r.End = r.Start
			}
			v.snapshotLineCollapseButton(info, row, row, true)
		}
		r.End = min(r.End, r.Start+budget)
		for r.End > r.Start && !v.document.model.IsRuneBoundary(r.End) {
			r.End--
		}
		text, _ := v.document.model.Slice(r)
		budget -= len(text)
		spans = append(spans, map[string]any{"range": r, "text": text})
		if len(spans) == 1 {
			info.TextEditing.VisibleRange, info.TextEditing.VisibleText = r, text
		}
	}
	meta["visibleSpans"] = spans
}

func (v *SourceView) snapshotLineCollapseButton(info *gui.WidgetInfo, row, sourceRow int, enabled bool) {
	badge := v.lineCollapseBadge(row, v.cachedParagraph(row))
	if badge.Width <= 0 || badge.Y+badge.Height <= 0 || badge.Y >= v.viewport.Height {
		return
	}
	child := gui.WidgetInfo{ID: fmt.Sprintf("%s.line-collapse.%d", info.ID, sourceRow), Role: gui.RoleButton,
		Name: resource.ExpandLongLine(), Bounds: badge.Translate(info.Bounds.Pos), Visible: true, Enabled: enabled && gui.IsEnabled(v)}
	if child.Enabled {
		child.Actions = []gui.Action{gui.ActionClick}
	}
	child.SetAttribute("goui.sourceview.collapsedLine", sourceRow)
	info.Children = append(info.Children, child)
}
