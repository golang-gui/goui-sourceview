package sourceview

import (
	"fmt"
	"slices"

	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui/textedit"
)

// LineMarkID identifies a mark for the lifetime of its owning Document.
// Zero is never allocated. IDs are not reused after removal.
type LineMarkID uint64

// LineMarkKind selects a built-in glyph, not debugger behavior.
type LineMarkKind uint8

const (
	MarkBreakpoint LineMarkKind = iota
	MarkConditionalBreakpoint
	MarkLogpoint
	MarkBookmark
)

// LineMarkState describes the state supplied by the application.
type LineMarkState uint8

const (
	MarkEnabled LineMarkState = iota
	MarkDisabled
	MarkPending
	MarkRejected
)

// LineMark describes a logical, zero-based line. Label is application-owned
// accessible text; an empty label uses the localized built-in description.
type LineMark struct {
	Line  int
	Kind  LineMarkKind
	State LineMarkState
	Label string
}

// LineMarkInfo is a detached snapshot. An invalid mark retains its identity
// and description but has Line=-1 until the application updates or removes it.
type LineMarkInfo struct {
	LineMark
	ID    LineMarkID
	Valid bool
}

type trackedLineMark struct {
	mark     LineMark
	offset   int
	revision uint64
	valid    bool
}

func (d *Document) validateLineMark(mark LineMark) error {
	if mark.Line < 0 || mark.Line >= d.model.LineCount() {
		return fmt.Errorf("sourceview: invalid mark line %d", mark.Line)
	}
	if mark.Kind > MarkBookmark || mark.State > MarkRejected {
		return fmt.Errorf("sourceview: invalid line mark kind or state")
	}
	return nil
}

// AddLineMark adds an independently tracked mark. It does not alter text or
// undo history. Multiple marks may occupy the same line.
func (d *Document) AddLineMark(mark LineMark) (LineMarkID, error) {
	if err := d.validateLineMark(mark); err != nil {
		return 0, err
	}
	if d.marks == nil {
		d.marks = make(map[LineMarkID]*trackedLineMark)
	}
	d.nextMark++
	id := d.nextMark
	r, _ := d.model.LineRange(mark.Line)
	d.marks[id] = &trackedLineMark{mark, r.Start, d.model.Revision(), true}
	if d.markConnection == nil {
		d.markConnection = d.model.ConnectChange(d.mapLineMarks)
	}
	d.lineMarksChanged()
	return id, nil
}

// UpdateLineMark replaces a mark's description and anchor, including an
// invalid anchor. Unknown IDs and invalid descriptions leave the mark unchanged.
func (d *Document) UpdateLineMark(id LineMarkID, mark LineMark) error {
	if d.marks[id] == nil {
		return fmt.Errorf("sourceview: unknown line mark %d", id)
	}
	if err := d.validateLineMark(mark); err != nil {
		return err
	}
	r, _ := d.model.LineRange(mark.Line)
	d.marks[id] = &trackedLineMark{mark, r.Start, d.model.Revision(), true}
	d.lineMarksChanged()
	return nil
}

// RemoveLineMark reports whether a mark existed. Removing the last mark also
// disconnects the model observer, independently of any mounted views.
func (d *Document) RemoveLineMark(id LineMarkID) bool {
	if d.marks[id] == nil {
		return false
	}
	delete(d.marks, id)
	if len(d.marks) == 0 && d.markConnection != nil {
		d.markConnection.Disconnect()
		d.markConnection = nil
	}
	d.lineMarksChanged()
	return true
}

// LineMarks returns snapshots ordered by ID. During a reentrant model change,
// anchors awaiting a newer change notification are temporarily reported invalid.
// Callers should refresh on ConnectLineMarksChanged rather than retain line numbers.
func (d *Document) LineMarks() []LineMarkInfo {
	marks := make([]LineMarkInfo, 0, len(d.marks))
	for id, entry := range d.marks {
		info := LineMarkInfo{LineMark: entry.mark, ID: id, Valid: entry.valid && entry.revision == d.model.Revision()}
		info.Line = -1
		if info.Valid {
			info.Line = d.model.LineAt(entry.offset)
		}
		marks = append(marks, info)
	}
	slices.SortFunc(marks, func(a, b LineMarkInfo) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return marks
}

// ConnectLineMarksChanged observes additions, updates, removals and text-driven
// anchor changes. The caller owns the connection. Callbacks may edit the model.
func (d *Document) ConnectLineMarksChanged(fn func()) signal.Handle {
	return d.marksChanged.Connect(fn)
}

func (d *Document) lineMarksChanged() {
	for _, mark := range d.marks {
		if mark.revision != d.model.Revision() {
			return
		}
	}
	for view := range d.views {
		view.RequestPaint()
	}
	d.marksChanged.Emit()
}

// Mapping uses change coordinates, never the possibly newer model contents.
// Insertions at an anchor use right gravity; deletion covering it invalidates it.
func mapLineAnchor(offset int, valid bool, c textedit.Change) (int, bool) {
	if c.Reset {
		return offset, false
	}
	for _, edit := range c.Edits {
		if edit.Range.Start <= offset && offset < edit.Range.End {
			valid = false
		}
		offset = textedit.MapOffset(offset, edit)
	}
	return offset, valid
}

func (d *Document) mapLineMarks(c textedit.Change) {
	changed := false
	for _, mark := range d.marks {
		if mark.revision >= c.Revision {
			continue
		}
		mark.offset, mark.valid = mapLineAnchor(mark.offset, mark.valid, c)
		mark.revision = c.Revision
		changed = true
	}
	if changed && c.Revision == d.model.Revision() {
		d.lineMarksChanged()
	}
}
