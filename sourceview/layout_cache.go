package sourceview

import (
	"encoding/binary"
	"image/color"
	"slices"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

const maxLayouts = 128

type layoutKey struct {
	id, revision uint64
	row          int
	width        float32
}

func (k layoutKey) sameText(other layoutKey) bool {
	return k.id == other.id && k.revision == other.revision && k.row == other.row
}

type paragraph struct {
	view.ParagraphResources

	pageVersion uint64
	colorRange  TextRange
	textStart   int
	sourceRange TextRange

	whitespace    *view.WhitespaceLayout
	fragment      textcore.Fragment
	text          string
	width, height float32
	style         paragraphStyle
	styled        bool
	replaced      bool
	foreground    color.NRGBA
	colors        []paragraphColor
	used          uint64
	row           int
	numberText    string
}

// Non-default effective foreground ranges, after token and selection overlays.
// Retaining their values avoids invalidating native shaping when analysis
// revisions change but the actual colors do not.
type paragraphColor struct {
	start, end int
	color      color.NRGBA
}

// Paint-only state is deliberately separate from the native layout's text,
// font, tab and wrapping identity.
type paragraphStyle struct {
	scheme   *scheme.Scheme
	language language.ID
	// Display snapshot identity; semantic readiness is tracked by tokenValid.
	highlightEpoch, highlightStyleEpoch uint64
	tokenVersion                        uint64
	languageEpoch                       uint64
	tokenValid                          bool
	selection                           TextRange
	selections                          string
	focused                             bool
}

func (v *SourceView) dropLayouts() {
	v.layoutEpoch++
	v.layoutPending = false
	v.initialLayout = nil
	v.layoutFailures = nil
	old := v.layouts
	v.layouts = make(map[layoutKey]*paragraph)
	for _, p := range old {
		p.ParagraphResources.Destroy()
	}
}

// Width is part of measurement identity. Keep native resources and their
// immutable metrics together so returning to a recent width needs no shaping.
// Font, tabs, wrapping mode, projection and Context changes still drop layouts.
func (v *SourceView) paragraphWidth() float32 {
	if v.wrap == WrapNone {
		return 1 << 20
	}
	return max(1, v.textRight()-v.gutter-2*padding)
}

// This lookup is also used by Snapshot: it neither shapes nor updates tokens.
func (v *SourceView) paragraphKey(row int) layoutKey {
	key := layoutKey{revision: v.document.model.Revision(), row: row, width: v.paragraphWidth()}
	if tokenRow, ok := v.highlightRow(row); ok && v.document.engine != nil {
		if line := v.document.engine.Line(tokenRow); line != nil {
			key = layoutKey{id: line.ID, width: key.width}
		}
	}
	return key
}

// Explicit editing operations still request exact cluster geometry. Viewport
// layout/paint can instead enqueue a missing result for the next frame.
func (v *SourceView) paragraph(row int) *paragraph { return v.resolveParagraph(row, true) }

func (v *SourceView) viewportParagraph(row int) *paragraph {
	return v.resolveParagraph(row, !v.frameLayout)
}

// A read-only lookup must not shape, recolor, adopt identities or update heights.
func (v *SourceView) cachedParagraph(row int) *paragraph {
	key := v.paragraphKey(row)
	p := v.layouts[key]
	if p == nil && key.id != 0 {
		p = v.layouts[layoutKey{revision: v.document.model.Revision(), row: row, width: key.width}]
	}
	return p
}

// Keep live row ownership current and identify resources whose input was
// replaced. Only those resources may be transferred to a new text identity;
// equal text in two live rows must never share a mutable native layout.
func (v *SourceView) mapParagraphs(splices []highlight.Splice) {
	for _, p := range v.layouts {
		if p.replaced {
			continue
		}
		for _, s := range splices {
			if p.row >= s.Index && p.row < s.Index+s.Removed {
				p.replaced = true
				break
			}
			if p.row >= s.Index+s.Removed {
				p.row += s.Added - s.Removed
			}
		}
	}
}

func (v *SourceView) resolveParagraph(row int, measure bool) *paragraph {
	if v.context == nil || row < 0 || row >= v.displayCount() {
		return nil
	}
	if v.mounted && v.observed != v.document.model.Revision() {
		return nil
	}
	v.ensureHeights()
	if _, visible := v.visibleLines.Index(row); !visible {
		return nil
	}
	r := v.displayRange(row)
	key := v.paragraphKey(row)
	e := v.document.engine
	tokenRow, hasTokens := v.highlightRow(row)
	var version uint64
	valid := false
	var tokens []highlight.Token
	if hasTokens && e != nil {
		_, _, valid = e.Tokens(tokenRow)
		tokens, version, _ = e.PaintTokens(tokenRow)
		if !valid {
			e.Ensure(tokenRow)
			v.document.requestWork()
		}
	}
	focus := activeFocus(v)
	sel := TextRange{}
	if v.colors.SelectionForeground.A == 255 && focus && v.projection == nil {
		selection := v.Selection().Range()
		sel = TextRange{Start: max(0, min(r.End, selection.Start)-r.Start), End: max(0, min(r.End, selection.End)-r.Start)}
		if sel.Start == sel.End {
			sel = TextRange{}
		}
	}
	// Selection geometry is painted independently. Only an actual foreground
	// override makes selection/focus part of the native text's color identity.
	style := paragraphStyle{highlightEpoch: v.document.highlightEpoch, highlightStyleEpoch: v.highlightStyleEpoch, scheme: v.appliedScheme, language: v.document.language, languageEpoch: v.document.languageEpoch, tokenVersion: version, tokenValid: valid, selection: sel, focused: sel.End > sel.Start}
	if len(v.additional) > 0 && v.colors.SelectionForeground.A == 255 && focus && v.projection == nil {
		var encoded []byte
		for _, s := range selectionIntersections(v.selectionRanges(), r) {
			encoded = binary.AppendVarint(encoded, int64(max(r.Start, s.Start)-r.Start))
			encoded = binary.AppendVarint(encoded, int64(min(r.End, s.End)-r.Start))
		}
		style.selections = string(encoded)
		style.focused = len(encoded) > 0
	}
	state, epoch := v.operationState(), v.layoutEpoch
	p := v.layouts[key]
	adopted := false
	if p == nil && key.id != 0 {
		// Initial lexical indexing supplies stable IDs after a plain first
		// frame. The revision/row entry already has exactly this text.
		fallback := layoutKey{revision: v.document.model.Revision(), row: row, width: key.width}
		p = v.layouts[fallback]
		if p != nil {
			delete(v.layouts, fallback)
			v.layouts[key] = p
			adopted = true
		}
	}
	var text string
	if r.End-r.Start > 64*1024 {
		return v.resolveLongParagraph(row, key, p, style, tokens, focus, measure)
	}
	if p == nil && measure {
		text = v.displayText(row)
		var previous layoutKey
		for old, candidate := range v.layouts {
			if candidate.replaced && old.width == key.width && candidate.text == text && (p == nil || candidate.used > p.used) {
				previous, p = old, candidate
			}
		}
		if p != nil {
			delete(v.layouts, previous)
			v.layouts[key] = p
			p.replaced, p.styled = false, false
			adopted = true
		}
	}
	v.tick++
	if p != nil {
		if !p.styled || p.style != style {
			if !measure {
				v.requestLayoutWork()
			} else if !v.colorParagraph(key, p, style, tokens, focus) {
				return nil
			}
		}
		p.used = v.tick
		p.row = row
		v.setParagraphHeight(row, p.height)
		width := p.width
		if i := v.foldHeader(row); i >= 0 && v.folds[i].Collapsed {
			width += 33 // Reserve the maximum badge extent, independent of paint-only LF markers.
		}
		v.contentWidth = max(v.contentWidth, width)
		if adopted {
			// Several provisional widths can acquire one stable identity after
			// indexing. Apply the same bound when adopting, not only on creation.
			v.pruneLayouts(key)
			if !state.valid(v) || epoch != v.layoutEpoch || v.layouts[key] != p {
				return nil
			}
		}
		return p
	}
	if !measure {
		if _, failed := v.layoutFailures[key]; failed {
			return nil
		}
		v.requestLayoutWork()
		return nil
	}
	format := typography.TextFormat{Font: v.font, WrapMode: v.wrap, TextColor: v.colors.Foreground, TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}
	native, err := v.context.NewTextLayout(text, format, key.width, 1<<24)
	if err != nil {
		// A failed automatic request must not retry/report on every frame.
		// Bound failures like retained resources; new input identities can retry.
		if v.frameLayout && state.valid(v) && epoch == v.layoutEpoch {
			if v.layoutFailures == nil || len(v.layoutFailures) >= maxLayouts {
				v.layoutFailures = make(map[layoutKey]struct{})
			}
			v.layoutFailures[key] = struct{}{}
		}
		v.report(err)
		return nil
	}
	if !state.valid(v) || epoch != v.layoutEpoch {
		native.Destroy()
		return nil
	}
	delete(v.layoutFailures, key)
	p = &paragraph{ParagraphResources: view.ParagraphResources{Layout: native}, text: text, used: v.tick, row: row, foreground: v.colors.Foreground}
	if !v.colorParagraph(key, p, style, tokens, focus) {
		native.Destroy() // Not cached yet: the local call still owns this object.
		return nil
	}
	w, h := native.MeasureSize()
	lines, clusters := native.MeasureMetrics()
	p.whitespace = view.NewWhitespaceLayout(text, lines, clusters)
	var g textcore.Geometry
	fragment, fragmentErr := textcore.NewFragment(TextRange{End: len(text)}, geometry.Size{Width: w, Height: h}, lines, clusters)
	if fragmentErr == nil {
		g = fragment.Geometry(v.lineHeight)
	} else {
		// Preserve Geometry's general semantics for nonpartitioning metrics.
		g = textcore.NewGeometry(lines, clusters, len(text), v.lineHeight)
	}
	p.fragment = fragment
	p.Geometry, p.width, p.height = g, w, max(v.lineHeight, h)
	oldSize := v.ContentSize()
	v.layouts[key] = p
	v.setParagraphHeight(row, p.height)
	if i := v.foldHeader(row); i >= 0 && v.folds[i].Collapsed {
		w += 33
	}
	v.contentWidth = max(v.contentWidth, w)
	if v.ContentSize() != oldSize {
		v.RequestLayout()
	}
	v.pruneLayouts(key)
	if !state.valid(v) || epoch != v.layoutEpoch || v.layouts[key] != p {
		return nil
	}
	return p
}

func (v *SourceView) colorParagraph(key layoutKey, p *paragraph, style paragraphStyle, tokens []highlight.Token, focus bool) bool {
	state, epoch := v.operationState(), v.layoutEpoch
	owner := v.layouts[key]
	valid := func() bool {
		return state.valid(v) && epoch == v.layoutEpoch && v.layouts[key] == owner && v.appliedScheme == style.scheme && v.document.language == style.language && v.document.highlightEpoch == style.highlightEpoch && v.highlightStyleEpoch == style.highlightStyleEpoch && activeFocus(v) == focus
	}
	if !valid() {
		return false
	}
	length := len(p.text)
	if p.Paged != nil {
		length = v.displayRange(p.row).End - v.displayRange(p.row).Start
	}
	r := v.displayRange(p.row)
	local := TextRange{End: length}
	if p.Paged != nil {
		local = p.Paged.VisibleRange()
	}
	spans := v.displayHighlights(TextRange{Start: r.Start + local.Start, End: r.Start + local.End})
	for i := range spans {
		spans[i].Range.Start -= r.Start
		spans[i].Range.End -= r.Start
	}
	colors := v.paragraphColors(length, style, tokens, spans...)
	if p.Paged != nil {
		visible := p.Paged.VisibleRange()
		clipped := colors[:0]
		for _, c := range colors {
			c.start, c.end = max(c.start, visible.Start), min(c.end, visible.End)
			if c.end > c.start {
				clipped = append(clipped, c)
			}
		}
		colors = clipped
		if p.foreground != v.colors.Foreground || !slices.Equal(p.colors, colors) || p.pageVersion != p.Paged.Version() || p.colorRange != visible {
			p.Paged.SetColor(v.colors.Foreground)
			if !valid() {
				return false
			}
			for _, c := range colors {
				p.Paged.SetTextColor(TextRange{Start: c.start, End: c.end}, c.color)
				if !valid() {
					return false
				}
			}
		}
		p.style, p.styled, p.colors, p.foreground = style, true, colors, v.colors.Foreground
		p.pageVersion = p.Paged.Version()
		p.colorRange = visible
		return valid()
	}
	if p.foreground != v.colors.Foreground || !slices.Equal(p.colors, colors) {
		// Complete replacement clears obsolete tokens/selection and bounds
		// backend attributes. A fresh layout already has TextFormat's base.
		if len(p.text) > 0 && (p.foreground != v.colors.Foreground || len(p.colors) > 0) {
			p.Layout.SetTextColor(0, len(p.text), v.colors.Foreground)
			if !valid() {
				return false
			}
		}
		for _, c := range colors {
			p.Layout.SetTextColor(c.start, c.end-c.start, c.color)
			if !valid() {
				return false
			}
		}
		p.foreground, p.colors = v.colors.Foreground, colors
	}
	if p.Number != nil && (!p.styled || p.style.scheme != style.scheme) {
		p.Number.SetTextColor(0, len(p.numberText), v.colors.GutterForeground)
		if !valid() {
			return false
		}
	}
	p.style, p.styled = style, true
	return true
}

func (v *SourceView) paragraphColors(length int, style paragraphStyle, tokens []highlight.Token, spans ...HighlightSpan) []paragraphColor {
	var out []paragraphColor
	appendRange := func(start, end int, c color.NRGBA) {
		if start >= end || c == v.colors.Foreground {
			return
		}
		if n := len(out); n > 0 && out[n-1].end == start && out[n-1].color == c {
			out[n-1].end = end
		} else {
			out = append(out, paragraphColor{start, end, c})
		}
	}
	sel := TextRange{}
	if v.colors.SelectionForeground.A == 255 && style.focused {
		sel = style.selection
	}
	selected := []TextRange{sel}
	if style.selections != "" {
		selected = nil
		data := []byte(style.selections)
		for len(data) > 0 {
			start, n := binary.Varint(data)
			data = data[n:]
			end, n := binary.Varint(data)
			data = data[n:]
			selected = append(selected, TextRange{Start: int(start), End: int(end)})
		}
	}
	appendSelected := func(start, end int, c color.NRGBA) {
		for _, s := range selectionIntersections(selected, TextRange{Start: start, End: end}) {
			if s.Start > start {
				appendRange(start, min(end, s.Start), c)
			}
			lo, hi := max(start, s.Start), min(end, s.End)
			if hi > lo {
				appendRange(lo, hi, v.colors.SelectionForeground)
				start = hi
			}
		}
		appendRange(start, end, c)
	}

	si := 0
	appendHighlighted := func(start, end int, c color.NRGBA) {
		for start < end {
			for si < len(spans) && spans[si].Range.End <= start {
				si++
			}
			if si == len(spans) || spans[si].Range.Start >= end {
				appendSelected(start, end, c)
				return
			}
			s := spans[si]
			if start < s.Range.Start {
				appendSelected(start, min(end, s.Range.Start), c)
				start = s.Range.Start
			}
			stop := min(end, s.Range.End)
			appendSelected(start, stop, v.highlightColor(s.Kind))
			start = stop
		}
	}
	at := 0
	for _, token := range tokens {
		if token.Start < at || token.End > length || token.End <= token.Start {
			continue
		}
		appendHighlighted(at, token.Start, v.colors.Foreground)
		c, ok := v.colors.Tokens[token.Kind]
		if !ok {
			c = v.colors.Foreground
		}
		appendHighlighted(token.Start, token.End, c)
		at = token.End
	}
	appendHighlighted(at, length, v.colors.Foreground)
	return out
}

func (v *SourceView) pruneLayouts(keep layoutKey) {
	// Two measured widths per text identity bound the extra native working
	// set during continuous resizing. The existing whole-view budgets apply
	// across all widths, rather than separately to every width.
	widths := 0
	for key := range v.layouts {
		if key.sameText(keep) {
			widths++
		}
	}
	bytes := 0
	for _, p := range v.layouts {
		bytes += len(p.text)
	}
	epoch := v.layoutEpoch
	first, last := v.visibleRows()
	width := v.paragraphWidth()
	revealFirst, revealLast, anchor := -1, -1, -1
	if request := v.reveal; request != nil && request.valid(v) {
		row := v.displayLine(v.displayCaret())
		revealFirst = v.paragraphAt(max(0, v.paragraphTop(row)-v.viewport.Height))
		revealLast = v.neighboringRow(row, 1)
	}
	if v.pendingAnchor && v.anchor.valid && v.projection == nil {
		anchor = v.document.model.LineAt(v.clampOffset(v.anchor.offset))
	}
	pinned := func(key layoutKey, p *paragraph) bool {
		if key.width != width || p.row < 0 || p.row >= v.displayCount() {
			return false
		}
		expected := v.paragraphKey(p.row)
		if key != expected && (key.id != 0 || key.revision != v.document.model.Revision() || key.row != p.row) {
			return false // Old revisions/line identities must not be pinned by a reused row number.
		}
		_, shown := v.visibleLines.Index(p.row)
		return shown && (p.row >= first && p.row <= last || p.row >= revealFirst && p.row <= revealLast || p.row == anchor)
	}
	for widths > 2 || len(v.layouts) > maxLayouts || bytes > 8*1024*1024 && len(v.layouts) > 1 {
		var oldest layoutKey
		var found bool
		for key, p := range v.layouts {
			if widths > 2 && !key.sameText(keep) {
				continue
			}
			if pinned(key, p) {
				continue
			}
			if key != keep && (!found || p.used < v.layouts[oldest].used) {
				oldest = key
				found = true
			}
		}
		if !found {
			return
		}
		p := v.layouts[oldest]
		if oldest.sameText(keep) {
			widths--
		}
		bytes -= len(p.text)
		delete(v.layouts, oldest)
		p.ParagraphResources.Destroy()
		if epoch != v.layoutEpoch || v.Destroyed() {
			return
		}
	}
}

func (v *SourceView) resolveLongParagraph(row int, key layoutKey, p *paragraph, style paragraphStyle, tokens []highlight.Token, focus, measure bool) *paragraph {
	state, epoch := v.operationState(), v.layoutEpoch
	if _, failed := v.layoutFailures[key]; failed {
		return nil
	}
	if p == nil {
		for old, candidate := range v.layouts {
			if candidate.Paged != nil && candidate.row == row {
				delete(v.layouts, old)
				p = candidate
				v.layouts[key] = p
				p.replaced = false
				break
			}
		}
	}
	if p == nil {
		if !measure {
			v.requestLayoutWork()
			return nil
		}
		p = &paragraph{row: row, foreground: v.colors.Foreground}
		format := typography.TextFormat{Font: v.font, WrapMode: v.wrap, TextColor: v.colors.Foreground, TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}
		r := v.displayRange(row)
		p.sourceRange = r
		p.Paged = view.NewParagraphLayout(v.context, format, key.width, r.End-r.Start, func(local TextRange) (string, error) {
			r := v.displayRange(p.row)
			local.Start += r.Start
			local.End += r.Start
			if v.projection != nil {
				return v.projection.Slice(v.document.model, local)
			}
			return v.document.model.Slice(local)
		})
		v.layouts[key] = p
	}
	p.Paged.SetWidth(key.width)
	if measure && v.reveal != nil {
		r := v.displayRange(row)
		if p.Paged.GeometryAt(v.displayCaret()-r.Start) == nil {
			v.requestLayoutWork()
		}
	} else if measure && v.pendingAnchor && v.anchor.valid {
		r := v.displayRange(row)
		if v.anchor.offset >= r.Start && v.anchor.offset <= r.End && p.Paged.GeometryAt(v.anchor.offset-r.Start) == nil {
			v.requestLayoutWork()
		}
	}
	viewport := geometry.Rect(v.offset.X, max(0, v.offset.Y-padding-v.paragraphTop(row)), v.textRight(), max(v.lineHeight, v.viewport.Height))
	if v.lineCollapsed(row) {
		viewport.X, viewport.Y, viewport.Height = 0, 0, v.lineHeight
	}
	ready := p.Paged.Prepare(viewport, measure)
	// An unattached/offscreen view has no frame source. Allow a small fixed
	// number of bounded steps, never an unbounded walk to distant text.
	for step := 0; !ready && measure && v.Root() == nil && step < 4; step++ {
		ready = p.Paged.Prepare(viewport, true)
	}
	if !state.valid(v) || epoch != v.layoutEpoch || v.layouts[key] != p {
		return nil
	}
	if err := p.Paged.Err(); err != nil {
		if v.layoutFailures == nil || len(v.layoutFailures) >= maxLayouts {
			v.layoutFailures = make(map[layoutKey]struct{})
		}
		v.layoutFailures[key] = struct{}{}
		v.report(err)
		return nil
	}
	if !ready {
		v.requestLayoutWork()
	}
	frame := p.Paged.Geometry()
	p.Geometry = frame.Geometry
	p.GeometryX, p.GeometryY = frame.X, frame.Y
	size := p.Paged.Size()
	p.width, p.height = size.Width, size.Height
	v.setParagraphHeight(row, p.height)
	v.contentWidth = max(v.contentWidth, p.width)
	r := p.Paged.VisibleRange()
	if !measure && p.textStart != r.Start {
		p.text = ""
	}
	p.textStart = r.Start
	if measure {
		p.text = p.Paged.Text(r)
		if v.whitespace != WhitespaceNone || v.showLineEndings {
			v.refreshWhitespace(p, r.Start, p.Paged.Version())
		}
	} else if (v.whitespace != WhitespaceNone || v.showLineEndings) && (p.whitespace == nil || p.whitespace.TextStart != r.Start || p.whitespace.Version != p.Paged.Version() || p.whitespace.Revision != v.document.model.Revision()) {
		v.requestLayoutWork()
	}
	if !p.styled || p.style != style || p.pageVersion != p.Paged.Version() || p.colorRange != r {
		if !measure {
			v.requestLayoutWork()
		} else if !v.colorParagraph(key, p, style, tokens, focus) {
			return nil
		}
	}
	v.tick++
	p.used = v.tick
	if r.End == 0 {
		return nil
	}
	v.pruneLayouts(key)
	return p
}

// Byte ranges remain model-relative even when the native working set is small.
func (v *SourceView) changeLongParagraphs(c TextChange) {
	for key, p := range v.layouts {
		if p.Paged == nil || c.Reset {
			continue
		}
		r := p.sourceRange
		var local []TextEdit
		invalid := false
		for _, e := range c.Edits {
			if e.Range.End <= r.Start && e.Range.Start < r.Start {
				r.Start = textcore.MapOffset(r.Start, e)
				r.End = textcore.MapOffset(r.End, e)
				continue
			}
			if e.Range.Start > r.End {
				continue
			}
			if e.Range.Start < r.Start || e.Range.End > r.End || strings.Contains(e.Text, "\n") {
				invalid = true
				break
			}
			x := e
			x.Range.Start -= r.Start
			x.Range.End -= r.Start
			local = append(local, x)
			r.End += len(e.Text) - (e.Range.End - e.Range.Start)
		}
		if p.row < 0 || p.row >= v.displayCount() || r != v.displayRange(p.row) {
			invalid = true
		}
		if invalid {
			delete(v.layouts, key)
			p.Paged.Destroy()
			if p.Number != nil {
				p.Number.Destroy()
			}
			continue
		}
		p.sourceRange = r
		if len(local) > 0 {
			p.Paged.Change(r.End-r.Start, local)
			p.styled = false
		}
	}
}

// Keep native and input coordinates local. The subtraction must happen before
// narrowing a document position to float32.
func (v *SourceView) geometryOrigin(row int, x, y float64) geometry.Point {
	return geometry.Point{X: v.gutter + padding + float32(x-float64(v.offset.X)), Y: float32(float64(padding) + v.paragraphTop64(row) + y - float64(v.offset.Y))}
}
func (v *SourceView) paragraphOrigin(row int, p *paragraph) geometry.Point {
	return v.geometryOrigin(row, p.GeometryX, p.GeometryY)
}
func (v *SourceView) paragraphCaret(row int, p *paragraph, pos textcore.Position, cached bool) (geometry.Rectangle, bool) {
	g, origin := &p.Geometry, v.paragraphOrigin(row, p)
	if p.Paged != nil {
		var frame *view.ParagraphGeometry
		if cached {
			frame = p.Paged.CachedGeometryAt(pos.Offset)
		} else {
			frame = p.Paged.GeometryAt(pos.Offset)
		}
		if frame == nil {
			return geometry.Rectangle{}, false
		}
		g, origin = &frame.Geometry, v.geometryOrigin(row, frame.X, frame.Y)
	}
	return g.Caret(pos).Translate(origin), true
}
