// Package sourceview provides a native GOUI code editor. All widget and document
// operations are sequential and, while mounted, belong on the GUI thread.
package sourceview

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/highlight"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

var nextDocumentID atomic.Uint64

// Document shares normalized text, language, history, output line endings and
// lexical analysis between views. It owns no native resources and needs no Destroy.
type Document struct {
	id                           uint64
	model                        *textedit.Model
	lineEnding                   LineEnding
	lineEndingEpoch              uint64
	lineEndingChanged            signal.Signal2[LineEnding, uint64]
	language                     language.ID
	definition                   *language.Language
	languageEpoch                uint64
	engine                       *highlight.Engine
	connection                   signal.Handle
	views                        map[*SourceView]struct{}
	driver                       gui.Root
	lastFrame                    time.Time
	observed                     uint64
	folding                      *foldingQueue
	foldGeneration, foldRevision uint64
	foldReady                    bool
	foldRanges                   []language.FoldRange
	foldDue                      time.Time
}

// NewDocument detects the source line ending before normalizing the editing
// model to UTF-8/LF. Mixed endings use the most frequent kind; ties use the
// first encountered kind. A document without line breaks defaults to LF.
func NewDocument(text string) *Document {
	ending, _ := DetectLineEnding(text)
	d := NewDocumentWithModel(textedit.NewModel(text))
	d.lineEnding = ending
	return d
}

// NewDocumentWithModel borrows a normalized model and defaults to LF output.
// Its original line endings cannot be recovered; use SetLineEnding when known.
func NewDocumentWithModel(model *textedit.Model) *Document {
	if model == nil {
		model = textedit.NewModel("")
	}
	plain, _ := language.Builtin(language.Plain)
	return &Document{id: nextDocumentID.Add(1), definition: plain, model: model, views: make(map[*SourceView]struct{}), observed: model.Revision()}
}

// LineEnding specifies the UTF-8 sequence used to serialize logical newlines.
// It never changes the model's normalized UTF-8/LF coordinate system.
type LineEnding uint8

const (
	LineEndingLF LineEnding = iota
	LineEndingCRLF
	LineEndingCR
)

// String returns the format name for labels, not its encoded bytes.
func (ending LineEnding) String() string {
	switch ending {
	case LineEndingLF:
		return "LF"
	case LineEndingCRLF:
		return "CRLF"
	case LineEndingCR:
		return "CR"
	default:
		return fmt.Sprintf("LineEnding(%d)", ending)
	}
}

// DetectLineEnding returns the most frequent line ending and whether two or
// more kinds occur. CRLF counts once, ties use the first encountered kind, and
// text without line breaks returns LF. It does not normalize or retain text.
func DetectLineEnding(text string) (ending LineEnding, mixed bool) {
	var counts [3]int
	first := [3]int{-1, -1, -1}
	for at := 0; at < len(text); {
		next := strings.IndexAny(text[at:], "\r\n")
		if next < 0 {
			break
		}
		at += next
		kind := LineEndingLF
		if text[at] == '\r' {
			kind = LineEndingCR
			if at+1 < len(text) && text[at+1] == '\n' {
				kind = LineEndingCRLF
			}
		}
		if counts[kind] == 0 {
			first[kind] = at
		}
		counts[kind]++
		at++
		if kind == LineEndingCRLF {
			at++
		}
	}
	kinds := 0
	for kind, count := range counts {
		if count == 0 {
			continue
		}
		kinds++
		if count > counts[ending] || count == counts[ending] && first[kind] < first[ending] {
			ending = LineEnding(kind)
		}
	}
	return ending, kinds > 1
}

// LineEnding returns the document's serialization format. Model always uses LF.
func (d *Document) LineEnding() LineEnding { return d.lineEnding }

// SetLineEnding configures serialization only. It changes neither text,
// revision nor undo history, and is not an undoable format-conversion command.
func (d *Document) SetLineEnding(ending LineEnding) error {
	if ending > LineEndingCR {
		return fmt.Errorf("sourceview: invalid line ending %d", ending)
	}
	if d.lineEnding != ending {
		d.lineEnding = ending
		d.lineEndingEpoch++
		d.lineEndingChanged.Emit(ending, d.lineEndingEpoch)
	}
	return nil
}

// ConnectLineEndingChanged observes output configuration, separately from model
// edits. The caller owns the returned connection and disconnects it when done.
func (d *Document) ConnectLineEndingChanged(fn func(LineEnding)) signal.Handle {
	return d.lineEndingChanged.Connect(func(ending LineEnding, epoch uint64) {
		if d.lineEndingEpoch == epoch {
			fn(ending)
		}
	})
}

const documentWriteChunk = 32 << 10

// WriteTo writes UTF-8 using LineEnding, preserving every logical newline,
// including their absence or repetition at EOF. It uses bounded chunks and
// returns bytes actually written. Output is not an atomic file replacement.
// Access is sequential like Model. If a writer callback changes the model or
// line-ending configuration, it stops with ErrOperationChanged after that write.
func (d *Document) WriteTo(w io.Writer) (int64, error) {
	model, revision, epoch := d.model, d.model.Revision(), d.lineEndingEpoch
	ending := "\n"
	if d.lineEnding == LineEndingCRLF {
		ending = "\r\n"
	} else if d.lineEnding == LineEndingCR {
		ending = "\r"
	}
	var written int64
	for at, length := 0, model.Len(); at < length; {
		end := at + min(documentWriteChunk, length-at)
		for !model.IsRuneBoundary(end) {
			end--
		}
		text, err := model.Slice(textedit.Range{Start: at, End: end})
		if err != nil {
			return written, err
		}
		if ending != "\n" {
			text = strings.ReplaceAll(text, "\n", ending)
		}
		n, err := io.WriteString(w, text)
		written += int64(n)
		if err != nil {
			return written, err
		}
		if n != len(text) {
			return written, io.ErrShortWrite
		}
		if model.Revision() != revision || d.lineEndingEpoch != epoch {
			return written, ErrOperationChanged
		}
		at = end
	}
	return written, nil
}

var _ io.WriterTo = (*Document)(nil)

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
	d.requestFolding(true)
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
		v.clearFolds()
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
	if d.foldReady && d.foldRevision == d.model.Revision() {
		v.acceptFolds(d.foldRanges)
	} else {
		d.requestFolding(true)
	}

}
func (d *Document) release(v *SourceView) {
	if _, ok := d.views[v]; !ok {
		return
	}
	delete(d.views, v)
	if len(d.views) == 0 {
		d.stopFolding()
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
		if q := queues[d.driver]; q != nil && q.analysisTimer != nil {
			q.requestAnalysis()
			return
		}
		_ = d.driver.RequestPaint()
	}
}
func (d *Document) changed(c textedit.Change) {
	if d.engine == nil {
		return
	}
	d.requestFolding(false)
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
