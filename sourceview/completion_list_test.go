package sourceview

import (
	"testing"

	"github.com/golang-gui/goui-sourceview/internal/controls"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/layout"
)

type completionFixtureView struct {
	*controls.CompletionList
	view    *SourceView
	session *completionSession
}

func completionFixture(t *testing.T, items []Completion) (*completionFixtureView, *shapedContext) {
	t.Helper()
	v := NewSourceView()
	v.SetScheme(scheme.Light())
	ctx := &shapedContext{}
	v.context = ctx
	s := &completionSession{items: items, prefix: "tr"}
	l := newCompletionList(v, s)
	s.list = l
	v.completion = s
	t.Cleanup(func() {
		v.completion = nil
		l.Release()
		if ctx.created != ctx.destroyed {
			t.Errorf("completion leaked layouts: %d/%d", ctx.created, ctx.destroyed)
		}
	})
	return &completionFixtureView{l, v, s}, ctx
}
func arrangeCompletion(l *completionFixtureView) {
	if l.session.panel == nil {
		l.session.panel = controls.NewCompletionPanel(l.CompletionList)
	}
	p := l.session.panel
	m := p.Measure(layout.Unbounded())
	p.Arrange(geometry.Rect(0, 0, m.Width, m.Height))
}
func TestCompletionUnicodePrefixAndWordAnchor(t *testing.T) {
	v, _ := colorFixture(t)
	v.SetShowLineNumbers(false)
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	v.SetSelection(TextSelection{Anchor: 5, Caret: 5})
	caret, ok := v.caretRect()
	if !ok {
		t.Fatal("missing caret")
	}
	c := CompletionContext{Range: TextRange{Start: 1, End: 5}}
	position := v.completionPosition(c, caret)
	if position != (geometry.Point{X: v.gutter + 4, Y: caret.Y + caret.Height + 2}) {
		t.Fatal("list text did not align with word start", position)
	}
	caret.Y += 20
	if p := v.completionPosition(c, caret); p.X != caret.X-12 {
		t.Fatal("wrapped start displaced popup", p)
	}
	v.offset.X = 30
	if p := v.completionPosition(c, caret); p.X != caret.X-12 {
		t.Fatal("hidden start displaced popup", p)
	}
	if v.Selection() != (TextSelection{Anchor: 5, Caret: 5}) {
		t.Fatal("anchoring changed selection")
	}
}
