package sourceview

import (
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

func (v *SourceView) ReplaceSelection(text string) error {
	return v.replace(v.Selection().Range(), text, true)
}
func (v *SourceView) replace(r textedit.Range, text string, atomic bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	text = textedit.Normalize(text)
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return ErrOperationChanged
	}
	if err := v.editable(); err != nil {
		return err
	}
	m, d := v.document.model, v.document
	revision := m.Revision()
	epoch := v.mountEpoch
	before := v.Selection()
	err := m.Apply(textedit.Edit{Range: r, Text: text}, textedit.EditOptions{Source: &v.source, Selection: &before, Atomic: atomic})
	if err == nil && v.document == d && v.mountEpoch == epoch && m.Revision() == revision+1 && v.selectionEpoch == state.selection {
		end := r.Start + len(text)
		v.setSelection(textedit.Selection{Anchor: end, Caret: end}, false, true)
	}
	return err
}
func (v *SourceView) Undo() bool { return v.history(true) }
func (v *SourceView) Redo() bool { return v.history(false) }
func (v *SourceView) history(undo bool) bool {
	if v.editable() != nil {
		return false
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) || v.editable() != nil {
		return false
	}
	d := v.document
	epoch := v.mountEpoch
	var result textedit.HistoryResult
	if undo {
		result = d.model.UndoWithSelection()
	} else {
		result = d.model.RedoWithSelection()
	}
	if result.Applied && result.Selection != nil && v.document == d && v.mountEpoch == epoch && d.model.Revision() == result.Revision && v.selectionEpoch == state.selection {
		v.setSelection(*result.Selection, false, true)
	}
	return result.Applied
}
func (v *SourceView) applyPlan(p editor.Plan) error {
	if err := v.editable(); err != nil {
		return err
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return ErrOperationChanged
	}
	if err := v.editable(); err != nil {
		return err
	}
	d := v.document
	revision := d.model.Revision()
	epoch := v.mountEpoch
	err := p.Apply(d.model, &v.source)
	if err == nil && len(p.Edits) > 0 && v.document == d && v.mountEpoch == epoch && d.model.Revision() == revision+1 && v.selectionEpoch == state.selection {
		v.setSelection(p.After, false, true)
	}
	return err
}
func (v *SourceView) Indent(outdent bool) error {
	p, err := editor.Indent(v.document.model, v.Selection(), editor.IndentOptions{TabWidth: v.indent.TabWidth, IndentWidth: v.indent.IndentWidth, InsertSpaces: v.indent.InsertSpaces}, outdent)
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}
func (v *SourceView) tab(outdent bool) error {
	s := v.Selection()
	if outdent || s.Anchor != s.Caret {
		return v.Indent(outdent)
	}
	text := "\t"
	if v.indent.InsertSpaces {
		column := 0
		r, _ := v.document.model.LineRange(v.document.model.LineAt(s.Caret))
		r.End = s.Caret
		prefix, _ := v.document.model.Slice(r)
		for _, ch := range prefix {
			if ch == '\t' {
				column += v.indent.TabWidth - column%v.indent.TabWidth
			} else {
				column++
			}
		}
		text = strings.Repeat(" ", v.indent.TabWidth-column%v.indent.TabWidth)
	}
	return v.ReplaceSelection(text)
}
func (v *SourceView) Copy() error {
	if v.Selection().Anchor == v.Selection().Caret {
		return nil
	}
	if !v.mounted || gui.App == nil {
		return ErrNotMounted
	}
	text, err := v.document.model.Slice(v.Selection().Range())
	if err != nil {
		return err
	}
	clipboard := gui.App.Clipboard()
	if clipboard == nil {
		return ErrClipboardUnavailable
	}
	return clipboard.SetText(text)
}
func (v *SourceView) Cut() error {
	if err := v.editable(); err != nil {
		return err
	}
	s, d, revision, epoch := v.Selection(), v.document, v.document.model.Revision(), v.selectionEpoch
	if s.Anchor == s.Caret {
		return nil
	}
	if err := v.Copy(); err != nil {
		return err
	}
	if v.document != d || d.model.Revision() != revision || v.selectionEpoch != epoch || !v.mounted {
		return nil
	}
	return v.ReplaceSelection("")
}

// Paste accepts an asynchronous request. Delayed failures are reported through
// ConnectError. A changed document, selection or mount cancels the request.
func (v *SourceView) Paste() error {
	if err := v.editable(); err != nil {
		return err
	}
	if !v.mounted || gui.App == nil {
		return ErrNotMounted
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return ErrOperationChanged
	}
	if err := v.editable(); err != nil {
		return err
	}
	clipboard := gui.App.Clipboard()
	if clipboard == nil {
		return ErrClipboardUnavailable
	}

	v.cancelPaste()
	request := &pasteRequest{view: v, document: v.document, revision: v.document.model.Revision(), mount: v.mountEpoch, selection: v.selectionEpoch, id: v.pasteEpoch}
	v.pendingPaste = request
	clipboard.RequestText(request.complete)
	return nil
}
func (v *SourceView) Search() SearchQuery { return v.query }
func (v *SourceView) SetSearch(q SearchQuery) {
	q.Text = textedit.Normalize(q.Text)
	if q == v.query {
		return
	}
	v.query = q
	v.search = editor.Compile(editor.Query{Text: q.Text, IgnoreCase: q.IgnoreCase, WholeWord: q.WholeWord})
	v.currentMatch = nil
	v.RequestPaint()
	v.searchState.Emit()
}
func (v *SourceView) FindNext(backward bool) (textedit.Range, bool) {
	if v.search.Empty() {
		return textedit.Range{}, false
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return textedit.Range{}, false
	}
	start := v.Selection().Range().End
	if backward {
		start = v.Selection().Range().Start
	}
	if match := v.currentMatch; match != nil && match.revision == v.document.model.Revision() && match.selectionEpoch == v.selectionEpoch {
		if backward {
			start = match.raw.Start
		} else {
			start = match.raw.End
		}
	}
	r, ok := v.search.Find(v.document.model, start, backward)
	if ok {
		v.document.model.BreakUndoGroup()
		d, revision := v.document, v.document.model.Revision()
		expectedSelectionEpoch := v.selectionEpoch + 1
		v.setSelection(textedit.Selection{Anchor: r.Start, Caret: r.End}, false, true)
		if v.document == d && d.model.Revision() == revision && v.selectionEpoch == expectedSelectionEpoch {
			v.currentMatch = &searchMatch{raw: r, revision: revision, selectionEpoch: v.selectionEpoch}
		}
	}
	return r, ok
}
func (v *SourceView) ReplaceCurrent(text string) (bool, error) {
	if err := v.editable(); err != nil {
		return false, err
	}
	if v.search.Empty() {
		return false, nil
	}
	raw := v.Selection().Range()
	if match := v.currentMatch; match != nil && match.revision == v.document.model.Revision() && match.selectionEpoch == v.selectionEpoch {
		raw = match.raw
	}
	if !v.search.Matches(v.document.model, raw) {
		return false, nil
	}
	return true, v.replace(raw, text, true)
}
func (v *SourceView) ReplaceAll(text string) (int, error) {
	if err := v.editable(); err != nil {
		return 0, err
	}
	if v.search.Empty() {
		return 0, nil
	}
	p, err := v.search.ReplaceAll(v.document.model, v.Selection(), text)
	if err != nil {
		return 0, err
	}
	err = v.applyPlan(p)
	return len(p.Edits), err
}

// The platform may retain a callback while a remote owner is unresponsive.
// Cancelling clears widget/document references even before that callback runs.
type pasteRequest struct {
	view                           *SourceView
	document                       *Document
	revision, mount, selection, id uint64
}

func (v *SourceView) cancelPaste() {
	v.pasteEpoch++
	if pending := v.pendingPaste; pending != nil {
		pending.view = nil
		pending.document = nil
		v.pendingPaste = nil
	}
}
func (r *pasteRequest) complete(text string, err error) {
	v, d := r.view, r.document
	r.view = nil
	r.document = nil
	if v == nil {
		return
	}
	if v.pendingPaste != r {
		return
	}
	v.pendingPaste = nil
	if !v.mounted || v.mountEpoch != r.mount || v.pasteEpoch != r.id || v.document != d || d.model.Revision() != r.revision || v.selectionEpoch != r.selection || v.editable() != nil {
		return
	}
	if err != nil {
		v.report(err)
		return
	}
	if text == "" {
		return
	}
	v.report(v.ReplaceSelection(text))
}

// ApplyEdits applies original-coordinate edits as one undo unit, with an
// explicit post-edit selection. Document identity and revision must match.
// All ranges and the selection are validated before mutation by textedit.
func (v *SourceView) ApplyEdits(document *Document, revision uint64, edits []textedit.Edit, after textedit.Selection) error {
	if document != v.document || revision != v.document.model.Revision() {
		return ErrOperationChanged
	}
	return v.applyPlan(editor.Plan{Edits: edits, Before: v.Selection(), After: after})
}
