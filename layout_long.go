package sourceview

import (
	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/typography"
	"strings"
)

func (p *paragraph) geometryAt(offset int) *textedit.Geometry {
	if p.paged != nil {
		if g := p.paged.GeometryAt(offset); g != nil {
			return g
		}
	}
	return &p.geometry
}

func (v *SourceView) resolveLongParagraph(row int, key layoutKey, p *paragraph, style paragraphStyle, tokens []highlight.Token, focus, measure bool) *paragraph {
	state, epoch := v.operationState(), v.layoutEpoch
	if _, failed := v.layoutFailures[key]; failed {
		return nil
	}
	if p == nil {
		for old, candidate := range v.layouts {
			if candidate.paged != nil && candidate.row == row {
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
		p.paged = gui.NewParagraphLayout(v.context, format, key.width, r.End-r.Start, func(local textedit.Range) (string, error) {
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
	p.paged.SetWidth(key.width)
	if measure && v.reveal != nil {
		r := v.displayRange(row)
		if p.paged.GeometryAt(v.displayCaret()-r.Start) == nil {
			v.requestLayoutWork()
		}
	} else if measure && v.pendingAnchor && v.anchor.valid {
		r := v.displayRange(row)
		if v.anchor.offset >= r.Start && v.anchor.offset <= r.End && p.paged.GeometryAt(v.anchor.offset-r.Start) == nil {
			v.requestLayoutWork()
		}
	}
	viewport := geometry.Rect(v.offset.X, max(0, v.offset.Y-padding-v.paragraphTop(row)), v.viewport.Width, max(v.lineHeight, v.viewport.Height))
	ready := p.paged.Prepare(viewport, measure)
	// An unattached/offscreen view has no frame source. Allow a small fixed
	// number of bounded steps, never an unbounded walk to distant text.
	for step := 0; !ready && measure && v.Root() == nil && step < 4; step++ {
		ready = p.paged.Prepare(viewport, true)
	}
	if !state.valid(v) || epoch != v.layoutEpoch || v.layouts[key] != p {
		return nil
	}
	if err := p.paged.Err(); err != nil {
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
	p.geometry = *p.paged.Geometry()
	size := p.paged.Size()
	p.width, p.height = size.Width, size.Height
	v.setParagraphHeight(row, p.height)
	v.contentWidth = max(v.contentWidth, p.width)
	r := p.paged.VisibleRange()
	if !measure && p.textStart != r.Start {
		p.text = ""
	}
	p.textStart = r.Start
	if measure {
		p.text = p.paged.Text(r)
		if v.whitespace != WhitespaceNone || v.showLineEndings {
			v.refreshWhitespace(p, r.Start, p.paged.Version())
		}
	} else if (v.whitespace != WhitespaceNone || v.showLineEndings) && (p.whitespace == nil || p.whitespace.textStart != r.Start || p.whitespace.version != p.paged.Version() || p.whitespace.revision != v.document.model.Revision()) {
		v.requestLayoutWork()
	}
	if !p.styled || p.style != style || p.pageVersion != p.paged.Version() || p.colorRange != r {
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
func (v *SourceView) changeLongParagraphs(c textedit.Change) {
	for key, p := range v.layouts {
		if p.paged == nil || c.Reset {
			continue
		}
		r := p.sourceRange
		var local []textedit.Edit
		invalid := false
		for _, e := range c.Edits {
			if e.Range.End <= r.Start && e.Range.Start < r.Start {
				r.Start = textedit.MapOffset(r.Start, e)
				r.End = textedit.MapOffset(r.End, e)
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
			p.paged.Destroy()
			if p.number != nil {
				p.number.Destroy()
			}
			continue
		}
		p.sourceRange = r
		if len(local) > 0 {
			p.paged.Change(r.End-r.Start, local)
			p.styled = false
		}
	}
}
