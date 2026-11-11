package sourceview

import (
	"fmt"
	"math"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

// ViewState is an immutable, process-local snapshot of a view. It belongs to
// one Document and revision, retains no document/native resources, and can be
// reused by multiple views. It is not a file or undo-history snapshot.
type ViewState struct {
	document, revision uint64
	selection          textedit.Selection
	upstream           bool
	anchor             viewportAnchor
}

type viewRestore struct {
	snapshot *ViewState
	mount    uint64
	host     gui.Widget
	input    *restoreInput
}

// SaveState captures committed selection and viewport coordinates without
// measuring text. The view must be mounted and have observed the current model
// revision. An unmeasured viewport uses its last text anchor or a logical line.
func (v *SourceView) SaveState() (*ViewState, error) {
	if !v.mounted || v.Destroyed() {
		return nil, ErrNotMounted
	}
	if v.observed != v.document.model.Revision() {
		return nil, ErrOperationChanged
	}
	return &ViewState{document: v.document.ID(), revision: v.observed,
		selection: v.Selection(), upstream: v.upstream, anchor: v.savedViewportAnchor()}, nil
}

// RestoreState restores a snapshot from this Document's current revision.
// It accepts read-only views, does not acquire focus or edit text, and returns
// after accepting the request; geometry is resolved by the normal event loop.
// Input, explicit navigation, document changes and unmount cancel pending work.
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
	q := &viewRestore{snapshot: s, mount: v.mountEpoch}
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
	old := v.selection
	foldingChanged := v.unfoldSelection(s.selection)
	v.selection, v.upstream = s.selection, s.upstream
	v.selectionRevision = s.revision
	v.selectionEpoch++
	v.selectionTrail.steps = nil
	v.currentMatch = nil
	v.preferredSet = false
	v.reveal = nil
	v.anchor, v.anchorRevision, v.pendingAnchor = s.anchor, s.revision, true
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
	if old != v.selection {
		v.selectionSignal.Emit(v.selection)
	}
	if v.viewRestore != q || !state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	if foldingChanged {
		v.foldingSignal.Emit()
	}
	if v.viewRestore != q || !state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	v.searchState.Emit()
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
		v.releaseViewRestore()
		v.pendingAnchor, v.anchor.valid = false, false
	}
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
	pos := textedit.Position{Offset: at - r.Start, Upstream: a.upstream}
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
