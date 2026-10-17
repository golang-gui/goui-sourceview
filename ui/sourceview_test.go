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
	v.Update(ctx, w)
	if w.Search() != (source.SearchQuery{Text: "user query", WholeWord: true}) {
		t.Fatal("rebuild cleared interactive query")
	}
	SourceView().Search(source.SearchQuery{}).Update(ctx, w)
	if w.Search() != (source.SearchQuery{}) {
		t.Fatal("explicit controlled empty query ignored")
	}
	v.Unmount(ctx, w)
}
