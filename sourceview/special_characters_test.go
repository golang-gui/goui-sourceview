package sourceview

import (
	"strings"
	"testing"
	"time"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/platform/typography"
)

func TestSpecialCharactersNativeMarkersPreserveGeometryAndText(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"zero-width", "A\u200bB\u2060C", "U+200B ZERO WIDTH SPACE"},
		{"controls", "a\x00b\x1bc\x7fd", "U+0000 NUL\nU+001B ESC\nU+007F DEL"},
		{"joiner", "a\u034f\u0301 👩\u200d💻", "U+200D ZERO WIDTH JOINER"},
		{"bidi", "abc \u2067אבג\u2069 de\u202eFG\u202c h", "U+2067 RIGHT-TO-LEFT ISOLATE"},
		{"separators", "A\u2028B\u2029C", "U+2028 LINE SEPARATOR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, ctx := collapsedLineFixture(t, tc.text)
			v.SetWrapMode(WrapNone)
			v.LayoutVisible(geometry.Size{Width: 600, Height: 180}, geometry.Point{})
			p := v.paragraph(0)
			if p == nil {
				t.Fatal("missing native paragraph")
			}
			native, created := p.Layout, ctx.created
			beforeCaret := p.Geometry.Caret(textcore.Position{Offset: len(tc.text)})
			beforeHit := p.Geometry.Hit(geometry.Point{X: 20, Y: 10})
			d := v.Document()
			point := d.CaptureSavePoint()
			d.MarkSaved(point)
			revision := d.Revision()
			v.SetShowSpecialCharacters(true)
			out := &recordingPainter{}
			origin := v.paragraphOrigin(0, p)
			v.paintSpecialCharacters(out, 0, p, origin)
			s := p.special
			if s == nil || s.Pending() {
				t.Fatal("marker index did not complete")
			}
			var labels []string
			count := 0
			s.Visit(func(m view.SpecialMarker) {
				count++
				labels = append(labels, m.Tooltip())
				v.pointerPoint = geometry.Point{X: origin.X + m.Cell.X, Y: origin.Y + m.Cell.Y + m.Cell.Height/2}
				if got := v.specialCharacterTooltip(); got == "" {
					t.Fatal("marker tooltip did not use native position")
				}
			})
			if count == 0 || out.lines != count*4 {
				t.Fatal("missing visible marker/name", count, out.lines, labels)
			}
			for _, want := range strings.Split(tc.want, "\n") {
				if !strings.Contains(strings.Join(labels, "\n"), want) {
					t.Fatal("missing control character name", want, labels)
				}
			}
			if tc.name == "controls" && p.width < v.spaceWidth*3 {
				t.Fatal("native layout truncated the text at a control character", p.width, v.spaceWidth)
			}
			if v.paragraph(0) != p || p.Layout != native || ctx.created != created || p.Geometry.Caret(textcore.Position{Offset: len(tc.text)}) != beforeCaret || p.Geometry.Hit(geometry.Point{X: 20, Y: 10}) != beforeHit {
				t.Fatal("display option changed native geometry")
			}
			if d.Text() != tc.text || d.Revision() != revision || d.Modified() || d.CanUndo() {
				t.Fatal("marker changed source/save point/history")
			}
			if v.Snapshot().Attributes["goui.sourceview"].(map[string]any)["specialCharacters"] != true {
				t.Fatal("snapshot omitted marker state")
			}
			v.SetShowSpecialCharacters(false)
			if p.special != nil || v.specialCharacterTooltip() != "" || d.Modified() {
				t.Fatal("disable retained marker cache or changed document")
			}
		})
	}
}

func TestSpecialCharactersEditUndoPreeditAndViewIsolation(t *testing.T) {
	v, ctx := collapsedLineFixture(t, "A\u200bB")
	d := v.Document()
	d.MarkSaved(d.CaptureSavePoint())
	v.SetShowSpecialCharacters(true)
	p := v.paragraph(0)
	v.paintSpecialCharacters(&recordingPainter{}, 0, p, v.paragraphOrigin(0, p))
	other := NewSourceView()
	other.SetDocument(d)
	observe(t, other)
	other.context = ctx
	other.SetShowLineNumbers(false)
	other.SetFoldingEnabled(false)
	t.Cleanup(other.dropLayouts)
	other.LayoutVisible(geometry.Size{Width: 400, Height: 180}, geometry.Point{})
	if other.ShowSpecialCharacters() {
		t.Fatal("shared document shared view option")
	}
	other.SetShowSpecialCharacters(true)
	q := other.paragraph(0)
	other.paintSpecialCharacters(&recordingPainter{}, 0, q, other.paragraphOrigin(0, q))
	if p.special == q.special {
		t.Fatal("views share mutable marker cache")
	}
	// CoreText may attach ZWSP to A. The independent native metrics define
	// the indivisible source cluster; selecting it must not invent a stop.
	_, clusters := p.Layout.MeasureMetrics()
	var marked TextRange
	for _, c := range clusters {
		if c.Start <= 1 && 1 < c.Start+c.Length {
			marked = TextRange{Start: c.Start, End: c.Start + c.Length}
			break
		}
	}
	if marked.Start > 1 || marked.End < 4 {
		t.Fatal("native reference omitted ZWSP cluster", marked)
	}
	want := d.Text()[:marked.Start] + d.Text()[marked.End:]
	v.SetSelection(TextSelection{Anchor: marked.Start, Caret: marked.End})
	if v.Selection().Range() != marked {
		t.Fatal("selection disagreed with native cluster", v.Selection(), marked)
	}
	if err := v.ReplaceSelection(""); err != nil || d.Text() != want || !d.Modified() {
		t.Fatal("edit", err, d.Text())
	}
	p = v.paragraph(0)
	out := &recordingPainter{}
	v.paintSpecialCharacters(out, 0, p, v.paragraphOrigin(0, p))
	if out.lines != 0 {
		t.Fatal("stale marker survived deletion")
	}
	if !v.Undo() || d.Text() != "A\u200bB" || d.Modified() {
		t.Fatal("undo/save identity")
	}
	p = v.paragraph(0)
	v.paintSpecialCharacters(out, 0, p, v.paragraphOrigin(0, p))
	if out.lines != 4 {
		t.Fatal("undo did not rebuild marker")
	}
	projection := textcore.NewProjection(d.model, v.Selection())
	projection.Update("中", 3)
	v.projection = projection
	before := d.Revision()
	v.SetShowSpecialCharacters(false)
	v.SetShowSpecialCharacters(true)
	preedit := &recordingPainter{}
	v.paintSpecialCharacters(preedit, 0, p, v.paragraphOrigin(0, p))
	if preedit.lines != 0 || v.projection != projection || d.Revision() != before || d.Modified() {
		t.Fatal("marker toggle interfered with preedit")
	}
	v.projection = nil
}

func TestSpecialCharactersLongPagedWindowAndScrollInvalidation(t *testing.T) {
	const at = 80000
	v, ctx := collapsedLineFixture(t, strings.Repeat("x", at)+"\u200btail")
	v.SetWrapMode(WrapNone)
	v.SetShowSpecialCharacters(true)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 180}, geometry.Point{})
	p := v.paragraph(0)
	if p == nil || p.Paged == nil {
		t.Fatal("expected bounded native pages")
	}
	v.paintSpecialCharacters(&recordingPainter{}, 0, p, v.paragraphOrigin(0, p))
	old := p.special
	if old == nil {
		t.Fatal("no initial marker window")
	}
	count := 0
	old.Visit(func(view.SpecialMarker) { count++ })
	if count != 0 {
		t.Fatal("offscreen marker retained")
	}
	// Generic monospace can fall back to a proportional font. Measure x
	// itself rather than assuming its advance equals the space advance.
	ref, err := ctx.NewTextLayout("x", typography.TextFormat{Font: v.font}, 1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	advance, _ := ref.MeasureSize()
	ref.Destroy()
	if advance <= 0 {
		t.Fatal("native x advance is empty")
	}
	offset := geometry.Point{X: float32(float64(at)*float64(advance)) - 100}
	for i := 0; i < 100; i++ {
		v.LayoutVisible(geometry.Size{Width: 400, Height: 180}, offset)
		v.specialCharacterStep(time.Time{})
	}
	p = v.paragraph(0)
	out := &recordingPainter{}
	v.paintSpecialCharacters(out, 0, p, v.paragraphOrigin(0, p))
	if p.special == old || out.lines != 4 {
		t.Fatal("paged marker missing or stale after scroll", out.lines, p.textStart, len(p.text), v.spaceWidth)
	}
	if len(p.text) > 128*1024 {
		t.Fatal("marker materialized an entire long paragraph")
	}
}

func TestSpecialCharactersCollapsedPreviewExcludesHiddenTooltip(t *testing.T) {
	text := "A\u200bB " + strings.Repeat("x", 45) + "\u2060" + strings.Repeat("y", 400) + "\ntail"
	v, _ := collapsedLineFixture(t, text)
	v.SetShowSpecialCharacters(true)
	p := v.paragraph(0)
	origin := v.paragraphOrigin(0, p)
	cell := p.Geometry.Caret(textcore.Position{Offset: strings.Index(text, "\u2060")})
	_, previewHeight := v.linePreview(p)
	if cell.Y < previewHeight || cell.Y+cell.Height > v.viewport.Height {
		t.Fatal("fixture must place the second marker below the preview but inside the viewport", cell)
	}
	v.paintSpecialCharacters(&recordingPainter{}, 0, p, origin)
	v.pointerPoint = geometry.Point{X: origin.X + cell.X, Y: origin.Y + cell.Y + cell.Height/2}
	if !strings.Contains(v.specialCharacterTooltip(), "U+2060 WORD JOINER") {
		t.Fatal("expanded marker tooltip missing")
	}
	if !v.CollapseLine(0) {
		t.Fatal("fixture did not collapse")
	}
	old := p.special
	out := &recordingPainter{}
	v.paintSpecialCharacters(out, 0, p, v.paragraphOrigin(0, p))
	if out.lines != 4 || p.special == old || v.specialCharacterTooltip() != "" {
		t.Fatal("collapsed preview retained hidden markers or tooltip", out.lines, v.specialCharacterTooltip())
	}
	if !v.ExpandLine(0) {
		t.Fatal("fixture did not expand")
	}
	v.paintSpecialCharacters(&recordingPainter{}, 0, p, v.paragraphOrigin(0, p))
	if !strings.Contains(v.specialCharacterTooltip(), "U+2060 WORD JOINER") || v.Document().Text() != text {
		t.Fatal("expansion did not restore the marker or changed text")
	}
}
