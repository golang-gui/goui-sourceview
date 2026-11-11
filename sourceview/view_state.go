package sourceview

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"

	"github.com/golang-gui/goui-sourceview/internal/folding"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
)

// ViewState is an immutable, process-local snapshot of a view. It belongs to
// one Document and revision, retains no document/native resources, and can be
// reused by multiple views. It is not a file or undo-history snapshot.
type ViewState struct {
	document, revision uint64
	language           uint64
	selection          TextSelection
	additional         []textcore.SelectionState
	upstream           bool
	anchor             viewportAnchor
	folds, lines       []TextRange
}

const maxSavedFolds = 4096

type viewRestore struct {
	snapshot     *ViewState
	host         gui.Widget
	input        *restoreInput
	wanted       map[TextRange]bool
	foldsPending bool
}

// SaveState captures committed selection, viewport and collapse intent without
// measuring text. The view must be mounted and have observed the current model
// revision. An unmeasured viewport uses its last text anchor or a logical line.
// Saving more than 4096 structural and manual line collapses returns ErrOperationLimit.
func (v *SourceView) SaveState() (*ViewState, error) {
	if !v.mounted || v.Destroyed() {
		return nil, ErrNotMounted
	}
	if v.observed != v.document.model.Revision() {
		return nil, ErrOperationChanged
	}
	s := &ViewState{document: v.document.ID(), revision: v.observed, language: v.document.languageEpoch,
		selection: v.Selection(), upstream: v.upstream, anchor: v.savedViewportAnchor(), additional: cursorHistory(v.additional)}
	if q := v.viewRestore; q != nil && q.foldsPending && q.snapshot.language == s.language {
		s.folds = slices.Clone(q.snapshot.folds)
	} else if v.foldingEnabled {
		if len(v.collapsedFolds) > maxSavedFolds {
			return nil, ErrOperationLimit
		}
		for _, r := range v.collapsedFolds {
			s.folds = append(s.folds, r)
		}
	}
	if len(s.folds)+len(v.collapsedLines) > maxSavedFolds {
		return nil, ErrOperationLimit
	}
	for _, r := range v.collapsedLines {
		s.lines = append(s.lines, r)
	}
	return s, nil
}

// RestoreState restores a snapshot from this Document's current revision.
// It accepts read-only views, does not acquire focus or edit text, and returns
// after accepting the request; geometry is resolved by the normal event loop.
// Input, explicit navigation, document changes and unmount cancel pending work.
// A changed language definition discards structural fold intent. Configuration,
// including language, wrapping and whether folding is enabled, is unchanged.
func (v *SourceView) RestoreState(s *ViewState) error {
	if s == nil || s.document == 0 {
		return fmt.Errorf("sourceview: invalid view state")
	}
	if !v.mounted || v.Destroyed() {
		return ErrNotMounted
	}
	if s.document != v.document.ID() || s.revision != v.document.model.Revision() || v.observed != s.revision {
		return ErrOperationChanged
	}
	v.cancelViewRestore()
	// Copy the opaque value as well as its private slice headers. Callers can
	// reuse or replace their ViewState variable after submitting the request.
	snapshot := *s
	snapshot.additional = slices.Clone(s.additional)
	s = &snapshot
	q := &viewRestore{snapshot: s}
	v.viewRestore = q
	accepted := false
	defer func() {
		if !accepted && v.viewRestore == q {
			v.cancelViewRestore()
		}
	}()
	state := v.operationState()
	v.pendingInput = nil
	v.cancelPaste()
	v.cancelEdit()
	v.cancelPreedit()
	if v.viewRestore != q || !state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	v.ClearInlineSuggestion()
	v.closeCompletion()
	if v.viewRestore != q || !state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	v.document.model.BreakUndoGroup()
	old := v.Selections()
	v.selection, v.upstream = s.selection, s.upstream
	v.additional = cursorsFromHistory(s.additional)
	v.selectionRevision = s.revision
	v.selectionEpoch++
	v.selectionTrail.steps = nil
	v.currentMatch = nil
	v.preferredSet = false
	v.reveal = nil
	v.anchor, v.anchorRevision, v.pendingAnchor = s.anchor, s.revision, true
	lines := make(map[int]TextRange, len(s.lines))
	v.collapsedRevision = s.revision
	for _, r := range s.lines {
		lines[v.document.model.LineAt(r.Start)] = r
	}
	manualChanged := !maps.Equal(v.collapsedLines, lines)
	v.collapsedLines = lines
	v.startFoldRestore(q)
	if manualChanged {
		v.layoutEpoch++
		v.resetHeights()
		v.contentWidth = 0
	}
	// Restoring a distant viewport must not turn the old visible rows into a
	// synchronous first-allocation job. layoutStep prioritizes the text anchor.
	v.initialLayout = nil
	q.host = v
	for p := v.Parent(); p != nil; p = p.Parent() {
		if _, ok := p.(*gui.ScrollView); ok {
			q.host = p
			break
		}
	}
	q.input = &restoreInput{EventControllerBase: gui.NewEventControllerBase(gui.PhaseCapture), view: v}
	q.host.AddEventController(q.input)
	v.resetBlink()
	v.requestLayoutWork()
	v.RequestLayout()
	v.RequestPaint()
	state = v.operationState()
	v.emitSelections(old)
	if v.viewRestore != q || !state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	if manualChanged {
		v.foldingSignal.Emit()
	}
	if v.viewRestore != q || !state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	v.searchState.Emit()
	if v.viewRestore != q || !state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	accepted = true
	return nil
}

func (v *SourceView) savedViewportAnchor() viewportAnchor {
	if v.anchor.valid && v.anchorRevision == v.observed && (v.pendingAnchor || v.projection != nil) {
		return v.anchor
	}
	if s := v.inlineSuggestion; s != nil && s.multi != nil {
		if _, _, hit := v.inlineVirtualHit(geometry.Point{X: v.gutter + padding}); hit {
			return viewportAnchor{offset: s.request.Position, upstream: true, x: v.offset.X, valid: true}
		}
	}
	row := v.document.model.LineAt(v.Selection().Caret)
	if v.viewport.Height > 0 && v.heights.Count() > 0 {
		row = v.paragraphAt(max(0, v.offset.Y-padding))
	}
	r, _ := v.document.model.LineRange(row)
	a := viewportAnchor{offset: r.Start, x: v.offset.X, valid: true}
	if v.projection == nil {
		if p := v.cachedParagraph(row); p != nil && p.Geometry.LineCount() > 0 {
			origin := v.paragraphOrigin(row, p)
			pos := p.Geometry.Hit(geometry.Point{X: v.gutter + padding - origin.X, Y: -origin.Y})
			index := p.Geometry.LineIndex(pos)
			edge := p.Geometry.LineEdge(index, false)
			a.offset, a.upstream = r.Start+edge.Offset, edge.Upstream
			a.inset = -origin.Y - p.Geometry.LineMetrics(index).Y
		}
	}
	return a
}

func (v *SourceView) cancelViewRestore() {
	if v.viewRestore != nil {
		if v.foldIntake != nil && v.foldIntake.restore == v.viewRestore {
			intake := v.foldIntake
			v.foldIntake = nil
			v.releaseViewRestore()
			if intake.revision == v.document.model.Revision() && intake.generation == v.document.foldGeneration {
				v.queueFolds(intake.snapshot)
			}
		} else {
			v.releaseViewRestore()
		}
		v.pendingAnchor, v.anchor.valid = false, false
	}
}

// Candidate matching stays in the existing budgeted intake. A view-owned
// candidate slice is borrowed read-only until publication or cancellation.
func (v *SourceView) startFoldRestore(q *viewRestore) {
	d := v.document
	if !v.foldingEnabled || q.snapshot.language != d.languageEpoch {
		return
	}
	q.wanted = make(map[TextRange]bool, len(q.snapshot.folds))
	for _, r := range q.snapshot.folds {
		q.wanted[r] = true
	}
	var f *Folds
	if d.foldReady && d.foldRevision == d.model.Revision() && d.foldSnapshot != nil {
		f = d.foldSnapshot
	} else if v.foldIntake != nil && v.foldIntake.generation == d.foldGeneration {
		f = v.foldIntake.snapshot
	} else if len(v.folds) > 0 && v.foldDefinition == d.definition {
		f = &Folds{entries: v.folds, length: d.model.Len(), lines: d.model.LineCount()}
	}
	q.foldsPending = f != nil || len(q.wanted) > 0 && d.hasFolding()
	if f != nil {
		v.queueFolds(f)
	} else if q.foldsPending {
		d.requestWork()
	}
}

func (v *SourceView) restoreFoldsPending() bool {
	q := v.viewRestore
	if q == nil {
		return false
	}
	if q.snapshot.language != v.document.languageEpoch {
		q.foldsPending = false
		q.wanted = nil
	}
	return q.foldsPending
}

// Resolve only the two selection endpoints, using cached first-line geometry.
// A narrower destination may need to expand a saved one-line preview to keep
// the selection accessible. Missing geometry is prepared by layoutStep.
func (v *SourceView) protectRestoredSelection(prepare func(int) bool) bool {
	if v.viewRestore == nil || v.restoreFoldsPending() {
		return v.viewRestore == nil
	}
	q := v.viewRestore
	changed, pending := false, false
	endpoints := make([]int, 0, (len(v.additional)+1)*2)
	for _, s := range v.Selections() {
		endpoints = append(endpoints, s.Anchor, s.Caret)
	}
	for _, at := range endpoints {
		row := v.document.model.LineAt(at)
		r, _ := v.document.model.LineRange(row)
		if !v.lineCollapsed(row) || at == r.Start {
			continue
		}
		if prepare != nil && !prepare(row) {
			if v.viewRestore != q {
				return false
			}
			pending = true
			break
		}
		p := v.cachedParagraph(row)
		g, _ := v.linePreview(p)
		if g == nil {
			v.requestLayoutWork()
			pending = true
			break
		}
		if v.collapsedOffsetHidden(row, p, at, v.upstream && at == v.selection.Caret) {
			delete(v.collapsedLines, row)
			if i := v.foldHeader(row); i >= 0 && v.folds[i].Collapsed {
				v.folds[i].Collapsed = false
				delete(v.collapsedFolds, row)
			}
			changed = true
		}
	}
	if changed {
		var hidden []folding.Interval
		rows := make([]int, 0, len(v.collapsedFolds))
		for row := range v.collapsedFolds {
			rows = append(rows, row)
		}
		sort.Ints(rows)
		for _, row := range rows {
			r := v.collapsedFolds[row]
			hidden = folding.AppendHidden(hidden, row+1, v.document.model.LineAt(r.End)+1)
		}
		v.refreshFoldVisibility(hidden)
		v.initialLayout = nil
		v.foldingSignal.Emit()
		return false
	}
	return !pending
}

func (v *SourceView) releaseViewRestore() {
	q := v.viewRestore
	if q == nil {
		return
	}
	v.viewRestore = nil
	if q.input != nil {
		q.host.RemoveEventController(q.input)
		q.input.view = nil
	}
}

// Capture input on the scroll host so scrollbar drags and wheels also cancel.
// Layout-driven scroll corrections never travel through this controller.
type restoreInput struct {
	gui.EventControllerBase
	view *SourceView
}

func (c *restoreInput) HandleEvent(ctx gui.EventContext) {
	if c.view == nil {
		return
	}
	switch ctx.Event().Type() {
	case events.PointerDown, events.Wheel, events.KeyDown:
		c.view.cancelViewRestore()
	}
}

func (v *SourceView) resolveViewportAnchor() {
	if v.restoreFoldsPending() || !v.protectRestoredSelection(nil) {
		return
	}
	a := v.anchor
	state, epoch := v.operationState(), v.layoutEpoch
	at := v.clampOffset(a.offset)
	row := v.document.model.LineAt(at)
	r, _ := v.document.model.LineRange(row)
	p := v.viewportParagraph(row)
	if p == nil || !state.valid(v) || epoch != v.layoutEpoch || v.Destroyed() {
		return
	}
	g, baseY := &p.Geometry, p.GeometryY
	if p.Paged != nil {
		frame := p.Paged.CachedGeometryAt(at - r.Start)
		if frame == nil {
			v.requestLayoutWork()
			return
		}
		g, baseY = &frame.Geometry, frame.Y
	}
	if g.LineCount() == 0 {
		v.requestLayoutWork()
		return
	}
	pos := textcore.Position{Offset: at - r.Start, Upstream: a.upstream}
	rect := g.Caret(pos)
	inset := a.inset
	if v.viewRestore != nil {
		inset = max(-padding, min(inset, max(0, g.LineMetrics(g.LineIndex(pos)).Height-1)))
	}
	size := v.ContentSize()
	x := max(0, min(a.x, size.Width-v.viewport.Width))
	y := max(0, min(float32(float64(padding)+v.paragraphTop64(row)+baseY+float64(rect.Y)+float64(inset)), max(0, size.Height-v.viewport.Height)))
	if math.IsNaN(float64(x)) || math.IsNaN(float64(y)) {
		v.cancelViewRestore()
		return
	}
	v.pendingAnchor = false
	v.offset = geometry.Point{X: x, Y: y}
	v.releaseViewRestore()
	v.scrollSignal.Emit(geometry.Rect(x, y, v.viewport.Width, v.viewport.Height))
}
