package ui

import (
	"testing"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/gui"
	base "github.com/golang-gui/goui/ui"
)

type context struct {
	base.BuildContext
	value any
}

func (c *context) State() any         { return c.value }
func (c *context) SetState(value any) { c.value = value }
func TestBindingRestoresDefaultsAndDisconnects(t *testing.T) {
	ctx := &context{}
	initial := SourceView()
	widget := initial.Mount(ctx).(*source.SourceView)
	initial.Update(ctx, widget)
	document := NewDocument("abc")
	events := 0
	configured := SourceView().Document(document).ReadOnly(true).ShowLineNumbers(false).AcceptsTab(true).WrapMode(WrapWordChar).Selection(TextSelection{Caret: 2}).OnSelection(func(TextSelection) { events++ })
	configured.Update(ctx, widget)
	options := SourceView().AutoPairs(false).AutomaticCompletion(false)
	options.Update(ctx, widget)
	if widget.AutoPairs() || widget.AutomaticCompletion() {
		t.Fatal("editor options ignored")
	}
	configured.Update(ctx, widget)
	if !widget.AutoPairs() || !widget.AutomaticCompletion() {
		t.Fatal("editor option defaults not restored")
	}
	if events != 0 || widget.Document() != document || !widget.ReadOnly() || widget.ShowLineNumbers() || !widget.AcceptsTab() {
		t.Fatal("update emitted or lost config")
	}
	widget.SetSelection(TextSelection{Caret: 3})
	if events != 1 {
		t.Fatal(events)
	}
	initial.Update(ctx, widget)
	if widget.Document() == document || widget.ReadOnly() || !widget.ShowLineNumbers() || widget.AcceptsTab() || widget.WrapMode() != WrapNone {
		t.Fatal("defaults not restored")
	}
	configured.Update(ctx, widget)
	configured.Unmount(ctx, gui.Widget(widget))
	widget.SetSelection(TextSelection{})
	if events != 1 {
		t.Fatal("unmount leaked callback", events)
	}
}

func TestUncontrolledSearchSurvivesDeclarativeUpdate(t *testing.T) {
	ctx := &context{}
	v := SourceView()
	w := v.Mount(ctx).(*source.SourceView)
	v.Update(ctx, w)
	w.SetSearch(SearchQuery{Text: "user query", WholeWord: true})
	w.SetHighlightSearchMatches(false)
	v.Update(ctx, w)
	if w.Search() != (SearchQuery{Text: "user query", WholeWord: true}) || w.HighlightSearchMatches() {
		t.Fatal("rebuild cleared interactive query")
	}
	SourceView().HighlightSearchMatches(true).Update(ctx, w)
	if !w.HighlightSearchMatches() {
		t.Fatal("explicit search highlight option ignored")
	}
	SourceView().Search(SearchQuery{}).Update(ctx, w)
	if w.Search() != (SearchQuery{}) {
		t.Fatal("explicit controlled empty query ignored")
	}
	v.Unmount(ctx, w)
}

func TestKeyBindingsAreExplicitAndCopied(t *testing.T) {
	ctx := &context{}
	view := SourceView()
	w := view.Mount(ctx).(*source.SourceView)
	view.Update(ctx, w)
	bindings := KeyBindings{CommandComplete: {{Key: gui.KeyF2}}}
	configured := SourceView().KeyBindings(bindings)
	bindings[CommandComplete][0].Key = gui.KeyF4
	configured.Update(ctx, w)
	if got := w.KeyBindings()[CommandComplete]; len(got) != 1 || got[0].Key != gui.KeyF2 {
		t.Fatal("view retained caller key slices", got)
	}
	view.Update(ctx, w)
	if len(w.KeyBindings()) != 1 {
		t.Fatal("uncontrolled update restored default bindings")
	}
	SourceView().KeyBindings(nil).Update(ctx, w)
	view.Update(ctx, w)
	if len(w.KeyBindings()) != 0 {
		t.Fatal("explicit empty bindings were ignored or reset")
	}
	view.Unmount(ctx, w)
}

// 未声明显示选项的重建保留交互状态，显式 None/false 仍可关闭。
func TestUncontrolledWhitespaceSurvivesDeclarativeUpdate(t *testing.T) {
	ctx := &context{}
	view := SourceView()
	w := view.Mount(ctx).(*source.SourceView)
	defer view.Unmount(ctx, w)
	view.Update(ctx, w)
	if w.WhitespaceMode() != WhitespaceNone || w.ShowLineEndings() {
		t.Fatal("new view defaults")
	}
	if err := w.SetWhitespaceMode(WhitespaceBoundary); err != nil {
		t.Fatal(err)
	}
	w.SetShowLineEndings(true)
	view.Update(ctx, w)
	if w.WhitespaceMode() != WhitespaceBoundary || !w.ShowLineEndings() {
		t.Fatal("rebuild reset interactive state")
	}
	SourceView().WhitespaceMode(WhitespaceNone).Update(ctx, w)
	if w.WhitespaceMode() != WhitespaceNone || !w.ShowLineEndings() {
		t.Fatal("explicit None or independent EOL option ignored")
	}
	SourceView().WhitespaceMode(WhitespaceAll).ShowLineEndings(false).Update(ctx, w)
	view.Update(ctx, w)
	if w.WhitespaceMode() != WhitespaceAll || w.ShowLineEndings() {
		t.Fatal("explicit false or retained mode ignored")
	}
}
