package sourceview

import (
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

// A one-change permission, consumed before model/selection callbacks can edit
// again. It never grants completion retention to unrelated or reentrant edits.
type completionEdit struct {
	state    operationState
	epoch    uint64
	session  *completionSession
	provider *CompletionProvider
	request  CompletionContext
	edit     textedit.Edit
	applied  bool
}

func (v *SourceView) completionInput(r textedit.Range, text string, atomic bool) (bool, error) {
	var keep *completionEdit
	state, epoch, session, provider := v.operationState(), v.completionEpoch, v.completion, v.completionProvider
	if !atomic && v.projection == nil {
		request, ok := v.liveCompletionRequest()
		s := v.Selection()
		if ok && request.Definition != nil && s.Anchor == s.Caret && r.Start <= s.Caret && r.End >= s.Caret && r.Start >= request.Range.Start && r.End <= request.Range.End {
			word := true
			for _, c := range text {
				if !request.Definition.IsWord(c) {
					word = false
					break
				}
			}
			if word && state.valid(v) && epoch == v.completionEpoch && session == v.completion && provider == v.completionProvider {
				keep = &completionEdit{state: state, epoch: epoch, session: session, provider: provider, request: request, edit: textedit.Edit{Range: r, Text: text}}
			}
		}
	}
	// IsWord is an application callback. A context replacement during that
	// query must not turn the old text input into an edit of the new context.
	if !state.valid(v) || epoch != v.completionEpoch || session != v.completion || provider != v.completionProvider {
		return true, ErrOperationChanged
	}
	err := v.replaceKeepingCompletion(r, text, atomic, keep)
	if keep == nil {
		return false, err
	}
	expected := keep.state
	expected.revision++
	expected.selection++
	if err != nil || !keep.applied {
		if v.completion == keep.session && v.completionEpoch == keep.epoch {
			v.closeCompletion()
		}
		return true, err
	}
	if !expected.valid(v) || keep.epoch != v.completionEpoch || v.completion != keep.session || v.completionProvider != keep.provider || !v.mounted || !v.Focused() || v.editable() != nil {
		return true, err
	}
	c, ok := v.completionContext(keep.request.Explicit)
	if !expected.valid(v) || keep.epoch != v.completionEpoch {
		return true, err
	}
	want := keep.request.Range
	want.End += len(text) - (r.End - r.Start)
	if !ok || c.Range != want || c.Definition != keep.request.Definition || c.Range.Start == c.Range.End {
		v.closeCompletion()
		return true, nil
	}
	c.Trigger = text
	return true, v.refreshCompletions(expected, c)
}

func (v *SourceView) liveCompletionRequest() (CompletionContext, bool) {
	if s := v.completion; s != nil {
		if !s.state.valid(v) || !v.completionLanguageCurrent(s) {
			return CompletionContext{}, false
		}
		return CompletionContext{Definition: s.definition, Range: s.range_, Explicit: s.explicit}, true
	}
	if q := v.completionQueue; q != nil {
		job := q.next
		if job == nil {
			job = q.active
		}
		if job != nil && job.state.valid(v) {
			c := job.request
			return CompletionContext{Definition: c.Definition, Range: c.Range, Explicit: c.Explicit}, true
		}
	}
	return CompletionContext{}, false
}

func (v *SourceView) retainCompletionChange(c textedit.Change) bool {
	keep := v.completionEdit
	v.completionEdit = nil
	if keep == nil {
		return false
	}
	expected := keep.state
	expected.revision++
	if c.Reset || c.Revision != expected.revision || len(c.Edits) != 1 || c.Edits[0] != keep.edit || !expected.valid(v) || keep.epoch != v.completionEpoch || keep.session != v.completion || keep.provider != v.completionProvider {
		return false
	}
	keep.applied = true
	if s := keep.session; s != nil {
		s.range_.End += len(keep.edit.Text) - (keep.edit.Range.End - keep.edit.Range.Start)
		s.state = v.operationState()
	}
	return true
}

// Text arrives separately via IMClient/CommitText. Do not dismiss on its
// preceding printable key, including Shift and layout-specific unknown keys.
func completionTextKey(e events.KeyEvent) bool {
	if e.Modifiers&^(events.ModifierShift|events.ModifierAltGraph) != 0 {
		return false
	}
	return e.Key == events.KeyUnknown || e.Key == events.KeyShift || e.Key == events.KeyAltGraph || e.Key == events.KeySpace || e.Key >= events.KeyA && e.Key <= events.KeyNumpadDecimal
}
