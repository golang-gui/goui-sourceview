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

	"github.com/golang-gui/goui-sourceview/internal/folding"
	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/internal/resource"
	"github.com/golang-gui/goui-sourceview/internal/view"
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

func (v *SourceView) FoldingEnabled() bool { return v.foldingEnabled }

func (v *SourceView) SetFoldingEnabled(enabled bool) {
	if v.Destroyed() || v.foldingEnabled == enabled {
		return
	}
	v.cancelViewRestore()
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
		out[i] = FoldRegion(r.Region)
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
	if !v.canFold() || i < 0 {
		return false
	}
	v.cancelViewRestore()
	if v.folds[i].Collapsed == collapsed {
		return false
	}
	v.folds[i].Collapsed = collapsed
	if !collapsed {
		delete(v.collapsedLines, line)
	}
	v.finishFoldCommand()
	return true
}

func (v *SourceView) setAllFolded(collapsed bool) {
	if !v.canFold() {
		return
	}
	v.cancelViewRestore()
	changed := false
	for i := range v.folds {
		if v.folds[i].Collapsed != collapsed {
			v.folds[i].Collapsed = collapsed
			if !collapsed {
				delete(v.collapsedLines, v.folds[i].StartLine)
			}
			changed = true
		}
	}
	if changed {
		v.finishFoldCommand()
	}
}

func (v *SourceView) finishFoldCommand() {
	v.ClearInlineSuggestion()
	s := v.Selection()
	upstream := v.upstream
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
	// A wrapped header is itself a one-row preview. Do not leave either
	// endpoint in its hidden suffix (including the endpoint moved above).
	for _, at := range []int{s.Anchor, s.Caret} {
		row := v.document.model.LineAt(at)
		p := v.cachedParagraph(row)
		if v.collapsedOffsetHidden(row, p, at, false) {
			header, _ := v.document.model.LineRange(row)
			pos := textedit.Position{}
			if g, _ := v.linePreview(p); g != nil {
				pos = g.HitLine(0, 0)
			}
			s = textedit.Selection{Anchor: header.Start + pos.Offset, Caret: header.Start + pos.Offset}
			upstream = pos.Upstream
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
		v.setSelection(s, upstream, true)
	}
	if !v.Destroyed() && v.document == state.document && v.mountEpoch == state.mount {
		v.foldingSignal.Emit()
	}
}

func (v *SourceView) foldedLines() []folding.Interval {
	if !v.foldingEnabled {
		return nil
	}
	return folding.Hidden(v.folds)
}

func (v *SourceView) refreshFoldGeometry() {
	v.indexCollapsedFolds()
	v.refreshFoldVisibility(v.foldedLines())
}

func (v *SourceView) indexCollapsedFolds() {
	v.collapsedFolds = make(map[int]textedit.Range)
	for _, r := range v.folds {
		if r.Collapsed {
			v.collapsedFolds[r.StartLine] = textedit.Range{Start: r.Start, End: r.End}
		}
	}
}

// Intake builds visibility and the collapsed index incrementally. Publishing
// them must not rescan every candidate on the UI sequence.
func (v *SourceView) refreshFoldVisibility(hidden []folding.Interval) {
	v.foldState++
	v.hiddenLines = hidden
	if v.anchor.valid {
		row := v.document.model.LineAt(v.anchor.offset)
		i := sort.Search(len(hidden), func(i int) bool { return hidden[i].End > row })
		if i < len(hidden) && hidden[i].Start <= row {
			h, _ := v.document.model.LineRange(hidden[i].Start - 1)
			v.anchor.offset, v.anchor.inset = h.Start, 0
		}
		v.pendingAnchor = true
	}
	v.layoutEpoch++
	v.resetHeights()
	v.contentWidth = 0
	v.beginViewportInitialization()
	v.requestLayoutWork()
	v.RequestLayout()
	v.RequestPaint()
}

func (v *SourceView) acceptFolds(ranges []language.FoldRange) {
	if v.projection != nil {
		return
	}
	v.folds = folding.Reconcile(v.folds, ranges, v.document.model.LineRange)
	v.indexCollapsedFolds()
	v.foldRevision = v.document.model.Revision()
	v.foldDefinition = v.document.definition
	// Candidate analysis changes controls, not paragraph visibility, when no
	// collapsed extent changed. Preserve measured rows and completed reveals.
	if slices.Equal(v.hiddenLines, v.foldedLines()) && v.visibleLines.Total() == v.displayCount() && v.heights.Count() == v.visibleLines.Count() {
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
	v.folds = folding.MapEntries(v.folds, c, splices, incremental, v.document.model.LineAt)
	v.foldRevision = c.Revision
	// Candidate changes alone do not change paragraph visibility. The change
	// observer splices ordinary row heights and preserves unchanged layouts.
	if len(v.hiddenLines) > 0 || len(v.foldedLines()) > 0 {
		v.refreshFoldGeometry()
	}
}

func (v *SourceView) expandFoldRange(r textedit.Range) bool {
	lineChanged := v.expandCollapsedRange(r)
	changed := false
	for i := range v.folds {
		f := &v.folds[i]
		if !f.Collapsed {
			continue
		}
		start, _ := v.document.model.LineRange(f.StartLine + 1)
		header, _ := v.document.model.LineRange(f.StartLine)
		headerHidden := r.End >= header.Start && r.Start <= header.End && v.lineCollapsed(f.StartLine) &&
			(r.Start != r.End || v.collapsedOffsetHidden(f.StartLine, v.cachedParagraph(f.StartLine), r.Start, false))
		if r.Start <= f.End && r.End >= start.Start || headerHidden {
			f.Collapsed = false
			delete(v.collapsedLines, f.StartLine)
			changed = true
		}
	}
	if changed {
		v.refreshFoldGeometry()
	}
	return changed || lineChanged
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
	if v.foldClear {
		v.foldClear = false
		v.clearFolds()
		v.foldingSignal.Emit()
		if v.document != d || v.Destroyed() {
			return
		}
	}
	if d.foldReady && d.foldRevision == d.model.Revision() && (d.folding == nil || !d.folding.busy) {
		v.queueFolds(d.foldSnapshot)
	}
}

// Folds is an immutable, worker-safe folding snapshot. It retains ranges and
// normalized byte endpoints, never the source text or a mutable model.
type Folds struct {
	entries       []folding.Entry
	length, lines int
}

// FoldRange uses zero-based logical lines and an inclusive final line.
type FoldRange = language.FoldRange

// NewFolds copies and validates candidates against normalized UTF-8/LF text.
// Duplicate headers retain their outermost range; crossing ranges are invalid.
func NewFolds(text string, ranges []FoldRange) (*Folds, error) {
	snapshot, err := folding.BuildSnapshot(text, ranges)
	if err != nil {
		return nil, err
	}
	return &Folds{entries: snapshot.Entries, length: snapshot.Length, lines: snapshot.Lines}, nil
}

// SetFolds publishes analysis for revision on the document's owning sequence.
// A non-nil empty snapshot suppresses built-in folds; nil restores them. Stale
// revisions return false, nil. Callers also check their own analysis generation.
func (d *Document) SetFolds(revision uint64, folds *Folds) (bool, error) {
	if revision != d.model.Revision() {
		return false, nil
	}
	if folds != nil && (folds.length != d.model.Len() || folds.lines != d.model.LineCount()) {
		return false, fmt.Errorf("sourceview: folds do not match document size")
	}
	d.cancelFolding()
	d.externalFolds = folds != nil
	d.foldSnapshot = folds
	d.foldRevision = revision
	d.foldReady = folds != nil
	generation := d.foldGeneration
	for v := range d.views {
		if d.foldGeneration != generation {
			break
		}
		if folds == nil {
			if v.projection != nil {
				v.foldClear = true
				v.foldIntake = nil
			} else {
				v.clearFolds()
				v.foldingSignal.Emit()
			}
		} else {
			v.queueFolds(folds)
		}
	}
	if folds == nil {
		d.requestFolding(true)
	} else {
		d.requestWork()
	}
	return true, nil
}

func (d *Document) hasFolding() bool { return d.externalFolds || d.definition.Folding() != nil }

type foldIntake struct {
	snapshot                    *Folds
	generation, revision, state uint64
	old                         map[textedit.Range]bool
	entries                     []folding.Entry
	collapsed                   map[int]textedit.Range
	hidden                      []folding.Interval
	selection                   uint64
	restore                     *viewRestore
	at                          int
}

func (v *SourceView) queueFolds(f *Folds) {
	if f == nil {
		return
	}
	q := &foldIntake{snapshot: f, generation: v.document.foldGeneration, revision: v.document.model.Revision(), state: v.foldState,
		selection: v.selectionEpoch, old: make(map[textedit.Range]bool), collapsed: make(map[int]textedit.Range)}
	if r := v.viewRestore; r != nil && r.foldsPending && r.snapshot.language == v.document.languageEpoch && v.foldingEnabled {
		q.restore, q.old = r, r.wanted
	} else {
		for _, r := range v.collapsedFolds {
			q.old[r] = true
		}
	}
	v.foldIntake = q
	v.document.requestWork()
}
func (v *SourceView) foldingIntakeStep() {
	q, d := v.foldIntake, v.document
	if q == nil || v.projection != nil {
		return
	}
	if q.generation != d.foldGeneration || q.revision != d.model.Revision() {
		v.foldIntake = nil
		return
	}
	if q.state != v.foldState || q.selection != v.selectionEpoch {
		v.queueFolds(q.snapshot)
		return
	}
	deadline := time.Now().Add(2 * time.Millisecond)
	a, b := d.model.LineAt(v.Selection().Anchor), d.model.LineAt(v.Selection().Caret)
	for n := 0; n < 256 && time.Now().Before(deadline); n++ {
		if q.at == len(q.snapshot.entries) {
			break
		}
		r := q.snapshot.entries[q.at]
		key := textedit.Range{Start: r.Start, End: r.End}
		r.Collapsed = v.foldingEnabled && q.old[key] && !(a > r.StartLine && a <= r.EndLine || b > r.StartLine && b <= r.EndLine)
		if r.Collapsed {
			q.collapsed[r.StartLine] = key
			q.hidden = folding.AppendHidden(q.hidden, r.StartLine+1, r.EndLine+1)
		}
		q.entries = append(q.entries, r)
		q.at++
	}
	if q.at < len(q.snapshot.entries) {
		return
	}
	v.foldIntake = nil
	v.folds = q.entries
	v.collapsedFolds = q.collapsed
	v.foldRevision = q.revision
	v.foldDefinition = d.definition
	if slices.Equal(v.hiddenLines, q.hidden) && v.visibleLines.Total() == v.displayCount() && v.heights.Count() == v.visibleLines.Count() {
		v.RequestLayout()
		v.RequestPaint()
	} else {
		v.refreshFoldVisibility(q.hidden)
	}
	if r := q.restore; r != nil && v.viewRestore == r {
		r.foldsPending = false
		v.initialLayout = nil
		v.requestLayoutWork()
	}
	v.foldingSignal.Emit()
}

type foldingJob struct {
	revision, epoch uint64
	definition      *language.Language
	cancel          context.CancelFunc
	ctx             context.Context
	chunks          []string
	copied          int
}
type foldingQueue struct {
	owner  *Document
	busy   bool
	active *foldingJob
}

func (d *Document) cancelFolding() {
	if d.folding != nil && d.folding.active != nil {
		d.folding.active.cancel()
		d.folding.active = nil
	}
	d.foldGeneration++
}
func (d *Document) requestFolding(immediate bool) {
	if d.externalFolds && d.foldReady && d.foldRevision == d.model.Revision() {
		return
	}
	d.cancelFolding()
	d.foldReady = false
	d.foldDue = time.Now().Add(150 * time.Millisecond)
	if immediate {
		d.foldDue = time.Now()
	}
	if d.externalFolds {
		d.foldSnapshot = nil
		d.foldReady = true
	}
	d.requestWork()
}
func (d *Document) foldingStep() {
	for v := range d.views {
		v.foldingIntakeStep()
	}
	if d.externalFolds || d.foldReady || d.definition.Folding() == nil || gui.App == nil {
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
	job := q.active
	if job == nil {
		ctx, cancel := context.WithCancel(context.Background())
		job = &foldingJob{revision: d.model.Revision(), epoch: d.foldGeneration, definition: d.definition, cancel: cancel, ctx: ctx}
		q.active = job
	}
	// Model access stays on GUI; joining and analysis happen on the worker.
	end := min(d.model.Len(), job.copied+64*1024)
	for !d.model.IsRuneBoundary(end) {
		end--
	}
	chunk, _ := d.model.Slice(textedit.Range{Start: job.copied, End: end})
	job.chunks = append(job.chunks, chunk)
	job.copied = end
	if end < d.model.Len() {
		d.requestWork()
		return
	}
	d.foldReady = true
	q.busy = true
	fn, post := d.definition.Folding(), gui.App.Post
	go func() {
		text := strings.Join(job.chunks, "")
		job.chunks = nil
		ranges, err := fn(job.ctx, text)
		var f *Folds
		if err == nil {
			err = job.ctx.Err()
		}
		if err == nil {
			f, err = NewFolds(text, ranges)
		}
		post(func() { q.finish(job, f, err) })
	}()
}
func (q *foldingQueue) finish(job *foldingJob, f *Folds, err error) {
	q.busy = false
	job.cancel()
	d := q.owner
	if q.active == job {
		q.active = nil
	}
	if d == nil {
		return
	}
	if d.model.Revision() == job.revision && d.foldGeneration == job.epoch && d.definition == job.definition && !errors.Is(err, context.Canceled) {
		d.foldSnapshot = f
		d.foldRevision = job.revision
		d.foldReady = true
		for v := range d.views {
			if d.foldGeneration != job.epoch {
				break
			}
			if !v.mounted || v.document != d {
				continue
			}
			if err != nil {
				v.clearFolds()
				v.report(err)
			} else {
				v.queueFolds(f)
			}
		}
	}
	d.requestWork()
}
func (d *Document) stopFolding() {
	d.cancelFolding()
	if d.folding != nil {
		d.folding.owner = nil
		d.folding = nil
	}
	if !d.externalFolds {
		d.foldSnapshot = nil
		d.foldReady = false
	}
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
		return view.NRGBA(c)
	}
	return fallback
}

func (v *SourceView) foldButtonRect(row int) geometry.Rectangle {
	return geometry.Rect(v.gutter-v.foldColumn, padding+v.paragraphTop(row)-v.offset.Y, v.foldColumn, v.lineHeight)
}

func (v *SourceView) foldBadgeRect(row int, p *paragraph) geometry.Rectangle {
	if i := v.foldHeader(row); i < 0 || !v.folds[i].Collapsed || v.projection != nil || v.lineCollapsed(row) {
		return geometry.Rectangle{}
	}
	edge := p.Geometry.Caret(textedit.Position{Offset: len(p.text), Upstream: true})
	r := geometry.Rect(v.gutter+padding-v.offset.X+edge.X+5, padding+v.paragraphTop(row)-v.offset.Y+edge.Y, 16, edge.Height)
	if v.showLineEndings {
		r.X += 12
	}
	if v.wrap != WrapNone && r.X+r.Width > v.textRight()-padding {
		return geometry.Rectangle{}
	}
	return r
}

func (v *SourceView) paintFoldButton(p gui.Painter, row int) {
	collapsed, ok := v.gutterFold(row)
	if !ok {
		return
	}
	r := v.foldButtonRect(row)
	x, y := r.X+r.Width/2, r.Y+r.Height/2
	c := graphics.ColorOf(v.partColor("fold-toggle", v.colors.GutterForeground))
	if collapsed {
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
	if s := v.Selection().Range(); s.Start <= v.folds[v.foldHeader(row)].Start && s.End > v.folds[v.foldHeader(row)].End {
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
	if _, ok := v.gutterFold(row); !ok {
		return 0, false
	}
	if view.ContainsPoint(v.foldButtonRect(row), point) {
		return row, true
	}
	// The badge is painted only from cached geometry. Pointer motion and clicks
	// must not shape an unmeasured paragraph just to look for an invisible badge.
	if p := v.cachedParagraph(row); p != nil && view.ContainsPoint(v.foldBadgeRect(row, p), point) {
		return row, true
	}
	return 0, false
}

// Structural headers take precedence over long-line candidates. One gutter
// control describes the combined visible state and expands both intents.
func (v *SourceView) gutterFold(row int) (collapsed, available bool) {
	if v.Destroyed() || !v.foldingEnabled || v.foldColumn == 0 || v.projection != nil || v.foldRevision != v.document.model.Revision() || v.foldDefinition != v.document.definition {
		return false, false
	}
	if i := v.foldHeader(row); i >= 0 {
		return v.folds[i].Collapsed || v.wrap != WrapNone && v.manuallyCollapsed(row), true
	}
	return v.lineCollapsed(row), v.lineCollapsed(row) || v.longLineCandidate(row)
}

func (v *SourceView) toggleGutterFold(row int) bool {
	collapsed, ok := v.gutterFold(row)
	if !ok {
		return false
	}
	if collapsed {
		return v.expandLinePreview(row)
	}
	if v.foldHeader(row) >= 0 {
		return v.Fold(row)
	}
	return v.CollapseLine(row)
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
	meta["foldingPending"] = v.foldIntake != nil || v.viewRestore != nil && v.viewRestore.foldsPending && v.viewRestore.snapshot.language == v.document.languageEpoch || !v.document.externalFolds && v.document.definition.Folding() != nil && (!v.document.foldReady || v.document.folding != nil && v.document.folding.busy)
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
		if collapsed, ok := v.gutterFold(row); ok {
			label := resource.Fold()
			end, kind := row, "line"
			if i := v.foldHeader(row); i >= 0 {
				end, kind = v.folds[i].EndLine, "structure"
			} else {
				label = resource.CollapseLongLine()
			}
			if collapsed {
				label = resource.Unfold()
				if kind == "line" {
					label = resource.ExpandLongLine()
				}
			}
			r := v.foldButtonRect(row).Translate(geometry.Point{X: info.Bounds.X, Y: info.Bounds.Y})
			child := gui.WidgetInfo{ID: fmt.Sprintf("%s.fold.%d", info.ID, row), Role: gui.RoleButton, Name: label, Bounds: r, Visible: true, Enabled: gui.IsEnabled(v)}
			if child.Enabled {
				child.Actions = []gui.Action{gui.ActionClick}
			}
			child.SetAttribute("goui.sourceview.fold", map[string]any{"startLine": row, "endLine": end, "collapsed": collapsed, "kind": kind})
			info.Children = append(info.Children, child)
		}
	}
	if len(v.hiddenLines) > 0 {
		meta["visibleSpans"] = spans
	}
}
