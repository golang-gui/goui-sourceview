package sourceview

import (
	"image/color"
	"slices"
	"strconv"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
)

const padding float32 = 6
const maxLayouts = 128

type viewportAnchor struct {
	offset   int
	inset, x float32
	valid    bool
}

type layoutKey struct {
	id, revision uint64
	row          int
}
type paragraph struct {
	layout, number typography.TextLayout
	geometry       textedit.Geometry
	text           string
	width, height  float32
	style          paragraphStyle
	styled         bool
	used           uint64
	numberText     string
}

// Paint-only state is deliberately separate from the native layout's text,
// font, tab and wrapping identity.
type paragraphStyle struct {
	scheme       *scheme.Scheme
	language     language.ID
	tokenVersion uint64
	tokenValid   bool
	selection    textedit.Range
	focused      bool
}

func (v *SourceView) dropLayouts() {
	v.layoutEpoch++
	old := v.layouts
	v.layouts = make(map[layoutKey]*paragraph)
	for _, p := range old {
		p.layout.Destroy()
		if p.number != nil {
			p.number.Destroy()
		}
	}
}
func (v *SourceView) invalidateLayout() {
	if v.mounted && v.anchor.valid && v.anchorRevision == v.document.model.Revision() {
		v.pendingAnchor = true
	}

	v.dropLayouts()
	v.heights.Reset(v.displayCount(), v.lineHeight)
	v.contentWidth = 0
	v.RequestLayout()
	v.RequestPaint()
}
func (v *SourceView) StyleChanged() {
	v.closeCompletion()
	if v.Destroyed() {
		return
	}
	v.updateScheme()
	if v.Destroyed() {
		return
	}
	if !gui.IsEnabled(v) {
		v.cancelPreedit()
		if v.Destroyed() {
			return
		}
		v.cancelPaste()
		v.hideMenu()
		if v.Destroyed() {
			return
		}
	}
	// Read style and enabled state after teardown callbacks, using the view's
	// current tree location rather than a captured document/selection state.
	name := v.StyleName()
	if name == "" {
		name = "source-view"
	}
	state := style.Normal
	if !gui.IsEnabled(v) {
		state = style.Disabled
	}
	resolved := gui.ResolveWidgetStyle(v, name, "", state)
	font := typography.FontInfo{Family: "monospace", Size: 12}
	if family, ok := resolved.FontFamily(); ok {
		font.Family = family
	}
	if size, ok := resolved.FontSize(); ok {
		font.Size = size
	}
	if v.font == font {
		return
	}
	v.font = font
	v.metricsReady = false
	v.contentWidth = 0
	if v.anchor.valid {
		v.pendingAnchor = true
	}
	v.dropLayouts()
	v.heights.Reset(v.displayCount(), v.lineHeight)
}
func (v *SourceView) ensureMetrics() {
	if v.context == nil || v.metricsReady {
		return
	}
	t, err := v.context.NewTextLayout("0000000000", typography.TextFormat{Font: v.font}, 1<<20, 1<<20)
	if err != nil {
		v.report(err)
		return
	}
	w, h := t.MeasureSize()
	t.Destroy()
	v.digitWidth = max(1, w/10)
	v.spaceWidth = v.digitWidth
	spaces, err := v.context.NewTextLayout(" x", typography.TextFormat{Font: v.font}, 1<<20, 1<<20)
	if err == nil {
		_, clusters := spaces.MeasureMetrics()
		for _, c := range clusters {
			if c.Start == 0 && c.Length == 1 && c.Width > 0 {
				v.spaceWidth = c.Width
				break
			}
		}
		spaces.Destroy()
	}
	v.lineHeight = max(1, h)
	v.metricsReady = true
	v.heights.Reset(v.displayCount(), v.lineHeight)
}
func (v *SourceView) updateGutter() {
	old := v.gutter
	v.gutter = 0
	if v.showLineNumbers {
		v.gutter = float32(max(2, len(strconv.Itoa(v.document.model.LineCount()))))*max(v.spaceWidth, v.digitWidth) + 2*padding
	}
	if old != v.gutter && v.wrap != WrapNone {
		v.dropLayouts()
		v.heights.Reset(v.displayCount(), v.lineHeight)
	}
}
func (v *SourceView) Measure(c layout.Constraint) layout.Measurement {
	if !v.Visible() {
		return layout.Measurement{}
	}
	v.ensureMetrics()
	v.updateGutter()
	size := geometry.Size{Width: 640, Height: 320}
	size.Width = max(size.Width, v.MinSize().Width)
	size.Height = max(size.Height, v.MinSize().Height)
	if v.MaxSize().Width > 0 {
		size.Width = min(size.Width, v.MaxSize().Width)
	}
	if v.MaxSize().Height > 0 {
		size.Height = min(size.Height, v.MaxSize().Height)
	}
	return layout.Measured(c.Clamp(size))
}
func (v *SourceView) Arrange(rect geometry.Rectangle) {
	v.WidgetBase.Arrange(rect)
	v.LayoutVisible(rect.Size, v.offset)
}
func (v *SourceView) ContentSize() geometry.Size {
	v.ensureHeights()
	width := v.gutter + 2*padding + v.contentWidth
	if v.wrap != WrapNone {
		width = v.viewport.Width
	}
	return geometry.Size{Width: max(v.viewport.Width, width), Height: 2*padding + v.heights.Total()}
}
func (v *SourceView) ensureHeights() {
	if v.heights.Count() != v.displayCount() {
		v.heights.Reset(v.displayCount(), v.lineHeight)
	}
}
func (v *SourceView) LayoutVisible(viewport geometry.Size, offset geometry.Point) {
	if v.viewport != viewport || v.offset != offset {
		v.closeCompletion()
	}
	if v.viewport.Width != viewport.Width && v.wrap != WrapNone {
		v.dropLayouts()
		v.heights.Reset(v.displayCount(), v.lineHeight)
		v.contentWidth = 0
	}
	v.viewport = viewport
	v.offset = offset
	v.ensureMetrics()
	v.updateGutter()
	v.ensureHeights()
	if v.document.model.Revision() != v.observed && v.mounted {
		return
	}
	// Measure only the visible paragraphs. Updated estimates request a second
	// ScrollView layout; they never recurse into the parent.
	old := v.ContentSize()
	first, last := v.visibleRows()
	for row := first; row <= last; row++ {
		v.paragraph(row)
	}
	if v.ContentSize() != old {
		v.RequestLayout()
	}
	if v.pendingAnchor && v.projection == nil && v.anchor.valid {
		v.pendingAnchor = false
		at := v.clampOffset(v.anchor.offset)
		row := v.document.model.LineAt(at)
		r, _ := v.document.model.LineRange(row)
		if p := v.paragraph(row); p != nil {
			rect := p.geometry.Caret(textedit.Position{Offset: at - r.Start})
			y := max(0, min(padding+v.heights.Top(row)+rect.Y+v.anchor.inset, max(0, v.ContentSize().Height-viewport.Height)))
			v.offset = geometry.Point{X: v.anchor.x, Y: y}
			v.scrollSignal.Emit(geometry.Rect(v.anchor.x, y, viewport.Width, viewport.Height))
		}
	} else {
		v.captureAnchor()
	}
	v.updateIMCaret()
}
func (v *SourceView) visibleRows() (int, int) {
	v.ensureHeights()
	first := v.heights.At(max(0, v.offset.Y-padding))
	last := v.heights.At(max(0, v.offset.Y+v.viewport.Height-padding))
	return max(0, first), min(v.displayCount()-1, max(first, last)+1)
}
func (v *SourceView) paragraph(row int) *paragraph {
	if v.context == nil || row < 0 || row >= v.displayCount() {
		return nil
	}
	if v.mounted && v.observed != v.document.model.Revision() {
		return nil
	}
	v.ensureHeights()
	r := v.displayRange(row)
	key := layoutKey{revision: v.document.model.Revision(), row: row}
	e := v.document.engine
	tokenRow, hasTokens := v.highlightRow(row)
	if hasTokens && e != nil {
		if line := e.Line(tokenRow); line != nil {
			key = layoutKey{id: line.ID}
		}
	}
	var version uint64
	valid := false
	if hasTokens && e != nil {
		_, version, valid = e.Tokens(tokenRow)
		if !valid {
			e.Ensure(tokenRow)
			v.document.requestWork()
		}
	}
	sel := textedit.Range{}
	if v.projection == nil {
		selection := v.Selection().Range()
		sel = textedit.Range{Start: max(0, min(r.End, selection.Start)-r.Start), End: max(0, min(r.End, selection.End)-r.Start)}
	}
	style := paragraphStyle{scheme: v.appliedScheme, language: v.document.language, tokenVersion: version, tokenValid: valid, selection: sel, focused: activeFocus(v)}
	var tokens []highlight.Token
	if valid {
		tokens, _, _ = e.Tokens(tokenRow)
	}
	p := v.layouts[key]
	if p == nil && key.id != 0 {
		// Initial lexical indexing supplies stable IDs after a plain first
		// frame. The revision/row entry already has exactly this text.
		fallback := layoutKey{revision: v.document.model.Revision(), row: row}
		p = v.layouts[fallback]
		if p != nil {
			delete(v.layouts, fallback)
			v.layouts[key] = p
		}
	}
	v.tick++
	if p != nil {
		if (!p.styled || p.style != style) && !v.colorParagraph(key, p, style, tokens) {
			return nil
		}
		p.used = v.tick
		v.heights.Set(row, p.height)
		v.contentWidth = max(v.contentWidth, p.width)
		return p
	}
	text := v.displayText(row)
	width := float32(1 << 20)
	if v.wrap != WrapNone {
		width = max(1, v.viewport.Width-v.gutter-2*padding)
	}
	format := typography.TextFormat{Font: v.font, WrapMode: v.wrap, TextColor: v.colors.Foreground, TabWidth: float32(v.indent.TabWidth) * v.spaceWidth}
	state, epoch := v.operationState(), v.layoutEpoch
	native, err := v.context.NewTextLayout(text, format, width, 1<<24)
	if err != nil {
		v.report(err)
		return nil
	}
	if !state.valid(v) || epoch != v.layoutEpoch {
		native.Destroy()
		return nil
	}
	p = &paragraph{layout: native, text: text, used: v.tick}
	if !v.colorParagraph(key, p, style, tokens) {
		native.Destroy() // Not cached yet: the local call still owns this object.
		return nil
	}
	w, h := native.MeasureSize()
	lines, clusters := native.MeasureMetrics()
	g := textedit.NewGeometry(lines, clusters, len(text), v.lineHeight)
	p.geometry, p.width, p.height = g, w, max(v.lineHeight, h)
	oldSize := v.ContentSize()
	v.layouts[key] = p
	v.heights.Set(row, p.height)
	v.contentWidth = max(v.contentWidth, w)
	if v.ContentSize() != oldSize {
		v.RequestLayout()
	}
	v.pruneLayouts(key)
	return p
}

func (v *SourceView) colorParagraph(key layoutKey, p *paragraph, style paragraphStyle, tokens []highlight.Token) bool {
	state, epoch := v.operationState(), v.layoutEpoch
	owner := v.layouts[key]
	valid := func() bool {
		return state.valid(v) && epoch == v.layoutEpoch && v.layouts[key] == owner && v.appliedScheme == style.scheme && v.document.language == style.language && activeFocus(v) == style.focused
	}
	if !valid() {
		return false
	}
	// Complete replacement clears obsolete tokens and the previous selection.
	// Full-range overwrite also bounds the backend's stored color ranges.
	if len(p.text) > 0 {
		p.layout.SetTextColor(0, len(p.text), v.colors.Foreground)
		if !valid() {
			return false
		}
	}
	for _, token := range tokens {
		if token.Start < 0 || token.End > len(p.text) || token.End <= token.Start {
			continue
		}
		c, ok := v.colors.Tokens[token.Kind]
		if !ok {
			c = v.colors.Foreground
		}
		p.layout.SetTextColor(token.Start, token.End-token.Start, c)
		if !valid() {
			return false
		}
	}
	if style.focused && style.selection.End > style.selection.Start {
		p.layout.SetTextColor(style.selection.Start, style.selection.End-style.selection.Start, v.colors.SelectionForeground)
		if !valid() {
			return false
		}
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
func (v *SourceView) pruneLayouts(keep layoutKey) {
	bytes := 0
	for _, p := range v.layouts {
		bytes += len(p.text)
	}
	for len(v.layouts) > maxLayouts || bytes > 8*1024*1024 && len(v.layouts) > 1 {
		var oldest layoutKey
		var found bool
		for key, p := range v.layouts {
			if key != keep && (!found || p.used < v.layouts[oldest].used) {
				oldest = key
				found = true
			}
		}
		if !found {
			return
		}
		p := v.layouts[oldest]
		bytes -= len(p.text)
		delete(v.layouts, oldest)
		p.layout.Destroy()
		if p.number != nil {
			p.number.Destroy()
		}
	}
}
func (v *SourceView) Paint(p gui.Painter) {
	bounds := geometry.Rect(0, 0, v.Rect().Width, v.Rect().Height)
	p.FillRect(bounds, graphics.ColorOf(v.colors.Background))
	if v.context == nil || v.mounted && v.observed != v.document.model.Revision() {
		return
	}
	v.ensureMetrics()
	v.updateGutter()
	first, last := v.visibleRows()
	p.Save()
	p.SetClipRect(geometry.Rect(v.gutter, 0, max(0, bounds.Width-v.gutter), bounds.Height))
	caretRow := v.displayLine(v.displayCaret())
	selection := v.Selection().Range()
	var brackets []textedit.Range
	if activeFocus(v) && (v.colors.BracketMatchBackground.A != 0 || v.colors.BracketMatchBorder.A != 0) {
		brackets = v.matchingBrackets()
	}
	for row := first; row <= last; row++ {
		item := v.paragraph(row)
		if item == nil {
			continue
		}
		r := v.displayRange(row)
		origin := geometry.Point{X: v.gutter + padding - v.offset.X, Y: padding + v.heights.Top(row) - v.offset.Y}
		if activeFocus(v) && row == caretRow {
			p.FillRect(geometry.Rect(v.gutter, origin.Y, max(0, bounds.Width-v.gutter), item.height), graphics.ColorOf(v.colors.CurrentLine))
		}
		if v.projection == nil {
			// Put the quiet fill below search/selection, and the outline above
			// those backgrounds. Neither pass changes syntax foregrounds.
			bracketRects := bracketRectangles(item, r, origin, brackets)
			if c := v.colors.BracketMatchBackground; c.A != 0 {
				for _, rect := range bracketRects {
					p.FillRect(rect, graphics.ColorOf(c))
				}
			}
			for _, match := range v.search.InRange(v.document.model, r, 2000) {
				c := v.colors.SearchBackground
				if v.currentMatch != nil && v.currentMatch.raw == match {
					c = v.colors.CurrentSearchBackground
				}
				v.paintRange(p, item, r, origin, match, c)
			}
			c := v.colors.InactiveSelectionBackground
			if activeFocus(v) {
				c = v.colors.SelectionBackground
			}
			v.paintRange(p, item, r, origin, selection, c)
			if c := v.colors.BracketMatchBorder; c.A != 0 {
				for _, rect := range bracketRects {
					// Strokes are centered on the path. Keep the 1-DIP border
					// inside its character cell, including at viewport edges.
					p.DrawRect(geometry.Rect(rect.X+.5, rect.Y+.5, rect.Width-1, rect.Height-1), 1, graphics.ColorOf(c))
				}
			}
			if selection.Start <= r.End && selection.End > r.End && row < v.displayCount()-1 {
				edge := item.geometry.Caret(textedit.Position{Offset: len(item.text), Upstream: true})
				p.FillRect(geometry.Rect(origin.X+edge.X, origin.Y+edge.Y, v.spaceWidth, edge.Height), graphics.ColorOf(c))
			}
		}
		p.DrawTextLayout(origin, item.layout)
		if v.projection != nil {
			start := v.projection.Replacement().Start
			end := start + len(v.projection.Text())
			local := textedit.Range{Start: max(r.Start, start) - r.Start, End: min(r.End, end) - r.Start}
			if local.End > local.Start {
				for _, rect := range item.geometry.Selection(local) {
					y := origin.Y + rect.Y + rect.Height - 1
					p.DrawLine(geometry.Point{X: origin.X + rect.X, Y: y}, geometry.Point{X: origin.X + rect.X + rect.Width, Y: y}, 1, graphics.ColorOf(v.colors.PreeditUnderline))
				}
			}
		}
	}
	if activeFocus(v) && v.caretOn && gui.IsEnabled(v) {
		if r, ok := v.caretRect(); ok {
			p.FillRect(r, graphics.ColorOf(v.colors.Caret))
			v.im.SetCaretRect(r)
		}
	}
	p.Restore()
	if v.gutter > 0 {
		p.Save()
		p.SetClipRect(geometry.Rect(0, 0, v.gutter, bounds.Height))
		p.FillRect(geometry.Rect(0, 0, v.gutter, bounds.Height), graphics.ColorOf(v.colors.GutterBackground))
		for row := first; row <= last; row++ {
			item := v.paragraph(row)
			if item == nil {
				continue
			}
			number := row + 1
			if v.projection != nil {
				number = v.document.model.LineAt(v.projection.ModelOffset(v.displayRange(row).Start, false)) + 1
			}
			numberText := strconv.Itoa(number)
			if item.number == nil || item.numberText != numberText {
				if item.number != nil {
					item.number.Destroy()
					item.number = nil
				}
				native, err := v.context.NewTextLayout(numberText, typography.TextFormat{Font: v.font, TextColor: v.colors.GutterForeground}, 1<<16, 1<<16)
				if err != nil {
					continue
				}
				item.number = native
				item.numberText = numberText
			}
			w, _ := item.number.MeasureSize()
			p.DrawTextLayout(geometry.Point{X: v.gutter - padding - w, Y: padding + v.heights.Top(row) - v.offset.Y}, item.number)
		}
		p.Restore()
		p.DrawLine(geometry.Point{X: v.gutter - .5}, geometry.Point{X: v.gutter - .5, Y: bounds.Height}, 1, graphics.ColorOf(v.colors.Border))
	}
}

// Only a confirmed pair receives matching decoration. A lone candidate may be
// unmatched or simply have its partner beyond the bounded analysis window.
func bracketRectangles(item *paragraph, line textedit.Range, origin geometry.Point, brackets []textedit.Range) []geometry.Rectangle {
	if len(brackets) != 2 {
		return nil
	}
	var result []geometry.Rectangle
	for _, bracket := range brackets {
		if bracket.End <= line.Start || bracket.Start >= line.End {
			continue
		}
		local := textedit.Range{Start: max(line.Start, bracket.Start) - line.Start, End: min(line.End, bracket.End) - line.Start}
		for _, rect := range item.geometry.Selection(local) {
			// Native visual-line geometry keeps wrapped/bidi positions exact.
			// A small vertical inset separates the box from neighboring rows.
			if rect.Width <= 1 || rect.Height <= 3 {
				continue
			}
			rect.Y++
			rect.Height -= 2
			rect = rect.Translate(origin)
			// A shaping cluster may contain both characters. Paint its shared
			// rectangle once so translucent decorations do not accumulate.
			if !slices.Contains(result, rect) {
				result = append(result, rect)
			}
		}
	}
	return result
}
func (v *SourceView) paintRange(p gui.Painter, item *paragraph, line textedit.Range, origin geometry.Point, r textedit.Range, c color.NRGBA) {
	start, end := max(line.Start, r.Start)-line.Start, min(line.End, r.End)-line.Start
	if end <= start {
		return
	}
	for _, rect := range item.geometry.Selection(textedit.Range{Start: start, End: end}) {
		p.FillRect(rect.Translate(origin), graphics.ColorOf(c))
	}
}
func (v *SourceView) snapOffset(offset int, forward bool) int {
	row := v.document.model.LineAt(offset)
	r, _ := v.document.model.LineRange(row)
	if p := v.paragraph(row); p != nil {
		return r.Start + p.geometry.Snap(offset-r.Start, forward)
	}
	return v.clampOffset(offset)
}
func (v *SourceView) caretRect() (geometry.Rectangle, bool) {
	caret := v.displayCaret()
	row := v.displayLine(caret)
	r := v.displayRange(row)
	p := v.paragraph(row)
	if p == nil {
		return geometry.Rectangle{}, false
	}
	rect := p.geometry.Caret(textedit.Position{Offset: caret - r.Start, Upstream: v.upstream})
	return rect.Translate(geometry.Point{X: v.gutter + padding - v.offset.X, Y: padding + v.heights.Top(row) - v.offset.Y}), true
}
func (v *SourceView) updateIMCaret() {
	if r, ok := v.caretRect(); ok {
		v.im.SetCaretRect(r)
	}
}
func (v *SourceView) RevealSelection() {
	rect, ok := v.caretRect()
	if !ok {
		return
	}
	// ScrollView receives content coordinates. Include the fixed gutter in the
	// requested width so a caret at its leading edge remains visible.
	rect.X += v.offset.X - v.gutter
	rect.Y += v.offset.Y
	rect.Width += v.gutter + padding
	v.scrollSignal.Emit(rect)
}
func (v *SourceView) hit(point geometry.Point) textedit.Position {
	row := v.heights.At(max(0, point.Y+v.offset.Y-padding))
	r := v.displayRange(row)
	pos := textedit.Position{Offset: r.Start}
	if p := v.paragraph(row); p != nil {
		pos = p.geometry.Hit(geometry.Point{X: point.X - v.gutter - padding + v.offset.X, Y: point.Y + v.offset.Y - padding - v.heights.Top(row)})
		pos.Offset += r.Start
	}
	if v.projection != nil {
		pos.Offset = v.projection.ModelOffset(pos.Offset, pos.Upstream)
	}
	return pos
}
func (v *SourceView) Snapshot() gui.WidgetInfo {
	info := v.WidgetBase.Snapshot()
	info.Role = gui.Role("sourceview")
	info.Text = ""
	info.SetAttribute("goui.sourceview", map[string]any{"language": string(v.document.Language()), "revision": v.document.model.Revision(), "wrap": int(v.wrap), "lineNumbers": v.showLineNumbers})
	m := v.document.model
	if v.mounted && v.observed != m.Revision() {
		return info
	}
	first, last := v.visibleRows()
	if v.projection != nil {
		first = m.LineAt(v.projection.ModelOffset(v.displayRange(first).Start, false))
		last = m.LineAt(v.projection.ModelOffset(v.displayRange(last).End, true))
	}
	r, _ := m.LineRange(first)
	end, _ := m.LineRange(last)
	r.End = min(end.End, r.Start+64*1024)
	for r.End > r.Start && !m.IsRuneBoundary(r.End) {
		r.End--
	}
	text, _ := m.Slice(r)
	info.TextEditing = &gui.TextEditingInfo{ReadOnly: v.readOnly, Selection: v.Selection(), Length: m.Len(), VisibleRange: r, VisibleText: text}
	if v.projection != nil {
		text := v.projection.Text()
		limit := textedit.ClampOffset(text, min(len(text), 64*1024))
		info.TextEditing.Preedit = &gui.TextPreeditInfo{Replacement: v.projection.Replacement(), Text: text[:limit], Caret: v.projection.Caret(), Length: len(text)}
	}
	return info
}

// captureAnchor records committed byte coordinates, independent of pixel estimates.
func (v *SourceView) captureAnchor() {
	if v.projection != nil || !v.mounted || v.observed != v.document.model.Revision() {
		return
	}
	row := v.heights.At(max(0, v.offset.Y-padding))
	r := v.displayRange(row)
	p := v.paragraph(row)
	if p == nil {
		return
	}
	y := max(0, v.offset.Y-padding-v.heights.Top(row))
	pos := p.geometry.Hit(geometry.Point{Y: y})
	index := p.geometry.LineIndex(pos)
	edge := p.geometry.LineEdge(index, false)
	v.anchor = viewportAnchor{offset: r.Start + edge.Offset, inset: v.offset.Y - padding - v.heights.Top(row) - p.geometry.LineMetrics(index).Y, x: v.offset.X, valid: true}
	v.anchorRevision = v.document.model.Revision()
}
