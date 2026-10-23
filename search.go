package sourceview

import (
	"fmt"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/gui/textedit"
)

type SearchQuery struct {
	Text                         string
	IgnoreCase, WholeWord, Regex bool
}

type searchMatch struct {
	raw                      textedit.Range
	revision, selectionEpoch uint64
}

func (v *SourceView) Search() SearchQuery { return v.query }

func (v *SourceView) SetSearch(q SearchQuery) {
	q.Text = textedit.Normalize(q.Text)
	if q == v.query {
		return
	}
	v.query = q
	v.search = editor.Compile(editor.Query{Text: q.Text, IgnoreCase: q.IgnoreCase, WholeWord: q.WholeWord, Regex: q.Regex})
	v.search.SetRange(v.searchRange)
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
			if match.raw.Start == match.raw.End {
				start--
			}
		} else {
			start = match.raw.End
			if match.raw.Start == match.raw.End {
				start++
			}
		}
	}
	return v.findFrom(start, backward)
}

// findFrom includes the starting position, unlike advancing past a current
// match. FindBar uses it to reconcile results after changing the search scope.
func (v *SourceView) findFrom(start int, backward bool) (textedit.Range, bool) {
	r, ok := v.search.Find(v.document.model, start, backward)
	if ok {
		v.document.model.BreakUndoGroup()
		d, revision, search := v.document, v.document.model.Revision(), v.search
		scope, limited := v.SearchRange()
		expectedSelectionEpoch := v.selectionEpoch + 1
		v.setSelection(textedit.Selection{Anchor: r.Start, Caret: r.End}, false, true)
		actual, scoped := v.SearchRange()
		if !v.Destroyed() && v.document == d && v.search == search && actual == scope && scoped == limited && d.model.Revision() == revision && v.selectionEpoch == expectedSelectionEpoch {
			v.currentMatch = &searchMatch{raw: r, revision: revision, selectionEpoch: v.selectionEpoch}
		}
	}
	return r, ok
}

func (v *SourceView) ReplaceCurrent(text string) (bool, error) {
	if err := v.editable(); err != nil {
		return false, err
	}
	if err := v.SearchError(); err != nil {
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
	text, err := v.search.Replacement(v.document.model, raw, text)
	if err != nil {
		return false, err
	}
	return true, v.replace(raw, text, true)
}

func (v *SourceView) ReplaceAll(text string) (int, error) {
	if err := v.editable(); err != nil {
		return 0, err
	}
	if err := v.SearchError(); err != nil {
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

// SearchError reports a malformed regular expression. Invalid queries retain
// their text for editing, produce no matches and never mutate the document.
func (v *SourceView) SearchError() error { return v.search.Err() }

// HighlightSearchMatches reports whether search backgrounds are drawn.
// New views highlight matches, including searches made without a FindBar.
func (v *SourceView) HighlightSearchMatches() bool { return v.highlightSearchMatches }

// SetHighlightSearchMatches changes presentation only. Query, navigation,
// replacement and ordinary text selection are unaffected. FindBar enables it
// on open and disables it when closing or releasing an open search session.
func (v *SourceView) SetHighlightSearchMatches(highlight bool) {
	if !v.Destroyed() && v.highlightSearchMatches != highlight {
		v.highlightSearchMatches = highlight
		v.RequestPaint()
	}
}

// SearchRange is independent of the current selection and FindNext results.
func (v *SourceView) SearchRange() (textedit.Range, bool) {
	if v.searchRange == nil {
		return textedit.Range{}, false
	}
	return *v.searchRange, true
}

// SetSearchRange copies a validated byte range. nil searches the whole document.
// Regex anchors retain the full-document context. Empty ranges may match empty
// expressions; no caller-owned pointer is retained.
func (v *SourceView) SetSearchRange(r *textedit.Range) error {
	if v.Destroyed() {
		return nil
	}
	if r != nil {
		if _, err := v.document.model.Slice(*r); err != nil {
			return fmt.Errorf("sourceview: search range: %w", err)
		}
	}
	if r == nil && v.searchRange == nil || r != nil && v.searchRange != nil && *r == *v.searchRange {
		return nil
	}
	v.searchRange = nil
	if r != nil {
		copy := *r
		v.searchRange = &copy
	}
	if v.search != nil {
		v.search.SetRange(v.searchRange)
	}
	v.currentMatch = nil
	v.RequestPaint()
	v.searchState.Emit()
	return nil
}

func (v *SourceView) mapSearchRange(c textedit.Change) {
	if v.searchRange == nil {
		return
	}
	if c.Reset {
		v.searchRange = nil
	} else {
		for _, e := range c.Edits {
			start := v.searchRange.Start
			v.searchRange.Start = textedit.MapOffset(start, e)
			if start == e.Range.Start {
				v.searchRange.Start = e.Range.Start
			}
			v.searchRange.End = textedit.MapOffset(v.searchRange.End, e)
		}
	}
	if v.search != nil {
		v.search.SetRange(v.searchRange)
	}
}
