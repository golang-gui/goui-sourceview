// Package sourceview provides a native GOUI code editor. All widget and document
// operations are sequential and, while mounted, belong on the GUI thread.
package sourceview

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

// Document shares text, language, history and lexical analysis between views.
// It owns no native resources and requires no Destroy.
var nextDocumentID atomic.Uint64

type Document struct {
	id            uint64
	model         *textedit.Model
	language      language.ID
	definition    *language.Language
	languageEpoch uint64
	engine        *highlight.Engine
	connection    signal.Handle
	views         map[*SourceView]struct{}
	driver        gui.Root
	lastFrame     time.Time
	observed      uint64
}

func NewDocument(text string) *Document { return NewDocumentWithModel(textedit.NewModel(text)) }
func NewDocumentWithModel(model *textedit.Model) *Document {
	if model == nil {
		model = textedit.NewModel("")
	}
	plain, _ := language.Builtin(language.Plain)
	return &Document{id: nextDocumentID.Add(1), definition: plain, model: model, views: make(map[*SourceView]struct{}), observed: model.Revision()}
}

// ID is stable for this document lifetime and unique within this process.
func (d *Document) ID() uint64             { return d.id }
func (d *Document) Model() *textedit.Model { return d.model }
func (d *Document) Language() language.ID  { return d.language }
func (d *Document) SetLanguage(id language.ID) error {
	l, ok := language.Builtin(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownLanguage, id)
	}
	d.SetLanguageDefinition(l)
	return nil
}

// LanguageDefinition returns the immutable definition shared by all views.
func (d *Document) LanguageDefinition() *language.Language { return d.definition }

// SetLanguageDefinition binds a definition directly. Nil selects plain text.
// Identity matters: a different definition with the same ID invalidates analysis.
func (d *Document) SetLanguageDefinition(l *language.Language) {
	if l == nil {
		l, _ = language.Builtin(language.Plain)
	}
	if d.definition == l {
		return
	}
	d.definition, d.language = l, l.ID()
	d.languageEpoch++
	if d.engine != nil {
		d.engine.SetDefinition(l)
	}
	views := make([]*SourceView, 0, len(d.views))
	for v := range d.views {
		views = append(views, v)
	}
	for _, v := range views {
		if v.document != d || !v.mounted {
			continue
		}
		v.pairs, v.brackets = nil, bracketState{}
		v.closeCompletion()
		if v.document != d || !v.mounted {
			continue
		}
		v.updateScheme()
		v.RequestPaint()
	}
	d.requestWork()
}

func (d *Document) acquire(v *SourceView) {
	if len(d.views) == 0 {
		d.observed = d.model.Revision()
		d.engine = highlight.NewWithLanguage(d.model, d.definition)
		d.connection = d.model.ConnectChange(d.changed)
	}
	d.views[v] = struct{}{}
	if d.driver == nil {
		d.driver = v.Root()
	}
	v.observed = d.observed
	v.selectionRevision = d.model.Revision()
	v.selection = v.clampSelection(v.selection)
	d.requestWork()
}
func (d *Document) release(v *SourceView) {
	if _, ok := d.views[v]; !ok {
		return
	}
	delete(d.views, v)
	if len(d.views) == 0 {
		d.connection.Disconnect()
		d.connection = nil
		d.engine = nil
		d.driver = nil
	} else if d.driver == v.Root() {
		d.driver = nil
		for other := range d.views {
			if other.Root() != v.Root() {
				d.driver = other.Root()
				break
			}
		}
		if d.driver == nil {
			for other := range d.views {
				d.driver = other.Root()
				break
			}
		}
	}
}
func (d *Document) requestWork() {
	if d.driver != nil {
		_ = d.driver.RequestPaint()
	}
}
func (d *Document) changed(c textedit.Change) {
	if d.engine == nil {
		return
	}
	splices, reset := d.engine.Change(c)
	d.observed = c.Revision
	views := make([]*SourceView, 0, len(d.views))
	for v := range d.views {
		views = append(views, v)
	}
	for _, v := range views {
		if v.mounted && v.document == d {
			v.changed(c, splices, reset)
		}
	}
	d.requestWork()
}
