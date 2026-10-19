// Package ui provides thin GOUI declarative bindings for SourceView.
package ui

import (
	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	base "github.com/golang-gui/goui/ui"
)

type SourceViewView struct {
	base.ViewBase[SourceViewView]
	document                       *source.Document
	readOnly, acceptsTab           bool
	selection                      *textedit.Selection
	indent                         *source.IndentOptions
	wrap                           *source.WrapMode
	lineNumbers                    *bool
	colors                         *scheme.Scheme
	query                          *source.SearchQuery
	onChange                       func(textedit.Change)
	onSelection                    func(textedit.Selection)
	onError                        func(error)
	autoPairs, automaticCompletion *bool
	completionProvider             *source.CompletionProvider
}

func SourceView() *SourceViewView                                        { v := &SourceViewView{}; v.Self = v; return v }
func (v *SourceViewView) Document(d *source.Document) *SourceViewView    { v.document = d; return v }
func (v *SourceViewView) ReadOnly(b bool) *SourceViewView                { v.readOnly = b; return v }
func (v *SourceViewView) AcceptsTab(b bool) *SourceViewView              { v.acceptsTab = b; return v }
func (v *SourceViewView) Selection(s textedit.Selection) *SourceViewView { v.selection = &s; return v }
func (v *SourceViewView) IndentOptions(o source.IndentOptions) *SourceViewView {
	v.indent = &o
	return v
}
func (v *SourceViewView) WrapMode(w source.WrapMode) *SourceViewView  { v.wrap = &w; return v }
func (v *SourceViewView) ShowLineNumbers(b bool) *SourceViewView      { v.lineNumbers = &b; return v }
func (v *SourceViewView) Scheme(s *scheme.Scheme) *SourceViewView     { v.colors = s; return v }
func (v *SourceViewView) Search(q source.SearchQuery) *SourceViewView { v.query = &q; return v }
func (v *SourceViewView) OnChange(fn func(textedit.Change)) *SourceViewView {
	v.onChange = fn
	return v
}
func (v *SourceViewView) OnSelection(fn func(textedit.Selection)) *SourceViewView {
	v.onSelection = fn
	return v
}
func (v *SourceViewView) OnError(fn func(error)) *SourceViewView { v.onError = fn; return v }
func (v *SourceViewView) BindSelection(s base.BindState[textedit.Selection]) *SourceViewView {
	if s != nil {
		v.Selection(s.Get()).OnSelection(s.Set)
	}
	return v
}
func (v *SourceViewView) Build() base.View { return v }

func (v *SourceViewView) AutoPairs(enabled bool) *SourceViewView { v.autoPairs = &enabled; return v }
func (v *SourceViewView) AutomaticCompletion(enabled bool) *SourceViewView {
	v.automaticCompletion = &enabled
	return v
}
func (v *SourceViewView) CompletionProvider(provider *source.CompletionProvider) *SourceViewView {
	v.completionProvider = provider
	return v
}

type state struct {
	document    *source.Document
	indent      source.IndentOptions
	wrap        source.WrapMode
	lineNumbers bool
	onChange    func(textedit.Change)
	onSelection func(textedit.Selection)
	onError     func(error)
	handles     signal.Handles
}

func (v *SourceViewView) Mount(ctx base.BuildContext) gui.Widget {
	w := source.NewSourceView()
	s := &state{document: w.Document(), indent: w.IndentOptions(), wrap: w.WrapMode(), lineNumbers: w.ShowLineNumbers()}

	s.handles = signal.Handles{
		w.ConnectChange(func(c textedit.Change) {
			if s.onChange != nil {
				s.onChange(c)
			}
		}),
		w.ConnectSelection(func(value textedit.Selection) {
			if s.onSelection != nil {
				s.onSelection(value)
			}
		}),
		w.ConnectError(func(err error) {
			if s.onError != nil {
				s.onError(err)
			}
		}),
	}
	ctx.SetState(s)
	return w
}
func (v *SourceViewView) Update(ctx base.BuildContext, widget gui.Widget) {
	w, s := widget.(*source.SourceView), ctx.State().(*state)
	s.handles.Block()
	defer s.handles.Unblock()
	d := v.document
	if d == nil {
		d = s.document
	}
	w.SetDocument(d)
	w.SetReadOnly(v.readOnly)
	w.SetAcceptsTab(v.acceptsTab)
	autoPairs, automatic := true, true
	if v.autoPairs != nil {
		autoPairs = *v.autoPairs
	}
	if v.automaticCompletion != nil {
		automatic = *v.automaticCompletion
	}
	w.SetAutoPairs(autoPairs)
	w.SetAutomaticCompletion(automatic)
	w.SetCompletionProvider(v.completionProvider)
	indent := s.indent
	if v.indent != nil {
		indent = *v.indent
	}
	if err := w.SetIndentOptions(indent); err != nil {
		panic(err)
	}
	wrap := s.wrap
	if v.wrap != nil {
		wrap = *v.wrap
	}
	w.SetWrapMode(wrap)
	lines := s.lineNumbers
	if v.lineNumbers != nil {
		lines = *v.lineNumbers
	}
	w.SetShowLineNumbers(lines)
	w.SetScheme(v.colors)
	if v.query != nil {
		w.SetSearch(*v.query)
	}
	if v.selection != nil && w.Selection() != *v.selection {
		w.SetSelection(*v.selection)
	}
	s.onChange = v.onChange
	s.onSelection = v.onSelection
	s.onError = v.onError
}
func (v *SourceViewView) Unmount(ctx base.BuildContext, widget gui.Widget) {
	if s, ok := ctx.State().(*state); ok {
		s.handles.Disconnect()
		widget.(*source.SourceView).SetCompletionProvider(nil)
	}
}

var _ base.WidgetView = (*SourceViewView)(nil)
