package sourceview

import (
	"fmt"
	"image/color"
	"strings"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/style"
)

// HighlightSpan overrides foreground in a normalized document's UTF-8 byte range.
// KindPlain explicitly restores the ordinary foreground. It changes no lexical
// context, completion rules, diagnostics, or other editor behavior.
type HighlightSpan struct {
	Range textedit.Range
	Kind  language.Kind
}

// Highlights is an immutable, indexed foreground snapshot, safe to build on a
// worker and share between documents. It retains no source text or native objects.
// Its zero value is an empty snapshot for an empty document.
type Highlights struct {
	root   *highlight.SpanNode
	length int
}

// NewHighlights validates and copies ordered, nonempty, nonoverlapping spans.
// Text must be the exact normalized UTF-8/LF snapshot analyzed by the caller.
// Spans may cross lines, but must begin and end at rune boundaries.
func NewHighlights(text string, spans []HighlightSpan) (*Highlights, error) {
	if !utf8.ValidString(text) || strings.ContainsRune(text, '\r') {
		return nil, fmt.Errorf("sourceview: highlights require normalized UTF-8/LF text")
	}
	h := &Highlights{length: len(text)}
	end := 0
	var builder highlight.SpanBuilder
	for i, s := range spans {
		r := s.Range
		boundary := func(at int) bool { return at == len(text) || utf8.RuneStart(text[at]) }
		if r.Start < end || r.End <= r.Start || r.End > len(text) || !boundary(r.Start) || !boundary(r.End) || !s.Kind.Valid() {
			return nil, fmt.Errorf("sourceview: invalid highlight span %d", i)
		}
		builder.Append(highlight.Span{Range: s.Range, Kind: s.Kind})
		end = r.End
	}
	h.root = builder.Root()
	return h, nil
}

// SetHighlights publishes a complete snapshot on the document's owning sequence.
// Revision must identify the exact text passed to NewHighlights; callers must
// also guard document identity and their own analysis/configuration generation.
// Stale publications (including nil clears) return false, nil. A current snapshot
// with a different length returns an error without changing existing colors.
// Nil clears the layer. Publishing changes neither revision nor editor behavior.
func (d *Document) SetHighlights(revision uint64, h *Highlights) (bool, error) {
	if revision != d.model.Revision() {
		return false, nil
	}
	if h != nil && h.length != d.model.Len() {
		return false, fmt.Errorf("sourceview: highlight snapshot length does not match document")
	}
	d.highlightRoot = nil
	if h != nil {
		d.highlightRoot = h.root
	}
	d.highlightRevision = revision
	d.highlightEpoch++
	for v := range d.views {
		if v.mounted && v.document == d {
			v.RequestPaint()
		}
	}
	d.requestWork()
	return true, nil
}

func (d *Document) changeHighlights(c textedit.Change) {
	// A callback may have already published for this or a later committed revision.
	if c.Revision <= d.highlightRevision {
		return
	}
	changed := d.highlightRoot != nil
	if c.Reset || c.Revision != d.highlightRevision+1 {
		d.highlightRoot = nil
	}
	d.highlightRoot = highlight.MapSpans(d.highlightRoot, c.Edits)
	d.highlightRevision = c.Revision
	if changed {
		d.highlightEpoch++
	}
}
func (d *Document) highlightSpans(r textedit.Range) []HighlightSpan {
	if r.End <= r.Start || d.highlightRevision != d.model.Revision() {
		return nil
	}
	var out []HighlightSpan
	highlight.VisitSpans(d.highlightRoot, r, func(span highlight.Span) {
		// Defend native renderers even if a caller supplied the wrong same-length text.
		if d.model.IsRuneBoundary(span.Range.Start) && d.model.IsRuneBoundary(span.Range.End) {
			out = append(out, HighlightSpan{Range: span.Range, Kind: span.Kind})
		}
	})
	return out
}

// Preserve colors outside preedit without applying document colors to IME text.
func (v *SourceView) displayHighlights(r textedit.Range) []HighlightSpan {
	if v.projection == nil {
		return v.document.highlightSpans(r)
	}
	p := v.projection
	replace := p.Replacement()
	end := replace.Start + len(p.Text())
	delta := end - replace.End
	var spans []HighlightSpan
	if r.Start < replace.Start {
		spans = v.document.highlightSpans(textedit.Range{Start: r.Start, End: min(r.End, replace.Start)})
	}
	if r.End > end {
		suffix := v.document.highlightSpans(textedit.Range{Start: max(r.Start, end) - delta, End: r.End - delta})
		for i := range suffix {
			suffix[i].Range.Start += delta
			suffix[i].Range.End += delta
		}
		spans = append(spans, suffix...)
	}
	return spans
}

func (v *SourceView) highlightColor(kind language.Kind) color.NRGBA {
	if kind == language.KindPlain {
		return v.colors.Foreground
	}
	if v.explicitScheme != nil {
		return v.explicitScheme.Color(kind)
	}
	if c, ok := v.highlightColors[kind]; ok {
		return c
	}
	name := v.StyleName()
	if name == "" {
		name = "source-view"
	}
	c := v.colors.Foreground
	if base, ok := v.colors.Tokens[kind]; ok {
		c = base
	}
	if value, ok := gui.ResolveWidgetStyle(v, name, "token-"+kind.String(), style.Normal).ForegroundColor(); ok {
		c = view.NRGBA(value)
	}
	if v.highlightColors == nil {
		v.highlightColors = make(map[language.Kind]color.NRGBA)
	}
	// Bound application-defined style keys retained by a long-lived view.
	if len(v.highlightColors) >= 256 {
		clear(v.highlightColors)
	}
	v.highlightColors[kind] = c
	return c
}
