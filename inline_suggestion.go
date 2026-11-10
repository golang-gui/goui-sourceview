package sourceview

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
)

// InlineSuggestion is a literal suffix at a UTF-8 byte position. Logical line
// ends accept up to 64 KiB / 256 lines, normalized to UTF-8/LF. Tabs are allowed
// after the first newline; the initial appended fragment requires literal spaces.
// Text is uncommitted: it is never part of Model, copy, search or saved output.
type InlineSuggestion struct {
	Position int
	Text     string
}

type inlineSuggestionState struct {
	request    InlineSuggestion
	state      operationState
	layout     typography.TextLayout
	geometry   textedit.Geometry
	font       typography.FontInfo
	color      color.NRGBA
	multi      *multilineInlineLayout
	row        int
	baseHeight float32
}

func (v *SourceView) InlineSuggestionsEnabled() bool { return v.inlineSuggestionsEnabled }

func (v *SourceView) SetInlineSuggestionsEnabled(enabled bool) {
	if v.inlineSuggestionsEnabled == enabled {
		return
	}
	v.inlineSuggestionsEnabled = enabled
	if !enabled {
		v.ClearInlineSuggestion()
	}
}

// SetInlineSuggestion publishes a host-provided result on the GUI sequence.
// Foreign/stale documents, inactive views, selections and non-line-end positions
// return false, nil. Invalid text/ranges return an error without replacing the
// existing suggestion. The host owns requests, cancellation and generation order.
func (v *SourceView) SetInlineSuggestion(document *Document, revision uint64, suggestion InlineSuggestion) (bool, error) {
	if document != v.document || revision != v.document.model.Revision() {
		return false, nil
	}
	if suggestion.Position < 0 || suggestion.Position > document.model.Len() || !document.model.IsRuneBoundary(suggestion.Position) {
		return false, fmt.Errorf("sourceview: invalid inline suggestion position")
	}
	if !utf8.ValidString(suggestion.Text) || len(suggestion.Text) > 64<<10 {
		return false, fmt.Errorf("sourceview: invalid or oversized inline suggestion")
	}
	suggestion.Text = textedit.Normalize(suggestion.Text)
	head, _, _ := strings.Cut(suggestion.Text, "\n")
	if strings.TrimSpace(suggestion.Text) == "" || strings.Count(suggestion.Text, "\n") >= 256 || strings.ContainsRune(head, '\t') || strings.ContainsFunc(suggestion.Text, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\t' || r == '\u2028' || r == '\u2029'
	}) {
		return false, fmt.Errorf("sourceview: inline suggestion requires nonblank text, at most 256 lines, and no first-fragment tabs or other controls")
	}
	if !v.inlineSuggestionAvailable() {
		return false, nil
	}
	selection := v.Selection()
	line, _ := document.model.LineRange(document.model.LineAt(suggestion.Position))
	if selection.Anchor != selection.Caret || selection.Caret != suggestion.Position || suggestion.Position != line.End {
		return false, nil
	}
	state := v.operationState()
	v.ClearInlineSuggestion()
	if !state.valid(v) || v.Destroyed() {
		return false, ErrOperationChanged
	}
	s := &inlineSuggestionState{request: suggestion, state: state, row: document.model.LineAt(suggestion.Position)}
	v.inlineSuggestion = s
	if err := v.prepareInlineSuggestion(); err != nil {
		return false, err
	}
	v.RequestPaint()
	return v.inlineSuggestion == s && v.inlineSuggestionCurrent(), nil
}

// InlineSuggestion returns a detached current request, including when its caret
// is outside the viewport. Snapshot describes actual preview visibility.
func (v *SourceView) InlineSuggestion() (InlineSuggestion, bool) {
	if !v.inlineSuggestionCurrent() {
		return InlineSuggestion{}, false
	}
	return v.inlineSuggestion.request, true
}

func (v *SourceView) ClearInlineSuggestion() {
	s := v.inlineSuggestion
	if s == nil {
		return
	}
	v.inlineSuggestion = nil
	v.restoreInlineExtent(s)
	if s.multi != nil {
		s.multi.destroy()
		s.multi = nil
	}
	if s.layout != nil {
		s.layout.Destroy()
		s.layout = nil
	}
	v.RequestPaint()
}

func (v *SourceView) inlineSuggestionAvailable() bool {
	return !v.Destroyed() && v.inlineSuggestionsEnabled && v.mounted && v.Focused() && activeFocus(v) && v.editable() == nil && v.projection == nil && v.completion == nil &&
		(v.completionQueue == nil || !v.completionQueue.pending())
}

func (v *SourceView) inlineSuggestionCurrent() bool {
	s := v.inlineSuggestion
	return s != nil && s.state.valid(v) && v.inlineSuggestionAvailable() && v.Selection() == (textedit.Selection{Anchor: s.request.Position, Caret: s.request.Position})
}

// Native preparation stays outside Paint/Snapshot: one layout for a single-line
// request, or a measured head/tail pair for multiline view-local space.
func (v *SourceView) prepareInlineSuggestion() error {
	s := v.inlineSuggestion
	if s == nil {
		return nil
	}
	if !v.inlineSuggestionCurrent() {
		v.ClearInlineSuggestion()
		return nil
	}
	if v.context == nil {
		return nil
	}
	c := v.partColor("inline-suggestion", v.colors.GutterForeground)
	if strings.ContainsRune(s.request.Text, '\n') {
		return v.prepareMultilineInline(s, c)
	}
	if s.layout != nil && s.font == v.font && s.color == c {
		return nil
	}
	if s.layout != nil {
		old := s.layout
		s.layout = nil
		old.Destroy()
	}
	if v.inlineSuggestion != s || !v.inlineSuggestionCurrent() {
		return nil
	}
	font := v.font
	l, err := v.context.NewTextLayout(s.request.Text, typography.TextFormat{Font: font, WrapMode: typography.WrapNone, TextColor: c, TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}, 1<<20, 1<<20)
	if err != nil {
		if v.inlineSuggestion == s {
			v.ClearInlineSuggestion()
		}
		return err
	}
	if v.inlineSuggestion != s || !v.inlineSuggestionCurrent() {
		l.Destroy()
		return nil
	}
	lines, clusters := l.MeasureMetrics()
	if v.inlineSuggestion != s || !v.inlineSuggestionCurrent() {
		l.Destroy()
		return nil
	}
	s.layout, s.font, s.color = l, font, c
	s.geometry = textedit.NewGeometry(lines, clusters, len(s.request.Text), v.lineHeight)
	return nil
}

type inlinePreview struct {
	bounds, textClip geometry.Rectangle
	origin           geometry.Point
	truncated        bool
}

// Preview is one display row. A logical end inside an RTL run is deliberately
// withheld: appending an independent run there could overlap real source text.
func (v *SourceView) inlinePreview() (inlinePreview, bool) {
	s := v.inlineSuggestion
	if s != nil && s.multi != nil {
		return v.multilineInlinePreview(s)
	}
	if !v.inlineSuggestionCurrent() || s.layout == nil || s.font != v.font || s.color != v.partColor("inline-suggestion", v.colors.GutterForeground) || s.geometry.LineCount() != 1 {
		return inlinePreview{}, false
	}
	row := v.document.model.LineAt(s.request.Position)
	if v.lineCollapsed(row) {
		return inlinePreview{}, false
	}
	p := v.cachedParagraph(row)
	if p == nil {
		return inlinePreview{}, false
	}
	r, _ := v.document.model.LineRange(row)
	pos := textedit.Position{Offset: s.request.Position - r.Start, Upstream: true}
	g, origin := &p.geometry, v.paragraphOrigin(row, p)
	if p.paged != nil {
		frame := p.paged.CachedGeometryAt(pos.Offset)
		if frame == nil {
			return inlinePreview{}, false
		}
		g, origin = &frame.Geometry, v.geometryOrigin(row, frame.X, frame.Y)
	}
	if g.LineCount() == 0 {
		return inlinePreview{}, false
	}
	line := g.LineMetrics(g.LineIndex(pos))
	caret := g.Caret(pos)
	if caret.X < line.X+line.Width-.5 {
		return inlinePreview{}, false
	}
	x, y := origin.X+caret.X+1, origin.Y+line.Y
	width := v.textRight() - padding - x
	if x < v.gutter+padding || width < 16 || y < 0 || y+line.Height > v.viewport.Height {
		return inlinePreview{}, false
	}
	ghost := s.geometry.LineMetrics(0)
	width = min(width, max(0, ghost.Width))
	if width <= 0 {
		return inlinePreview{}, false
	}
	preview := inlinePreview{bounds: geometry.Rect(x, y, width, line.Height), origin: geometry.Point{X: x - ghost.X, Y: origin.Y + line.Baseline - ghost.Baseline}}
	preview.textClip = preview.bounds
	if ghost.Width > width {
		preview.truncated = true
		limit := max(0, width-14)
		stop := s.geometry.HitLine(0, ghost.X+limit)
		edge := s.geometry.Caret(stop).X - ghost.X
		if edge > limit {
			if previous, ok := s.geometry.Visual(stop, false); ok {
				edge = s.geometry.Caret(previous).X - ghost.X
			}
		}
		preview.textClip.Width = max(0, min(limit, edge))
	}
	return preview, true
}

func (v *SourceView) paintInlineSuggestion(p gui.Painter) {
	preview, ok := v.inlinePreview()
	if !ok {
		return
	}
	s := v.inlineSuggestion
	if s.multi != nil {
		v.paintMultilineInline(p, s)
		return
	}
	p.Save()
	p.SetClipRect(preview.textClip)
	p.DrawTextLayout(preview.origin, s.layout)
	p.Restore()
	if preview.truncated {
		c := graphics.ColorOf(s.color)
		x := preview.bounds.X + preview.bounds.Width - 12
		for i := range 3 {
			p.FillRect(geometry.Rect(x+float32(i)*4, preview.bounds.Y+preview.bounds.Height*.65, 2, 2), c)
		}
	}
}

// AcceptInlineSuggestion commits a visible, still-valid suffix as one undo unit.
// No visible suggestion is a normal no-op. It never enables a stale result.
func (v *SourceView) AcceptInlineSuggestion() error {
	if _, ok := v.inlinePreview(); !ok {
		return nil
	}
	s := v.inlineSuggestion
	at, text := s.request.Position, s.request.Text
	v.ClearInlineSuggestion()
	if !s.state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	end := at + len(text)
	return v.ApplyEdits(s.state.document, s.state.revision, []textedit.Edit{{Range: textedit.Range{Start: at, End: at}, Text: text}}, textedit.Selection{Anchor: end, Caret: end})
}

func (v *SourceView) inlineSuggestionKey(event events.KeyEvent) bool {
	if event.Modifiers != 0 {
		return false
	}
	if event.Key != events.KeyTab && event.Key != events.KeyEscape {
		return false
	}
	_, visible := v.inlinePreview()
	if event.Key == events.KeyEscape {
		// Also discard a retained offscreen request, while leaving ordinary
		// Escape handling intact when no suggestion was actually displayed.
		v.ClearInlineSuggestion()
		return visible
	}
	if !visible {
		return false
	}
	v.escapeTab = false
	v.report(v.AcceptInlineSuggestion())
	return true
}

func (v *SourceView) snapshotInlineSuggestion(info *gui.WidgetInfo) {
	meta := info.Attributes["goui.sourceview"].(map[string]any)
	meta["inlineSuggestionsEnabled"] = v.inlineSuggestionsEnabled
	if suggestion, ok := v.InlineSuggestion(); ok {
		preview, visible := v.inlinePreview()
		meta["inlineSuggestion"] = map[string]any{"position": suggestion.Position, "text": suggestion.Text, "visible": visible, "bounds": preview.bounds.Translate(info.Bounds.Pos), "truncated": preview.truncated, "lineCount": strings.Count(suggestion.Text, "\n") + 1}
	}
}
