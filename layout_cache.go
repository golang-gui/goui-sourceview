package sourceview

import (
	"image/color"
	"slices"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
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
	layout, number typography.TextLayout
	whitespace     *whitespaceLayout
	geometry       textedit.Geometry
	fragment       textedit.Fragment
	text           string
	width, height  float32
	style          paragraphStyle
	styled         bool
	replaced       bool
	foreground     color.NRGBA
	colors         []paragraphColor
	used           uint64
	row            int
	numberText     string
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
	scheme        *scheme.Scheme
	language      language.ID
	tokenVersion  uint64
	languageEpoch uint64
	tokenValid    bool
	selection     textedit.Range
	focused       bool
}

func (v *SourceView) dropLayouts() {
	v.layoutEpoch++
	v.layoutPending = false
	v.layoutFailures = nil
	old := v.layouts
	v.layouts = make(map[layoutKey]*paragraph)
	for _, p := range old {
		p.layout.Destroy()
		if p.number != nil {
			p.number.Destroy()
		}
	}
}

// Width is part of measurement identity. Keep native resources and their
// immutable metrics together so returning to a recent width needs no shaping.
// Font, tabs, wrapping mode, projection and Context changes still drop layouts.
func (v *SourceView) paragraphWidth() float32 {
	if v.wrap == WrapNone {
		return 1 << 20
	}
	return max(1, v.viewport.Width-v.gutter-2*padding)
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
	if _, visible := v.visibleLines.index(row); !visible {
		return nil
	}
	r := v.displayRange(row)
	key := v.paragraphKey(row)
	e := v.document.engine
	tokenRow, hasTokens := v.highlightRow(row)
	var version uint64
	valid := false
	if hasTokens && e != nil {
		_, version, valid = e.Tokens(tokenRow)
		if !valid {
			e.Ensure(tokenRow)
			v.document.requestWork()
		}
	}
	focus := activeFocus(v)
	sel := textedit.Range{}
	if v.colors.SelectionForeground.A == 255 && focus && v.projection == nil {
		selection := v.Selection().Range()
		sel = textedit.Range{Start: max(0, min(r.End, selection.Start)-r.Start), End: max(0, min(r.End, selection.End)-r.Start)}
		if sel.Start == sel.End {
			sel = textedit.Range{}
		}
	}
	// Selection geometry is painted independently. Only an actual foreground
	// override makes selection/focus part of the native text's color identity.
	style := paragraphStyle{scheme: v.appliedScheme, language: v.document.language, languageEpoch: v.document.languageEpoch, tokenVersion: version, tokenValid: valid, selection: sel, focused: sel.End > sel.Start}
	var tokens []highlight.Token
	if valid {
		tokens, _, _ = e.Tokens(tokenRow)
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
	p = &paragraph{layout: native, text: text, used: v.tick, row: row, foreground: v.colors.Foreground}
	if !v.colorParagraph(key, p, style, tokens, focus) {
		native.Destroy() // Not cached yet: the local call still owns this object.
		return nil
	}
	w, h := native.MeasureSize()
	lines, clusters := native.MeasureMetrics()
	p.whitespace = newWhitespaceLayout(text, lines, clusters)
	var g textedit.Geometry
	fragment, fragmentErr := textedit.NewFragment(textedit.Range{End: len(text)}, geometry.Size{Width: w, Height: h}, lines, clusters)
	if fragmentErr == nil {
		g = fragment.Geometry(v.lineHeight)
	} else {
		// Preserve Geometry's general semantics for nonpartitioning metrics.
		g = textedit.NewGeometry(lines, clusters, len(text), v.lineHeight)
	}
	p.fragment = fragment
	p.geometry, p.width, p.height = g, w, max(v.lineHeight, h)
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
		return state.valid(v) && epoch == v.layoutEpoch && v.layouts[key] == owner && v.appliedScheme == style.scheme && v.document.language == style.language && activeFocus(v) == focus
	}
	if !valid() {
		return false
	}
	colors := v.paragraphColors(len(p.text), style, tokens)
	if p.foreground != v.colors.Foreground || !slices.Equal(p.colors, colors) {
		// Complete replacement clears obsolete tokens/selection and bounds
		// backend attributes. A fresh layout already has TextFormat's base.
		if len(p.text) > 0 && (p.foreground != v.colors.Foreground || len(p.colors) > 0) {
			p.layout.SetTextColor(0, len(p.text), v.colors.Foreground)
			if !valid() {
				return false
			}
		}
		for _, c := range colors {
			p.layout.SetTextColor(c.start, c.end-c.start, c.color)
			if !valid() {
				return false
			}
		}
		p.foreground, p.colors = v.colors.Foreground, colors
	}
	if p.number != nil && (!p.styled || p.style.scheme != style.scheme) {
		p.number.SetTextColor(0, len(p.numberText), v.colors.GutterForeground)
		if !valid() {
			return false
		}
	}
	p.style, p.styled = style, true
	return true
}

func (v *SourceView) paragraphColors(length int, style paragraphStyle, tokens []highlight.Token) []paragraphColor {
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
	sel := textedit.Range{}
	if v.colors.SelectionForeground.A == 255 && style.focused {
		sel = style.selection
	}
	appendSelected := func(start, end int, c color.NRGBA) {
		if sel.Start < end && sel.End > start {
			appendRange(start, min(end, sel.Start), c)
			appendRange(max(start, sel.Start), min(end, sel.End), v.colors.SelectionForeground)
			appendRange(max(start, sel.End), end, c)
		} else {
			appendRange(start, end, c)
		}
	}
	at := 0
	for _, token := range tokens {
		if token.Start < at || token.End > length || token.End <= token.Start {
			continue
		}
		appendSelected(at, token.Start, v.colors.Foreground)
		c, ok := v.colors.Tokens[token.Kind]
		if !ok {
			c = v.colors.Foreground
		}
		appendSelected(token.Start, token.End, c)
		at = token.End
	}
	appendSelected(at, length, v.colors.Foreground)
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
		_, shown := v.visibleLines.index(p.row)
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
		p.layout.Destroy()
		if p.number != nil {
			p.number.Destroy()
		}
		if epoch != v.layoutEpoch || v.Destroyed() {
			return
		}
	}
}
