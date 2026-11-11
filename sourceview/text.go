package sourceview

import (
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/core/signal"
)

// TextRange is a half-open range of UTF-8 byte offsets in normalized text.
type TextRange = textcore.Range

// TextSelection preserves selection direction through its anchor and caret.
type TextSelection = textcore.Selection

// TextEdit replaces a byte range with text, normalized to valid UTF-8 and LF.
type TextEdit = textcore.Edit

// TextChange describes one committed revision. Edits use sequential coordinates
// in execution order; Reset requires readers to query the document again.
// Observers must treat Edits as read-only; clone the slice before modifying it.
type TextChange = textcore.Change

// Text materializes the entire normalized document. Prefer Slice for local reads.
func (d *Document) Text() string { return d.model.Text() }

// EqualText compares normalized text without materializing the document.
func (d *Document) EqualText(text string) bool { return d.model.EqualText(text) }

// Len returns the UTF-8 byte length of the normalized document.
func (d *Document) Len() int { return d.model.Len() }

// Revision increases monotonically with committed edits, resets, undo and redo.
func (d *Document) Revision() uint64 { return d.model.Revision() }

// LineCount returns the logical line count, including the final empty line.
func (d *Document) LineCount() int { return d.model.LineCount() }

// LineRange returns a logical line's byte range, excluding its LF separator.
func (d *Document) LineRange(line int) (TextRange, error) { return d.model.LineRange(line) }

// LineAt returns the logical line containing offset, clamped to the document.
func (d *Document) LineAt(offset int) int { return d.model.LineAt(offset) }

// Slice reads a valid range without copying unrelated document text.
func (d *Document) Slice(r TextRange) (string, error) { return d.model.Slice(r) }

// IsRuneBoundary reports whether offset is a valid UTF-8 boundary in the document.
func (d *Document) IsRuneBoundary(offset int) bool { return d.model.IsRuneBoundary(offset) }

// SetText normalizes text and resets history when content changes. It preserves
// the selected output LineEnding. Identical normalized content is a no-op.
func (d *Document) SetText(text string) { d.model.SetText(text) }

// Replace replaces a valid range. Consecutive compatible document edits may merge
// into one undo item. View editing also associates its selection and edit source.
func (d *Document) Replace(r TextRange, text string) error { return d.model.Replace(r, text) }

// ReplaceAtomic replaces a valid range as a separate undo item.
func (d *Document) ReplaceAtomic(r TextRange, text string) error {
	return d.model.ReplaceAtomic(r, text)
}

// ApplyEdits validates sorted, non-overlapping original-coordinate edits and
// commits one revision and undo item. Same-position insertions must be combined.
// Document edits record no view selection; use SourceView.ApplyEdits to supply it.
func (d *Document) ApplyEdits(edits []TextEdit) error {
	return d.model.ApplyBatch(edits, textcore.BatchOptions{})
}

// Undo reverts the latest edit. SourceView.Undo also restores its saved selection.
func (d *Document) Undo() bool { return d.model.Undo() }

// Redo reapplies the latest undone edit.
func (d *Document) Redo() bool { return d.model.Redo() }

// CanUndo reports whether an undo item is available.
func (d *Document) CanUndo() bool { return d.model.CanUndo() }

// CanRedo reports whether a redo item is available.
func (d *Document) CanRedo() bool { return d.model.CanRedo() }

// BreakUndoGroup prevents the next compatible edit from joining the current item.
func (d *Document) BreakUndoGroup() { d.model.BreakUndoGroup() }

// ClearHistory releases undo and redo items without changing text or revision.
func (d *Document) ClearHistory() { d.model.ClearHistory() }

// ConnectChange observes committed changes on the document's owning sequence.
// Handlers may edit again; notifications retain revision order, while querying
// the document from a handler may already observe a later revision.
func (d *Document) ConnectChange(fn func(TextChange)) signal.Handle {
	return d.model.ConnectChange(fn)
}
