package sourceview

import (
	"fmt"

	"github.com/golang-gui/goui/core/signal"
)

// SavePoint is an opaque, process-local history identity and output line-ending
// configuration for one Document. It holds no text, document or native resource.
// It is not a text snapshot or evidence that a file has been written.
type SavePoint struct {
	document uint64
	state    uint64
	ending   LineEnding
}

func (d *Document) currentSavePoint() SavePoint {
	return SavePoint{document: d.id, state: d.model.StateID(), ending: d.lineEnding}
}

// CaptureSavePoint closes the current input undo group and captures its identity.
// It does not change the saved baseline or write a file. Capture before writing;
// call MarkSaved with this point only after the corresponding write succeeds.
func (d *Document) CaptureSavePoint() SavePoint {
	d.model.BreakUndoGroup()
	return d.currentSavePoint()
}

// MarkSaved records a successfully saved point, including an older state while
// edits continue. A foreign/zero point returns an error without changing the
// baseline. The application owns write ordering and file/encoding semantics.
func (d *Document) MarkSaved(point SavePoint) error {
	if point.document == 0 || point.document != d.id {
		return fmt.Errorf("sourceview: save point belongs to another document")
	}
	if d.savedPoint != point {
		d.savedPoint = point
		d.savedPointChanged.Emit()
	}
	return nil
}

// Modified compares current history and output line endings with the saved
// baseline in constant time. Undo/Redo may return to clean; retyping identical
// bytes is still a different history state. New documents start clean.
func (d *Document) Modified() bool { return d.currentSavePoint() != d.savedPoint }

// ConnectModifiedChanged observes changes to Modified, without an initial call.
// Query Modified in the callback: reentrant/blocked transitions may coalesce.
// The returned handle owns all subscriptions; disconnect it when its owner ends.
// This works without mounted views and does not install a permanent subscription.
func (d *Document) ConnectModifiedChanged(fn func()) signal.Handle {
	last := d.Modified()
	observe := func() {
		current := d.Modified()
		if current != last {
			last = current
			fn()
		}
	}
	return signal.Handles{
		d.model.ConnectChange(func(TextChange) { observe() }),
		d.lineEndingChanged.Connect(func(LineEnding, uint64) { observe() }),
		d.savedPointChanged.Connect(observe),
	}
}
