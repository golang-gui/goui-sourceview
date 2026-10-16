package sourceview

import "github.com/golang-gui/goui-sourceview/internal/editor"

// MoveLines moves the current logical line or selected lines one row up/down.
// It preserves indentation and selection direction. A document edge is a no-op.
func (v *SourceView) MoveLines(down bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	p, err := editor.MoveLines(v.document.model, v.Selection(), down)
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}

// DuplicateLines copies the current logical line or selected lines above/below,
// retaining the selection in the new copy. The clipboard is not involved.
func (v *SourceView) DuplicateLines(down bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	p, err := editor.DuplicateLines(v.document.model, v.Selection(), down)
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}

// DeleteLines deletes the current logical line or selected lines, placing the
// caret at the deletion boundary. Deleting an empty document is a no-op.
func (v *SourceView) DeleteLines() error {
	if err := v.editable(); err != nil {
		return err
	}
	p, err := editor.DeleteLines(v.document.model, v.Selection())
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}
