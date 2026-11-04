package sourceview

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/graphics"
)

// MinimapOptions controls the bounded color-block overview, not text layout.
// Zero fields select defaults: 120 DIP wide and at most 120 sampled columns.
type MinimapOptions struct {
	Width      float32
	MaxColumns int
}

func (v *SourceView) ShowMinimap() bool { return v.showMinimap }
func (v *SourceView) SetShowMinimap(show bool) {
	if v.showMinimap == show {
		return
	}
	oldWidth := v.minimapWidth()
	v.showMinimap = show
	v.minimapChanged(oldWidth)
}
func (v *SourceView) MinimapOptions() MinimapOptions {
	o := v.minimapOptions
	if o.Width == 0 {
		o.Width = 120
	}
	if o.MaxColumns == 0 {
		o.MaxColumns = 120
	}
	return o
}
func (v *SourceView) SetMinimapOptions(o MinimapOptions) error {
	if math.IsNaN(float64(o.Width)) || math.IsInf(float64(o.Width), 0) || o.Width < 0 || o.Width > 320 || o.Width > 0 && o.Width < 40 || o.MaxColumns < 0 || o.MaxColumns > 256 {
		return fmt.Errorf("sourceview: minimap width must be 40..320 DIP and columns 1..256, or zero for defaults")
	}
	if v.minimapOptions != o {
		oldWidth := v.minimapWidth()
		v.minimapOptions = o
		v.minimapChanged(oldWidth)
	}
	return nil
}

// Width is already part of paragraph cache identity. Preserve old layouts and
// height estimates just as an ordinary viewport resize does; sampling columns
// alone must never invalidate text geometry.
func (v *SourceView) minimapChanged(oldWidth float32) {
	v.minimap = nil
	if oldWidth != v.minimapWidth() {
		v.layoutEpoch++
		if v.wrap != WrapNone {
			v.contentWidth = 0
		}
		v.RequestLayout()
	}
	v.RequestPaint()
}

// The body and its companions share one right edge. A narrow view leaves
// space for the gutter and at least 80 DIP of editable text before the map.
func (v *SourceView) minimapWidth() float32 {
	if !v.showMinimap {
		return 0
	}
	return v.minimapWidthAt(v.viewport.Width)
}
func (v *SourceView) minimapWidthAt(width float32) float32 {
	if !v.showMinimap {
		return 0
	}
	return min(v.MinimapOptions().Width, max(0, width-v.gutter-80))
}
func (v *SourceView) textRight() float32 { return v.viewport.Width - v.minimapWidth() }
func (v *SourceView) minimapRect() geometry.Rectangle {
	return geometry.Rect(v.textRight(), 0, v.minimapWidth(), v.viewport.Height)
}
func (v *SourceView) minimapHit(p geometry.Point) bool {
	r := v.minimapRect()
	return r.Width > 0 && p.X >= r.X && p.X < r.X+r.Width && p.Y >= 0 && p.Y < r.Height
}

type minimapGeneration struct {
	document                     *Document
	revision, geometry, language uint64
	highlights, highlightStyle   uint64
	scheme                       *scheme.Scheme
	width, height, lineHeight    float32
	columns, tab                 int
}
type minimapRowKey struct {
	id, version, language      uint64
	highlights, highlightStyle uint64
	start, columns, tab        int
	scheme                     *scheme.Scheme
}
type minimapState struct {
	generation                                   minimapGeneration
	pixels                                       *image.NRGBA
	cache                                        map[minimapRowKey][]color.NRGBA
	next                                         int
	refresh                                      bool
	total, extent                                float32
	dragging                                     bool
	dragTotal, dragHeight, dragThumb, dragOffset float32
}

// Work is per view and contains ordinary values only. No native layout is
// created, and the sampling budget is independent of document size.
func (v *SourceView) prepareMinimap() {
	if !v.showMinimap || v.minimapWidth() <= 0 || v.viewport.Height <= 0 {
		v.minimap = nil
		return
	}
	v.ensureHeights()
	g := minimapGeneration{v.document, v.document.model.Revision(), v.minimapGeometry, v.document.languageEpoch, v.document.highlightEpoch, v.highlightStyleEpoch, v.appliedScheme, v.minimapWidth(), v.viewport.Height, v.lineHeight, v.MinimapOptions().MaxColumns, v.indent.TabWidth}
	s := v.minimap
	if s == nil {
		s = &minimapState{cache: make(map[minimapRowKey][]color.NRGBA)}
		v.minimap = s
	}
	if s.generation == g {
		return
	}
	if s.generation.document != g.document || s.generation.scheme != g.scheme || s.generation.language != g.language {
		clear(s.cache)
	}
	if s.generation.revision != g.revision || s.generation.width != g.width || s.generation.height != g.height {
		s.dragging = false
	}
	s.total = max(v.viewport.Height, v.heights.Total()+2*padding)
	s.extent = min(v.viewport.Height, max(2, (v.heights.Total()+2*padding)/max(1, v.lineHeight)*2))
	w, h := g.columns, min(2048, max(1, int(math.Ceil(float64(s.extent)))))
	// Preserve the current document overview while replacing sampled rows. A
	// different document, appearance or raster size must start empty.
	if s.pixels == nil || s.pixels.Rect.Dx() != w || s.pixels.Rect.Dy() != h || s.generation.document != g.document || s.generation.scheme != g.scheme || s.generation.language != g.language {
		s.pixels = image.NewNRGBA(image.Rect(0, 0, w, h))
	}
	s.next, s.refresh, s.generation = 0, false, g
}
func (v *SourceView) minimapPending() bool {
	s := v.minimap
	return s != nil && (s.next < (s.pixels.Rect.Dy()+1)/2 || s.refresh)
}
func (v *SourceView) minimapStep(deadline time.Time) {
	s := v.minimap
	if s == nil || v.projection != nil {
		return
	}
	count := (s.pixels.Rect.Dy() + 1) / 2
	if s.next == count && s.refresh {
		s.next, s.refresh = 0, false
	}
	changed := false
	for n := 0; s.next < count && n < 16 && time.Now().Before(deadline); n++ {
		y := s.next * 2
		contentY := float32(y)/float32(s.pixels.Rect.Dy())*(v.heights.Total()+2*padding) - padding
		row := v.paragraphAt(max(0, contentY))
		r, err := v.document.model.LineRange(row)
		if err != nil {
			s.next++
			continue
		}
		index, _ := v.visibleLines.index(row)
		top, bottom := v.heights.Top(index), v.heights.Top(index+1)
		start := 0
		if v.wrap != WrapNone && bottom-top > v.lineHeight*1.5 {
			// Approximate the wrapped position with the existing paragraph height.
			// This is a thumbnail sample, never a source of caret or wrap geometry.
			fraction := max(0, min(1, (contentY-top)/max(1, bottom-top)))
			start = min(r.End-r.Start, int(float64(r.End-r.Start)*float64(fraction)))
		}
		for start > 0 && !v.document.model.IsRuneBoundary(r.Start+start) {
			start--
		}
		pixels := v.minimapRow(row, r, start, s)
		for x, c := range pixels {
			if s.pixels.NRGBAAt(x, y) != c {
				s.pixels.SetNRGBA(x, y, c)
				changed = true
			}
		}
		s.next++
	}
	if changed {
		v.RequestPaint()
	}
}
func (v *SourceView) minimapRow(row int, r textedit.Range, start int, s *minimapState) []color.NRGBA {
	e := v.document.engine
	key := minimapRowKey{highlights: v.document.highlightEpoch, highlightStyle: v.highlightStyleEpoch, start: start, columns: s.generation.columns, tab: max(1, v.indent.TabWidth), scheme: v.appliedScheme, language: v.document.languageEpoch}
	if e != nil && e.Ready() {
		if line := e.Line(row); line != nil {
			key.id = line.ID
		}
		_, key.version, _ = e.PaintTokens(row)
	}
	if key.id != 0 {
		if pixels, ok := s.cache[key]; ok {
			return pixels
		}
	}
	end := min(r.End, r.Start+start+4096)
	for end > r.Start+start && !v.document.model.IsRuneBoundary(end) {
		end--
	}
	text, _ := v.document.model.Slice(textedit.Range{Start: r.Start + start, End: end})
	pixels := make([]color.NRGBA, key.columns)
	var tokenAt func(int) color.NRGBA
	if e != nil && e.Ready() {
		tokens, _, ok := e.PaintTokens(row)
		if ok {
			tokenAt = func(at int) color.NRGBA {
				i := sort.Search(len(tokens), func(i int) bool { return tokens[i].End > at })
				if i < len(tokens) && tokens[i].Start <= at {
					if c, ok := v.colors.Tokens[tokens[i].Kind]; ok {
						return c
					}
				}
				return v.colors.Foreground
			}
		}
	}
	spans := v.document.highlightSpans(textedit.Range{Start: r.Start + start, End: end})
	si := 0
	column := 0
	for at := 0; at < len(text) && column < key.columns; {
		ch, size := utf8.DecodeRuneInString(text[at:])
		if ch == '\t' {
			column += key.tab - column%key.tab
		} else {
			if ch != ' ' && ch != '\r' && ch != '\n' {
				c := v.colors.Foreground
				if tokenAt != nil {
					c = tokenAt(start + at)
				}
				absolute := r.Start + start + at
				for si < len(spans) && spans[si].Range.End <= absolute {
					si++
				}
				if si < len(spans) && spans[si].Range.Start <= absolute {
					c = v.highlightColor(spans[si].Kind)
				}
				// Ordinary sRGB NRGBA; alpha makes the overview quieter than source text.
				c.A = uint8(uint16(c.A) * 180 / 255)
				pixels[column] = c
			}
			column++
		}
		at += size
	}
	if key.id != 0 {
		if len(s.cache) >= 2048 {
			clear(s.cache)
		}
		s.cache[key] = pixels
	}
	return pixels
}

func (v *SourceView) minimapThumb() geometry.Rectangle {
	s := v.minimap
	r := v.minimapRect()
	if s == nil {
		return geometry.Rect(r.X, 0, r.Width, r.Height)
	}
	total, extent := s.total, s.extent
	if s.dragging {
		total, extent = s.dragTotal, s.dragHeight
	}
	h := min(extent, max(8, extent*v.viewport.Height/max(1, total)))
	if s.dragging {
		h = s.dragThumb
	}
	y := float32(0)
	if total > v.viewport.Height {
		y = (extent - h) * max(0, min(1, v.offset.Y/(total-v.viewport.Height)))
	}
	return geometry.Rect(r.X, y, r.Width, h)
}
func (v *SourceView) beginMinimapDrag(point geometry.Point) {
	v.dragging = false
	if v.minimap == nil || v.projection != nil {
		return
	}
	s := v.minimap
	thumb := v.minimapThumb()
	s.dragging = true
	s.dragTotal, s.dragHeight, s.dragThumb = s.total, s.extent, thumb.Height
	s.dragOffset = thumb.Height / 2
	if point.Y >= thumb.Y && point.Y <= thumb.Y+thumb.Height {
		s.dragOffset = point.Y - thumb.Y
	}
	v.dragMinimap(point)
}
func (v *SourceView) dragMinimap(point geometry.Point) {
	s := v.minimap
	if s == nil || !s.dragging || !v.mounted {
		return
	}
	if !gui.IsEnabled(v) || v.projection != nil || s.generation.document != v.document || s.generation.revision != v.document.model.Revision() || s.generation.width != v.minimapWidth() || s.generation.height != v.viewport.Height {
		s.dragging = false
		return
	}
	ratio := max(0, min(1, (point.Y-s.dragOffset)/max(1, s.dragHeight-s.dragThumb)))
	y := ratio * max(0, s.dragTotal-v.viewport.Height)
	v.reveal = nil
	v.pendingAnchor = false
	v.anchor.valid = false
	v.scrollSignal.Emit(geometry.Rect(v.offset.X, y, v.viewport.Width, v.viewport.Height))
}
func (v *SourceView) endMinimapDrag() {
	if v.minimap != nil {
		v.minimap.dragging = false
	}
}

func (v *SourceView) paintMinimap(p gui.Painter) {
	r := v.minimapRect()
	if r.Width <= 0 {
		return
	}
	p.Save()
	defer p.Restore()
	p.SetClipRect(r)
	p.FillRect(r, graphics.ColorOf(v.partColor("minimap-background", v.colors.Background)))
	s := v.minimap
	if s != nil && s.pixels != nil && v.projection == nil {
		// A frame-local image also works with RenderWidget's separate painter.
		// The retained CPU bitmap is bounded; no painter-owned resource escapes.
		if img, err := p.NewImage(s.pixels); err == nil {
			p.DrawImage(geometry.Rect(r.X+3, 0, max(0, r.Width-6), s.extent), img)
			img.Destroy()
		}
		thumb := v.minimapThumb()
		p.FillRect(thumb, graphics.ColorOf(v.partColor("minimap-viewport", tint(v.colors.GutterForeground, 35))))
		p.DrawRect(geometry.Rect(thumb.X+.5, thumb.Y+.5, max(0, thumb.Width-1), max(0, thumb.Height-1)), 1, graphics.ColorOf(v.partColor("minimap-border", tint(v.colors.GutterForeground, 90))))
		if row, ok := v.ExecutionLine(); ok {
			y := s.extent * (padding + v.paragraphTop(v.markRow(row))) / max(1, v.heights.Total()+2*padding)
			p.FillRect(geometry.Rect(r.X+r.Width-3, y, 3, 2), graphics.ColorOf(v.partColor("execution-arrow", rgb(0xc39400))))
		}
	}
	p.DrawLine(geometry.Point{X: r.X + .5}, geometry.Point{X: r.X + .5, Y: r.Height}, 1, graphics.ColorOf(v.colors.Border))
}
func (v *SourceView) snapshotMinimap(info *gui.WidgetInfo) {
	if !v.showMinimap || v.minimapWidth() <= 0 {
		return
	}
	child := gui.WidgetInfo{Role: gui.Role("minimap"), ID: info.ID + ".minimap", Visible: true, Enabled: gui.IsEnabled(v) && v.projection == nil, Bounds: v.minimapRect().Translate(info.Bounds.Pos), Actions: []gui.Action{gui.ActionClick}}
	child.SetAttribute("goui.sourceview.minimap", map[string]any{"pending": v.minimap == nil || v.minimapPending(), "viewport": v.minimapThumb(), "columns": v.MinimapOptions().MaxColumns})
	info.Children = append(info.Children, child)
}
