package sourceview

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/style"
)

// FoldRegion is a detached snapshot. Lines are zero-based and EndLine inclusive.
type FoldRegion struct {
	StartLine, EndLine int
	Collapsed          bool
}

type foldEntry struct {
	FoldRegion
	start, end int
}

func normalizeFolds(ranges []language.FoldRange, lines int) ([]language.FoldRange, error) {
	ranges = slices.Clone(ranges)
	for _, r := range ranges {
		if r.StartLine < 0 || r.EndLine <= r.StartLine || r.EndLine >= lines {
			return nil, fmt.Errorf("sourceview: invalid fold range %v", r)
		}
	}
	if len(ranges) > lines {
		return nil, fmt.Errorf("sourceview: more fold headers than lines")
	}
	slices.SortFunc(ranges, func(a, b language.FoldRange) int {
		if a.StartLine != b.StartLine {
			return a.StartLine - b.StartLine
		}
		return b.EndLine - a.EndLine
	})
	var out []language.FoldRange
	var stack []int
	for _, r := range ranges {
		if r.StartLine < 0 || r.EndLine <= r.StartLine || r.EndLine >= lines {
			return nil, fmt.Errorf("sourceview: invalid fold range %v", r)
		}
		if len(out) > 0 && out[len(out)-1].StartLine == r.StartLine {
			continue
		}
		for len(stack) > 0 && r.StartLine > stack[len(stack)-1] {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && r.EndLine > stack[len(stack)-1] {
			return nil, fmt.Errorf("sourceview: crossing fold range %v", r)
		}
		stack = append(stack, r.EndLine)
		out = append(out, r)
	}
	return out, nil
}

func (v *SourceView) FoldingEnabled() bool { return v.foldingEnabled }

func (v *SourceView) SetFoldingEnabled(enabled bool) {
	if v.Destroyed() || v.foldingEnabled == enabled {
		return
	}
	v.foldingEnabled = enabled
	if !enabled {
		for i := range v.folds {
			v.folds[i].Collapsed = false
		}
	}
	v.refreshFoldGeometry()
	if enabled {
		v.document.requestFolding(true)
	}
	v.foldingSignal.Emit()
}

func (v *SourceView) FoldingRegions() []FoldRegion {
	if v.foldRevision != v.document.model.Revision() || v.foldDefinition != v.document.definition {
		return nil
	}
	out := make([]FoldRegion, len(v.folds))
	for i, r := range v.folds {
		out[i] = r.FoldRegion
	}
	return out
}

func (v *SourceView) ConnectFoldingChanged(fn func()) signal.Handle {
	return v.foldingSignal.Connect(fn)
}

func (v *SourceView) Fold(line int) bool { return v.setFolded(line, true) }

func (v *SourceView) Unfold(line int) bool { return v.setFolded(line, false) }

func (v *SourceView) ToggleFold(line int) bool {
	i := v.foldHeader(line)
	if i < 0 {
		return false
	}
	return v.setFolded(line, !v.folds[i].Collapsed)
}

func (v *SourceView) FoldAll() { v.setAllFolded(true) }

func (v *SourceView) UnfoldAll() { v.setAllFolded(false) }

func (v *SourceView) foldHeader(line int) int {
	i := sort.Search(len(v.folds), func(i int) bool { return v.folds[i].StartLine >= line })
	if i < len(v.folds) && v.folds[i].StartLine == line {
		return i
	}
	return -1
}

func (v *SourceView) canFold() bool {
	return !v.Destroyed() && v.foldingEnabled && gui.IsEnabled(v) && v.projection == nil && v.foldRevision == v.document.model.Revision() && v.foldDefinition == v.document.definition
}

func (v *SourceView) setFolded(line int, collapsed bool) bool {
	i := v.foldHeader(line)
	if !v.canFold() || i < 0 || v.folds[i].Collapsed == collapsed {
		return false
	}
	v.folds[i].Collapsed = collapsed
	v.finishFoldCommand()
	return true
}

func (v *SourceView) setAllFolded(collapsed bool) {
	if !v.canFold() {
		return
	}
	changed := false
	for i := range v.folds {
		if v.folds[i].Collapsed != collapsed {
			v.folds[i].Collapsed = collapsed
			changed = true
		}
	}
	if changed {
		v.finishFoldCommand()
	}
}

func (v *SourceView) finishFoldCommand() {
	s := v.Selection()
	for _, r := range v.folds {
		if !r.Collapsed {
			continue
		}
		a, b := v.document.model.LineAt(s.Anchor), v.document.model.LineAt(s.Caret)
		if a > r.StartLine && a <= r.EndLine || b > r.StartLine && b <= r.EndLine {
			header, _ := v.document.model.LineRange(r.StartLine)
			s = textedit.Selection{Anchor: header.End, Caret: header.End}
			break
		}
	}
	state := v.operationState()
	v.closeCompletion()
	if !state.valid(v) || v.Destroyed() {
		return
	}
	v.refreshFoldGeometry()
	if s != v.Selection() {
		v.setSelection(s, false, true)
	}
	if !v.Destroyed() && v.document == state.document && v.mountEpoch == state.mount {
		v.foldingSignal.Emit()
	}
}

func (v *SourceView) foldedLines() []lineInterval {
	var hidden []lineInterval
	if v.foldingEnabled {
		for _, r := range v.folds {
			if !r.Collapsed {
				continue
			}
			h := lineInterval{r.StartLine + 1, r.EndLine + 1}
			if n := len(hidden); n > 0 && h.start <= hidden[n-1].end {
				hidden[n-1].end = max(hidden[n-1].end, h.end)
			} else {
				hidden = append(hidden, h)
			}
		}
	}
	return hidden
}

func (v *SourceView) refreshFoldGeometry() {
	v.hiddenLines = v.foldedLines()
	if v.anchor.valid {
		row := v.document.model.LineAt(v.anchor.offset)
		for _, r := range v.folds {
			if r.Collapsed && row > r.StartLine && row <= r.EndLine {
				h, _ := v.document.model.LineRange(r.StartLine)
				v.anchor.offset = h.Start
				break
			}
		}
		v.pendingAnchor = true
	}
	v.layoutEpoch++
	v.resetHeights()
	v.contentWidth = 0
	v.RequestLayout()
	v.RequestPaint()
}

func (v *SourceView) acceptFolds(ranges []language.FoldRange) {
	if v.projection != nil {
		return
	}
	old := make(map[[2]int]bool, len(v.folds))
	for _, r := range v.folds {
		old[[2]int{r.start, r.end}] = r.Collapsed
	}
	v.folds = make([]foldEntry, 0, len(ranges))
	for _, r := range ranges {
		a, _ := v.document.model.LineRange(r.StartLine)
		b, _ := v.document.model.LineRange(r.EndLine)
		entry := foldEntry{FoldRegion: FoldRegion{StartLine: r.StartLine, EndLine: r.EndLine}, start: a.Start, end: b.End}
		entry.Collapsed = old[[2]int{entry.start, entry.end}]
		v.folds = append(v.folds, entry)
	}
	v.foldRevision = v.document.model.Revision()
	v.foldDefinition = v.document.definition
	// Candidate analysis changes controls, not paragraph visibility, when no
	// collapsed extent changed. Preserve measured rows and completed reveals.
	if slices.Equal(v.hiddenLines, v.foldedLines()) && v.visibleLines.total == v.displayCount() && v.heights.Count() == v.visibleLines.count {
		v.RequestPaint()
	} else {
		v.refreshFoldGeometry()
	}
	v.foldingSignal.Emit()
}

func (v *SourceView) clearFolds() {
	v.folds = nil
	v.foldRevision = v.document.model.Revision()
	v.foldDefinition = v.document.definition
	v.refreshFoldGeometry()
}

func (v *SourceView) mapFolds(c textedit.Change, splices []highlight.Splice) {
	if c.Reset {
		v.clearFolds()
		return
	}
	if len(v.folds) == 0 {
		v.foldRevision = c.Revision
		return
	}
	if v.foldRevision >= c.Revision {
		return
	}
	// Splices describe the same sequential edits in the old line index. An
	// untouched fold only needs the line-count delta of edits before it, not
	// two document position queries for every candidate on every keystroke.
	incremental := v.foldRevision+1 == c.Revision && len(splices) == len(c.Edits)
	out := v.folds[:0]
	for _, r := range v.folds {
		valid := true
		for i, e := range c.Edits {
			before := e.Range.Start == r.start && e.Range.Start == e.Range.End && strings.HasSuffix(e.Text, "\n")
			if !before && e.Range.Start <= r.end && e.Range.End >= r.start {
				valid = false
				break
			}
			if incremental && e.Range.Start <= r.start {
				delta := splices[i].Added - splices[i].Removed
				r.StartLine += delta
				r.EndLine += delta
			}
			r.start = textedit.MapOffset(r.start, e)
			r.end = textedit.MapOffset(r.end, e)
		}
		if valid {
			if !incremental {
				r.StartLine = v.document.model.LineAt(r.start)
				r.EndLine = v.document.model.LineAt(r.end)
			}
			if r.EndLine > r.StartLine {
				out = append(out, r)
			}
		}
	}
	v.folds = out
	v.foldRevision = c.Revision
	// Candidate changes alone do not change paragraph visibility. The change
	// observer splices ordinary row heights and preserves unchanged layouts.
	if len(v.hiddenLines) > 0 || len(v.foldedLines()) > 0 {
		v.refreshFoldGeometry()
	}
}

func (v *SourceView) expandFoldRange(r textedit.Range) bool {
	changed := false
	for i := range v.folds {
		f := &v.folds[i]
		if !f.Collapsed {
			continue
		}
		start, _ := v.document.model.LineRange(f.StartLine + 1)
		if r.Start <= f.end && r.End >= start.Start {
			f.Collapsed = false
			changed = true
		}
	}
	if changed {
		v.refreshFoldGeometry()
	}
	return changed
}

func (v *SourceView) unfoldSelection(s textedit.Selection) bool {
	a := v.expandFoldRange(textedit.Range{Start: s.Anchor, End: s.Anchor})
	b := v.expandFoldRange(textedit.Range{Start: s.Caret, End: s.Caret})
	return a || b
}

func (v *SourceView) foldNavigationTarget(at int, forward bool) int {
	for _, f := range v.folds {
		if !f.Collapsed {
			continue
		}
		row := v.document.model.LineAt(at)
		if row <= f.StartLine || row > f.EndLine {
			continue
		}
		if forward {
			if f.EndLine+1 < v.document.model.LineCount() {
				r, _ := v.document.model.LineRange(f.EndLine + 1)
				return r.Start
			}
			r, _ := v.document.model.LineRange(f.StartLine)
			return r.End
		}
		r, _ := v.document.model.LineRange(f.StartLine)
		return r.End
	}
	return at
}

func (v *SourceView) unfoldDeleteBoundary(forward bool) bool {
	s := v.Selection()
	if s.Anchor != s.Caret {
		return false
	}
	at := v.adjacent(s.Caret, forward)
	a, b := min(at, s.Caret), max(at, s.Caret)
	if v.expandFoldRange(textedit.Range{Start: a, End: b}) {
		v.foldingSignal.Emit()
		return true
	}
	return false
}

func (v *SourceView) resumeFolding() {
	d := v.document
	if d.foldReady && d.foldRevision == d.model.Revision() && (d.folding == nil || !d.folding.busy) {
		v.acceptFolds(d.foldRanges)
	}
}

type foldingJob struct {
	revision, epoch uint64
	definition      *language.Language
	cancel          context.CancelFunc
}

type foldingQueue struct {
	owner  *Document
	busy   bool
	active *foldingJob
}

func (d *Document) requestFolding(immediate bool) {
	if d.folding != nil && d.folding.active != nil {
		d.folding.active.cancel()
		d.folding.active = nil
	}
	d.foldGeneration++
	d.foldReady = false
	d.foldDue = time.Now().Add(150 * time.Millisecond)
	if immediate {
		d.foldDue = time.Now()
	}
	d.requestWork()
}

func (d *Document) foldingStep() {
	if d.foldReady || d.definition.Folding() == nil || gui.App == nil {
		return
	}
	wanted := false
	for v := range d.views {
		wanted = wanted || v.foldingEnabled
	}
	if !wanted {
		return
	}
	if d.folding == nil {
		d.folding = &foldingQueue{owner: d}
	}
	q := d.folding
	if q.busy || time.Now().Before(d.foldDue) {
		d.requestWork()
		return
	}
	d.foldReady = true // active job owns this revision; later edits request again
	ctx, cancel := context.WithCancel(context.Background())
	job := &foldingJob{revision: d.model.Revision(), epoch: d.foldGeneration, definition: d.definition, cancel: cancel}
	text, fn, post := d.model.Text(), d.definition.Folding(), gui.App.Post
	q.active = job
	q.busy = true
	go func() { ranges, err := fn(ctx, text); post(func() { q.finish(ranges, err) }) }()
}

func (q *foldingQueue) finish(ranges []language.FoldRange, err error) {
	q.busy = false
	job := q.active
	q.active = nil
	d := q.owner
	if job != nil {
		job.cancel()
		if d != nil && len(d.views) > 0 && d.model.Revision() == job.revision && d.foldGeneration == job.epoch && d.definition == job.definition {
			if err == nil {
				ranges, err = normalizeFolds(ranges, d.model.LineCount())
			}
			if !errors.Is(err, context.Canceled) {
				if err != nil {
					ranges = nil
				}
				d.foldRanges = ranges
				d.foldRevision = job.revision
				d.foldReady = true
				views := make([]*SourceView, 0, len(d.views))
				for v := range d.views {
					views = append(views, v)
				}
				for _, v := range views {
					if !v.mounted || v.document != d || d.model.Revision() != job.revision || d.definition != job.definition {
						continue
					}
					if err != nil {
						v.clearFolds()
						v.report(err)
					} else {
						v.acceptFolds(ranges)
					}
				}
			}
		}
	}
	if d != nil {
		d.requestWork()
	}
}

func (d *Document) stopFolding() {
	if d.folding != nil {
		if d.folding.active != nil {
			d.folding.active.cancel()
			d.folding.active = nil
		}
		d.folding.owner = nil
		d.folding = nil
	}
	d.foldRanges = nil
	d.foldReady = false
	d.foldGeneration++
}

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
	// The badge is painted only from cached geometry. Pointer motion and clicks
	// must not shape an unmeasured paragraph just to look for an invisible badge.
	if p := v.cachedParagraph(row); p != nil && containsPoint(v.foldBadgeRect(row, p), point) {
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
