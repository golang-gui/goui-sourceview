package sourceview

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
)

func multilineFixture(t *testing.T) (*SourceView, *countedNativeContext) {
	t.Helper()
	v, ctx := inlineFixture(t, "prefix\nTAIL")
	v.SetSelection(textedit.Selection{Anchor: 6, Caret: 6})
	return v, ctx
}
func TestInlineMultilineExtentIsViewLocalAndAcceptIsAtomic(t *testing.T) {
	v, ctx := multilineFixture(t)
	model := v.Document().Model()
	original, revision := model.Text(), model.Revision()
	top, size := v.paragraphTop(1), v.ContentSize()
	showInline(t, v, "\n\treturn value\n}")
	m := v.inlineSuggestion.multi
	if m == nil || m.extra <= 0 || v.paragraphTop(1) != top+m.extra || v.ContentSize().Height != size.Height+m.extra {
		t.Fatal("multiline preview does not shift following source by measured height")
	}
	if model.Text() != original || model.Revision() != revision || model.LineCount() != 2 || model.CanUndo() {
		t.Fatal("virtual lines entered source/history")
	}
	var saved bytes.Buffer
	if _, err := v.Document().WriteTo(&saved); err != nil || saved.String() != original {
		t.Fatal("preview leaked into export", err)
	}
	v.SetSearch(SearchQuery{Text: "return value"})
	if _, ok := v.FindNext(false); ok {
		t.Fatal("virtual text entered document search")
	}
	v.SetSearch(SearchQuery{})
	info := v.Snapshot()
	if info.TextEditing.Preedit != nil || info.TextEditing.Length != len(original) || strings.Contains(info.TextEditing.VisibleText, "return") {
		t.Fatal("virtual lines entered committed/IME semantics")
	}
	created, sizes, metrics := ctx.created, ctx.sizes, ctx.metrics
	v.Paint(&recordingPainter{})
	v.Snapshot()
	if ctx.created != created || ctx.sizes != sizes || ctx.metrics != metrics {
		t.Fatal("paint/snapshot measured or allocated native layout")
	}
	if !v.applyKey(events.KeyEvent{Key: events.KeyTab}) || model.Text() != "prefix\n\treturn value\n}\nTAIL" || v.Selection().Caret != len("prefix\n\treturn value\n}") {
		t.Fatal("Tab failed exact multiline insertion")
	}
	if !v.Undo() || model.Text() != original || model.CanUndo() {
		t.Fatal("multiline insertion is not one undo unit")
	}
	if !v.Redo() || model.LineCount() != 4 {
		t.Fatal("multiline redo failed")
	}
}
func TestInlineMultilineClearAndExternalEditRestoreExtent(t *testing.T) {
	for _, action := range []string{"escape", "selection", "external-before", "external-after", "reset", "replace", "readonly", "IME", "fold"} {
		t.Run(action, func(t *testing.T) {
			v, _ := multilineFixture(t)
			base := v.paragraphTop(1)
			showInline(t, v, "\nline one\nline two")
			switch action {
			case "escape":
				v.applyKey(events.KeyEvent{Key: events.KeyEscape})
			case "selection":
				v.SetSelection(textedit.Selection{})
			case "external-before":
				if err := v.Document().Model().Replace(textedit.Range{}, "X"); err != nil {
					t.Fatal(err)
				}
			case "external-after":
				at := v.Document().Model().Len()
				if err := v.Document().Model().Replace(textedit.Range{Start: at, End: at}, "X"); err != nil {
					t.Fatal(err)
				}
			case "reset":
				v.Document().Model().SetText("other\nTAIL")
			case "replace":
				showInline(t, v, "suffix")
				v.ClearInlineSuggestion()
			case "readonly":
				v.SetReadOnly(true)
			case "IME":
				v.applyPreedit("中", 3)
				v.cancelPreedit()
			case "fold":
				v.SetFoldingEnabled(true)
				v.acceptFolds([]language.FoldRange{{StartLine: 0, EndLine: 1}})
				v.Fold(0)
				v.Unfold(0)
			}
			if _, ok := v.InlineSuggestion(); ok {
				t.Fatal("multiline state retained", action)
			}
			v.Arrange(v.Rect())
			if v.paragraphTop(1) != base {
				t.Fatal("dismissal/edit retained virtual height", action, v.paragraphTop(1), base)
			}
		})
	}
}
func TestInlineMultilineHitUsesCommittedCoordinates(t *testing.T) {
	v, _ := multilineFixture(t)
	showInline(t, v, "tail\nvirtual one\nvirtual two")
	m := v.inlineSuggestion.multi
	gapTop := padding + v.paragraphTop(0) + v.cachedParagraph(0).height
	pos, _, ok := v.hitCaret(geometry.Point{X: v.gutter + 30, Y: gapTop + m.extra/2})
	if !ok || pos.Offset != 6 {
		t.Fatal("virtual-only space mapped to ghost bytes", pos)
	}
	tailTop := padding + v.paragraphTop(1)
	pos, _, ok = v.hitCaret(geometry.Point{X: v.gutter + padding + 1, Y: tailTop + v.lineHeight/2})
	if !ok || pos.Offset < 7 || pos.Offset > 11 {
		t.Fatal("following source hit ignored virtual height", pos)
	}
	v.SetSelection(textedit.Selection{Anchor: pos.Offset, Caret: pos.Offset})
	if v.inlineSuggestion != nil || v.Selection().Caret < 7 {
		t.Fatal("selection did not retain committed target after virtual extent removal")
	}
}
func TestInlineMultilineWrapUsesMeasuredRemainder(t *testing.T) {
	v, _ := multilineFixture(t)
	text := strings.Repeat("中文e\u0301 ", 40) + "\n\treturn 1\n}"
	showInline(t, v, text)
	wide := v.inlineSuggestion.multi.extra
	for _, width := range []float32{210.3, 310.7, 400} {
		v.Arrange(geometry.Rect(0, 0, width, 180))
		m := v.inlineSuggestion.multi
		if m == nil || m.headGeometry.LineCount() < 2 || !utf8.ValidString(m.tailText) || m.tailGeometry.LineCount() < 3 {
			t.Fatal("long first fragment did not flow into full-width continuation")
		}
		split := m.headGeometry.LineMetrics(1).Start
		head, rest, _ := strings.Cut(text, "\n")
		if head[:split]+strings.TrimSuffix(m.tailText, "\n"+rest) != head {
			t.Fatal("native wrap lost/duplicated prefix text")
		}
		if width < 400 && m.extra < wide {
			t.Fatal("narrow width unexpectedly reduced measured preview height")
		}
		if v.paragraphTop(1) != v.cachedParagraph(0).height+m.extra {
			t.Fatal("reflow extent does not match native suggestion layouts")
		}
	}
	v.ClearInlineSuggestion()
	if v.paragraphTop(1) != v.cachedParagraph(0).height {
		t.Fatal("reflow cleanup retained height")
	}
}
func TestInlineMultilineNormalizedLimitsAndTabs(t *testing.T) {
	v, _ := multilineFixture(t)
	showInline(t, v, "\r\n\treturn x\r\n}")
	suggestion, _ := v.InlineSuggestion()
	if suggestion.Text != "\n\treturn x\n}" {
		t.Fatal("line endings not normalized")
	}
	if err := v.AcceptInlineSuggestion(); err != nil || v.Document().Model().Text() != "prefix\n\treturn x\n}\nTAIL" {
		t.Fatal("normalized literal did not commit", err)
	}
	v.Undo()
	v.SetSelection(textedit.Selection{Anchor: 6, Caret: 6})
	for _, text := range []string{strings.Repeat("x", (64<<10)+1), "a" + strings.Repeat("\n", 256), "a\t\nrest", "a\n\x00b"} {
		if ok, err := v.SetInlineSuggestion(v.Document(), v.Document().Model().Revision(), InlineSuggestion{6, text}); ok || err == nil {
			t.Fatal("invalid multiline limit/control accepted", len(text), err)
		}
	}
	showInline(t, v, "\n"+strings.Repeat("\tline\n", 100)+"end")
	if v.inlineSuggestion.multi.tailGeometry.LineCount() < 101 {
		t.Fatal("multiline indentation/newlines lost")
	}
}
func TestInlineMultilineVisibleWhenCaretIsAboveViewport(t *testing.T) {
	v, _ := multilineFixture(t)
	showInline(t, v, "\n"+strings.Repeat("virtual line\n", 30)+"end")
	v.LayoutVisible(geometry.Size{Width: 400, Height: 100}, geometry.Point{Y: 160})
	if preview, ok := v.inlinePreview(); !ok || preview.bounds.Y < 0 || preview.bounds.Y+preview.bounds.Height > 100 {
		t.Fatal("scrolling lost visible continuation or viewport clip", preview)
	}
	pos, _, ok := v.hitCaret(geometry.Point{X: 80, Y: 30})
	if !ok || pos.Offset != 6 {
		t.Fatal("scrolled virtual line has a source offset", pos)
	}
	if err := v.AcceptInlineSuggestion(); err != nil || v.Document().Model().LineCount() < 32 {
		t.Fatal("visible continuation could not be accepted", err)
	}
}
func TestInlineMultilineNoWrapIncludesHorizontalExtent(t *testing.T) {
	v, _ := multilineFixture(t)
	v.SetWrapMode(WrapNone)
	v.Arrange(v.Rect())
	base := v.ContentSize()
	showInline(t, v, "suffix\n"+strings.Repeat("X", 300))
	if v.ContentSize().Width <= base.Width {
		t.Fatal("no-wrap preview omitted horizontal extent")
	}
	v.ClearInlineSuggestion()
	if v.ContentSize().Width != base.Width {
		t.Fatal("no-wrap cleanup retained horizontal extent")
	}
}

type multilineProbeContext struct {
	typography.Context
	target, phase string
	callback      func()
	fail          bool
}
type multilineProbeLayout struct {
	typography.TextLayout
	owner *multilineProbeContext
}

func (c *multilineProbeContext) fire(phase string) {
	if c.phase == phase && c.callback != nil {
		f := c.callback
		c.callback = nil
		f()
	}
}
func (c *multilineProbeContext) NewTextLayout(text string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	if text == c.target && c.fail {
		return nil, errors.New("multiline native failure")
	}
	l, err := c.Context.NewTextLayout(text, f, w, h)
	if err != nil || text != c.target {
		return l, err
	}
	c.fire("create")
	return &multilineProbeLayout{l, c}, nil
}
func (l *multilineProbeLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	a, b := l.TextLayout.MeasureMetrics()
	l.owner.fire("metrics")
	return a, b
}
func (l *multilineProbeLayout) MeasureSize() (float32, float32) {
	w, h := l.TextLayout.MeasureSize()
	l.owner.fire("size")
	return w, h
}
func TestInlineMultilineNativeFailureAndReentryReleaseBothLayouts(t *testing.T) {
	for _, target := range []string{"head", "tail\nend"} {
		for _, phase := range []string{"failure", "create", "metrics", "size"} {
			if target == "head" && phase == "size" {
				continue
			} // Only tail size is needed.
			t.Run(target+"/"+phase, func(t *testing.T) {
				v, ctx := multilineFixture(t)
				base := v.paragraphTop(1)
				beforeCreated, beforeDestroyed := ctx.created, ctx.destroyed
				probe := &multilineProbeContext{Context: ctx, target: target, phase: phase, fail: phase == "failure"}
				probe.callback = func() { v.ClearInlineSuggestion() }
				v.context = probe
				ok, err := v.SetInlineSuggestion(v.Document(), v.Document().Model().Revision(), InlineSuggestion{6, "head\ntail\nend"})
				if ok || (err != nil) != (phase == "failure") || v.inlineSuggestion != nil {
					t.Fatal("native failure/reentry retained request", ok, err)
				}
				if ctx.created-beforeCreated != ctx.destroyed-beforeDestroyed || v.paragraphTop(1) != base {
					t.Fatal("native failure/reentry leaked layout or height")
				}
			})
		}
	}
}
func TestInlineMultilineTabWidthReflowsAndDisposesOldLayout(t *testing.T) {
	v, ctx := multilineFixture(t)
	showInline(t, v, "\n\tvalue\n}")
	old := v.inlineSuggestion.multi
	before := ctx.destroyed
	o := v.IndentOptions()
	o.TabWidth = 8
	if err := v.SetIndentOptions(o); err != nil {
		t.Fatal(err)
	}
	v.Arrange(v.Rect())
	if v.inlineSuggestion.multi == old || v.inlineSuggestion.multi.tabWidth != 8*v.spaceWidth || ctx.destroyed <= before {
		t.Fatal("tab change retained stale native preview")
	}
}

func TestInlineMultilineReentryCannotOverwriteNewLayoutConfiguration(t *testing.T) {
	v, ctx := multilineFixture(t)
	probe := &multilineProbeContext{Context: ctx, target: "head", phase: "create"}
	probe.callback = func() { v.SetWrapMode(WrapNone); v.Arrange(v.Rect()) }
	v.context = probe
	showInline(t, v, "head\ntail\nend")
	m := v.inlineSuggestion.multi
	if m.wrap != WrapNone || m.head.Format().WrapMode != WrapNone {
		t.Fatal("old preparation overwrote reentrant current-width layout")
	}
}

func TestInlineMultilinePointerReleaseRetainsPressTarget(t *testing.T) {
	v, _ := multilineFixture(t)
	showInline(t, v, "\nvirtual line\nend")
	point := geometry.Point{X: 30, Y: padding + v.cachedParagraph(0).height + 5}
	var dispatch gui.EventDispatcher
	host := &bracketHost{widget: v, focused: v}
	if err := dispatch.DispatchEvent(host, events.FocusEvent{Focused: true}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []events.EventType{events.PointerDown, events.PointerMove, events.PointerUp} {
		if err := dispatch.DispatchEvent(host, events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: point}); err != nil {
			t.Fatal(err)
		}
		v.Arrange(v.Rect())
		if v.Selection() != (textedit.Selection{Anchor: 6, Caret: 6}) || v.inlineSuggestion != nil {
			t.Fatal("stationary pointer re-hit after preview space disappeared", kind, v.Selection())
		}
		if v.dragging {
			v.dragSelection(point)
		} // normal blink/autoscroll tick
	}
}

func TestInlineMultilinePagedSourceUsesCachedEndGeometry(t *testing.T) {
	prefix := strings.Repeat("X", 70<<10)
	v, _ := inlineFixture(t, prefix+"\nTAIL")
	v.SetWrapMode(WrapNone)
	v.SetSelection(textedit.Selection{Anchor: len(prefix), Caret: len(prefix)})
	v.Arrange(v.Rect())
	p := v.paragraph(0)
	if p == nil || p.paged == nil || p.paged.GeometryAt(len(prefix)) == nil {
		t.Fatal("missing source end page")
	}
	v.LayoutVisible(v.viewport, geometry.Point{})
	showInline(t, v, "\nvalue\n}")
	base := p.height
	if v.paragraphTop(1) <= base {
		t.Fatal("paged source lost multiline extent")
	}
	v.LayoutVisible(v.viewport, geometry.Point{Y: 30})
	if _, ok := v.inlinePreview(); !ok {
		t.Fatal("paged source lost cached anchor while scrolling")
	}
	v.ClearInlineSuggestion()
	if v.paragraphTop(1) != base {
		t.Fatal("paged source dismissal retained extent")
	}
}
