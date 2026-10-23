package sourceview

import (
	"github.com/golang-gui/goui/gui"
)

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
