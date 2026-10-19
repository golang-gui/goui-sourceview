package ui

import (
	"testing"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
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
	document := source.NewDocument("abc")
	events := 0
	configured := SourceView().Document(document).ReadOnly(true).ShowLineNumbers(false).AcceptsTab(true).WrapMode(source.WrapWordChar).Selection(textedit.Selection{Caret: 2}).OnSelection(func(textedit.Selection) { events++ })
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
	widget.SetSelection(textedit.Selection{Caret: 3})
	if events != 1 {
		t.Fatal(events)
	}
	initial.Update(ctx, widget)
	if widget.Document() == document || widget.ReadOnly() || !widget.ShowLineNumbers() || widget.AcceptsTab() || widget.WrapMode() != source.WrapNone {
		t.Fatal("defaults not restored")
	}
	configured.Update(ctx, widget)
	configured.Unmount(ctx, gui.Widget(widget))
	widget.SetSelection(textedit.Selection{})
	if events != 1 {
		t.Fatal("unmount leaked callback", events)
	}
}

func TestUncontrolledSearchSurvivesDeclarativeUpdate(t *testing.T) {
	ctx := &context{}
	v := SourceView()
	w := v.Mount(ctx).(*source.SourceView)
	v.Update(ctx, w)
	w.SetSearch(source.SearchQuery{Text: "user query", WholeWord: true})
	w.SetHighlightSearchMatches(false)
	v.Update(ctx, w)
	if w.Search() != (source.SearchQuery{Text: "user query", WholeWord: true}) || w.HighlightSearchMatches() {
		t.Fatal("rebuild cleared interactive query")
	}
	SourceView().HighlightSearchMatches(true).Update(ctx, w)
	if !w.HighlightSearchMatches() {
		t.Fatal("explicit search highlight option ignored")
	}
	SourceView().Search(source.SearchQuery{}).Update(ctx, w)
	if w.Search() != (source.SearchQuery{}) {
		t.Fatal("explicit controlled empty query ignored")
	}
	v.Unmount(ctx, w)
}

func TestKeyBindingsAreExplicitAndCopied(t *testing.T) {
	ctx := &context{}
	view := SourceView()
	w := view.Mount(ctx).(*source.SourceView)
	view.Update(ctx, w)
	bindings := source.KeyBindings{source.CommandComplete: {{Key: gui.KeyF2}}}
	configured := SourceView().KeyBindings(bindings)
	bindings[source.CommandComplete][0].Key = gui.KeyF4
	configured.Update(ctx, w)
	if got := w.KeyBindings()[source.CommandComplete]; len(got) != 1 || got[0].Key != gui.KeyF2 {
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
