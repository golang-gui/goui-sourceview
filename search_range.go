package sourceview

import (
	"fmt"

	"github.com/golang-gui/goui/gui/textedit"
)

// SearchError reports a malformed regular expression. Invalid queries retain
// their text for editing, produce no matches and never mutate the document.
func (v *SourceView) SearchError() error { return v.search.Err() }

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
