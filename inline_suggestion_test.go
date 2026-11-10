package sourceview

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
)

func inlineFixture(t *testing.T, text string) (*SourceView, *countedNativeContext) {
	t.Helper()
	v, ctx := collapsedLineFixture(t, text)
	v.SetAutomaticCompletion(false)
	focusBracketFixture(t, v)
	v.SetSelection(textedit.Selection{Anchor: len(text), Caret: len(text)})
	v.SetInlineSuggestionsEnabled(true)
	t.Cleanup(v.ClearInlineSuggestion)
	return v, ctx
}
func showInline(t *testing.T, v *SourceView, text string) {
	t.Helper()
	ok, err := v.SetInlineSuggestion(v.Document(), v.Document().Model().Revision(), InlineSuggestion{Position: v.Selection().Caret, Text: text})
	if err != nil || !ok {
		t.Fatal("suggestion not installed", ok, err)
	}
	if _, visible := v.inlinePreview(); !visible {
		t.Fatal("suggestion has no visible native preview")
	}
}

func TestInlineSuggestionIsViewOnlyAndAcceptsAsOneEdit(t *testing.T) {
	v, ctx := inlineFixture(t, "fmt.Pr")
	m := v.Document().Model()
	revision := m.Revision()
	size := v.ContentSize()
	showInline(t, v, `intln("hello")`)
	if m.Text() != "fmt.Pr" || m.Revision() != revision || m.CanUndo() || v.ContentSize() != size {
		t.Fatal("preview changed source/history/extents")
	}
	v.SetSearch(SearchQuery{Text: `intln("hello")`})
	if _, found := v.FindNext(false); found {
		t.Fatal("uncommitted suggestion participated in search")
	}
	v.SetSearch(SearchQuery{})
	var saved bytes.Buffer
	if _, err := v.Document().WriteTo(&saved); err != nil || saved.String() != "fmt.Pr" {
		t.Fatal("preview leaked into export", err)
	}
	info := v.Snapshot()
	meta := info.Attributes["goui.sourceview"].(map[string]any)
	if info.TextEditing.Preedit != nil || info.TextEditing.VisibleText != "fmt.Pr" || info.TextEditing.Length != 6 || meta["inlineSuggestion"].(map[string]any)["visible"] != true {
		t.Fatal("suggestion polluted committed or IME semantics")
	}
	before := ctx.created
	draw := &recordingPainter{}
	v.Paint(draw)
	v.Snapshot()
	if ctx.created != before || !slices.Contains(draw.order, "text") {
		t.Fatal("paint/snapshot performed native preparation or lost text")
	}
	v.SetAcceptsTab(false)
	if !v.applyKey(events.KeyEvent{Key: events.KeyTab}) || m.Text() != `fmt.Println("hello")` || v.Selection().Caret != len(m.Text()) {
		t.Fatal("Tab failed to accept exact text")
	}
	if _, ok := v.InlineSuggestion(); ok {
		t.Fatal("accepted suggestion remains active")
	}
	if !v.Undo() || m.Text() != "fmt.Pr" || m.CanUndo() {
		t.Fatal("accept was not one undo unit")
	}
	if !v.Redo() || m.Text() != `fmt.Println("hello")` {
		t.Fatal("redo did not restore accepted suggestion")
	}
}

func TestInlineSuggestionRejectsInvalidAndStaleResults(t *testing.T) {
	v, _ := inlineFixture(t, "prefix")
	d := v.Document()
	rev := d.Model().Revision()
	v.SetInlineSuggestionsEnabled(false)
	if ok, err := v.SetInlineSuggestion(d, rev, InlineSuggestion{6, "suffix"}); ok || err != nil {
		t.Fatal("default-off feature accepted a suggestion")
	}
	v.SetInlineSuggestionsEnabled(true)
	showInline(t, v, "suffix")
	for _, text := range []string{"", "   ", "a\tb", "a\u2028b", string([]byte{0xff}), strings.Repeat("x", (64<<10)+1)} {
		if ok, err := v.SetInlineSuggestion(d, rev, InlineSuggestion{6, text}); ok || err == nil {
			t.Fatal("invalid literal accepted", len(text), err)
		}
		if s, ok := v.InlineSuggestion(); !ok || s.Text != "suffix" {
			t.Fatal("invalid replacement destroyed valid suggestion")
		}
	}
	for _, document := range []*Document{nil, NewDocument("prefix")} {
		if ok, err := v.SetInlineSuggestion(document, rev, InlineSuggestion{6, "other"}); ok || err != nil {
			t.Fatal("foreign document result accepted")
		}
	}
	if ok, err := v.SetInlineSuggestion(d, rev+1, InlineSuggestion{6, "other"}); ok || err != nil {
		t.Fatal("stale revision accepted")
	}
	if ok, err := v.SetInlineSuggestion(d, rev, InlineSuggestion{99, "other"}); ok || err == nil {
		t.Fatal("invalid position accepted")
	}
	if ok, err := v.SetInlineSuggestion(d, rev, InlineSuggestion{3, "other"}); ok || err != nil {
		t.Fatal("non-caret/non-end suggestion accepted")
	}
	v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
	if ok, err := v.SetInlineSuggestion(d, rev, InlineSuggestion{3, "other"}); ok || err != nil {
		t.Fatal("midline suggestion accepted")
	}
}

func TestInlineSuggestionInvalidationAndEscape(t *testing.T) {
	for _, action := range []string{"escape", "selection", "edit", "reset", "document", "readonly", "disabled", "feature", "focus", "completion", "IME"} {
		t.Run(action, func(t *testing.T) {
			v, _ := inlineFixture(t, "prefix")
			showInline(t, v, "suffix")
			switch action {
			case "escape":
				if !v.applyKey(events.KeyEvent{Key: events.KeyEscape}) || v.escapeTab {
					t.Fatal("Escape did not dismiss exclusively")
				}
			case "selection":
				v.SetSelection(textedit.Selection{})
			case "edit":
				if err := v.ReplaceSelection("x"); err != nil {
					t.Fatal(err)
				}
			case "reset":
				v.Document().Model().SetText("reset")
			case "document":
				v.SetDocument(NewDocument("other"))
			case "readonly":
				v.SetReadOnly(true)
			case "disabled":
				v.SetEnabled(false)
				v.StyleChanged()
			case "feature":
				v.SetInlineSuggestionsEnabled(false)
			case "focus":
				var d gui.EventDispatcher
				host := &bracketHost{widget: v, focused: v}
				if err := d.DispatchEvent(host, events.FocusEvent{Focused: true}); err != nil {
					t.Fatal(err)
				}
				host.focused = nil
				if err := d.DispatchEvent(host, events.FocusEvent{Focused: false}); err != nil {
					t.Fatal(err)
				}
			case "completion":
				if err := v.ShowCompletion(); err != nil {
					t.Fatal(err)
				}
			case "IME":
				v.applyPreedit("中", 3)
			}
			if _, ok := v.InlineSuggestion(); ok || v.inlineSuggestion != nil {
				t.Fatal("stale suggestion survived", action)
			}
			if action == "IME" {
				if v.projection == nil {
					t.Fatal("IME was blocked")
				}
				v.cancelPreedit()
			}
		})
	}
}

func TestInlineSuggestionClipsAtNativeClustersAndReusesLayout(t *testing.T) {
	v, ctx := inlineFixture(t, "prefix")
	showInline(t, v, strings.Repeat("中文e\u0301😀", 30))
	original := v.Document().Model().Text()
	for _, width := range []float32{220.3, 310.7, 400} {
		before := v.inlineSuggestion.layout
		v.Arrange(geometry.Rect(0, 0, width, 180))
		preview, ok := v.inlinePreview()
		if !ok || !preview.truncated || v.inlineSuggestion.layout != before || preview.bounds.X+preview.bounds.Width > v.textRight()-padding+.01 {
			t.Fatal("resize lost/truncated outside text area or recreated suffix layout")
		}
		edge := preview.textClip.Width + v.inlineSuggestion.geometry.LineMetrics(0).X
		_, clusters := v.inlineSuggestion.layout.MeasureMetrics()
		for _, c := range clusters {
			if c.Width > 0 && c.X < edge-.01 && c.X+c.Width > edge+.01 {
				t.Fatal("preview cuts a native glyph cluster", edge, c)
			}
		}
		created := ctx.created
		v.Paint(&recordingPainter{})
		v.Snapshot()
		if ctx.created != created || v.Document().Model().Text() != original {
			t.Fatal("paint or resize mutated source or allocated native layouts")
		}
	}
	v.SetScheme(scheme.Dark())
	v.Arrange(v.Rect())
	if _, ok := v.inlinePreview(); !ok {
		t.Fatal("theme change failed to prepare the preview")
	}
}

func TestInlineSuggestionHiddenPreviewCannotStealTab(t *testing.T) {
	v, _ := inlineFixture(t, strings.Repeat("x", 38))
	v.SetWrapMode(WrapNone)
	v.Arrange(geometry.Rect(0, 0, 100, 180))
	if ok, err := v.SetInlineSuggestion(v.Document(), v.Document().Model().Revision(), InlineSuggestion{38, "suffix"}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, visible := v.inlinePreview(); visible {
		t.Fatal("offscreen suggestion reported visible")
	}
	if err := v.AcceptInlineSuggestion(); err != nil || v.Document().Model().Text() != strings.Repeat("x", 38) {
		t.Fatal("invisible suggestion was committed", err)
	}
	v.SetAcceptsTab(false)
	if v.applyKey(events.KeyEvent{Key: events.KeyTab}) {
		t.Fatal("invisible suggestion intercepted focus traversal")
	}
	if !v.applyKey(events.KeyEvent{Key: events.KeyEscape}) || v.inlineSuggestion != nil || !v.escapeTab {
		t.Fatal("Escape failed to clear hidden request or changed ordinary Escape behavior")
	}
}

func TestInlineSuggestionSharedViewAndUndoBoundary(t *testing.T) {
	v, _ := inlineFixture(t, "prefix")
	other := NewSourceView()
	other.SetDocument(v.Document())
	observe(t, other)
	if err := v.ReplaceSelection(" "); err != nil {
		t.Fatal(err)
	}
	showInline(t, v, "suffix")
	if _, ok := other.InlineSuggestion(); ok {
		t.Fatal("suggestion leaked to shared view")
	}
	if err := v.AcceptInlineSuggestion(); err != nil {
		t.Fatal(err)
	}
	if !v.Undo() || v.Document().Model().Text() != "prefix " {
		t.Fatal("accept undo erased prior input")
	}
	if !v.Undo() || v.Document().Model().Text() != "prefix" {
		t.Fatal("prior input history lost")
	}
}

type inlineReentryContext struct {
	typography.Context
	onCreate func()
}

func (c *inlineReentryContext) NewTextLayout(text string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(text, f, w, h)
	if c.onCreate != nil && text == "suffix" {
		f := c.onCreate
		c.onCreate = nil
		f()
	}
	return l, err
}
func TestInlineSuggestionNativeFailureAndReentry(t *testing.T) {
	v, ctx := inlineFixture(t, "prefix")
	v.context = &inlineReentryContext{Context: ctx, onCreate: func() { v.SetDocument(NewDocument("replacement")) }}
	if ok, err := v.SetInlineSuggestion(v.Document(), v.Document().Model().Revision(), InlineSuggestion{6, "suffix"}); ok || err != nil {
		t.Fatal("reentrant native creation attached stale suggestion", ok, err)
	}
	if v.inlineSuggestion != nil || v.Document().Model().Text() != "replacement" {
		t.Fatal("native callback replacement was overwritten")
	}
}

// Keep the context's real failure explicit; retry only on another host request.
type inlineFailureContext struct{ typography.Context }

func (c inlineFailureContext) NewTextLayout(text string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	if text == "suffix" {
		return nil, errors.New("inline layout failure")
	}
	return c.Context.NewTextLayout(text, f, w, h)
}
func TestInlineSuggestionNativeFailureIsReturned(t *testing.T) {
	v, ctx := inlineFixture(t, "prefix")
	v.context = inlineFailureContext{Context: ctx}
	if ok, err := v.SetInlineSuggestion(v.Document(), v.Document().Model().Revision(), InlineSuggestion{6, "suffix"}); ok || err == nil || v.inlineSuggestion != nil {
		t.Fatal("native failure was hidden or left retry state", ok, err)
	}
}

func TestInlineSuggestionWithholdsUnsafeRTLPlacement(t *testing.T) {
	v, _ := inlineFixture(t, "مرحبا")
	ok, err := v.SetInlineSuggestion(v.Document(), v.Document().Model().Revision(), InlineSuggestion{Position: len("مرحبا"), Text: "suffix"})
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, visible := v.inlinePreview(); visible {
		t.Fatal("RTL logical end overlaid existing source")
	}
	if err := v.AcceptInlineSuggestion(); err != nil || v.Document().Model().Text() != "مرحبا" {
		t.Fatal("invisible RTL suffix was accepted")
	}
}

func TestInlineSuggestionAcceptConsumesPriorEscapeTabIntent(t *testing.T) {
	v, _ := inlineFixture(t, "prefix")
	v.SetAcceptsTab(true)
	v.applyKey(events.KeyEvent{Key: events.KeyEscape})
	showInline(t, v, "suffix")
	if !v.applyKey(events.KeyEvent{Key: events.KeyTab}) || v.Document().Model().Text() != "prefixsuffix" {
		t.Fatal("visible suggestion did not accept after ordinary Escape")
	}
	if !v.applyKey(events.KeyEvent{Key: events.KeyTab}) || v.Document().Model().Text() != "prefixsuffix    " {
		t.Fatal("accepted suggestion left a stale focus-traversal Tab intent")
	}
}
