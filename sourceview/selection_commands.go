package sourceview

import (
	"sort"
	"strings"
	"time"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/internal/view"
	"github.com/golang-gui/goui/platform/events"
)

// AddCursor adds a primary cursor at the same horizontal position on the next
// or previous display line. The mounted view prepares missing geometry in turns.
func (v *SourceView) AddCursor(below bool) error {
	c := CommandAddCursorAbove
	if below {
		c = CommandAddCursorBelow
	}
	return v.queueSelectionCommand(c)
}
func (v *SourceView) SelectNextOccurrence() error {
	return v.queueSelectionCommand(CommandSelectNextOccurrence)
}
func (v *SourceView) SelectAllOccurrences() error {
	return v.queueSelectionCommand(CommandSelectAllOccurrences)
}
func (v *SourceView) SplitSelectionLines() error {
	return v.queueSelectionCommand(CommandSplitSelectionLines)
}
func (v *SourceView) queueSelectionCommand(c Command) error {
	if v.Destroyed() {
		return ErrDisabled
	}
	if !v.mounted {
		return ErrNotMounted
	}
	v.cancelPreedit()
	v.document.model.BreakUndoGroup()
	if v.pendingInput == nil {
		v.pendingInput = &inputBatch{state: v.operationState(), bindings: v.keyBindingsEpoch, readOnly: v.readOnly, direct: true}
	}
	v.pendingInput.actions = append(v.pendingInput.actions, deferredInput{selectionCommand: c})
	v.RequestPaint()
	return nil
}

type selectionQuery struct {
	pattern        string
	at, end, phase int
	results        []cursorState
	original       []cursorState
	column         bool
}

func (v *SourceView) advanceSelectionCommand(q *inputBatch, c Command, deadline time.Time) (bool, error) {
	if q.selecting == nil {
		q.selecting = &selectionQuery{original: v.cursors()}
	}
	job := q.selecting
	switch c {
	case CommandAddCursorAbove, CommandAddCursorBelow:
		key := events.KeyArrowUp
		if c == CommandAddCursorBelow {
			key = events.KeyArrowDown
		}
		primary := job.original[0]
		primary.selection.Anchor = primary.selection.Caret
		next, err := v.navigateCursor(primary, key, false)
		if err != nil {
			return false, err
		}
		if len(job.original) >= maxSelections {
			return false, ErrOperationLimit
		}
		after := append([]cursorState{next}, job.original...)
		v.setCursorStatesKeepingCompletion(normalizeCursors(after), true, nil)
		return true, nil
	case CommandSplitSelectionLines:
		if job.results == nil {
			job.results = make([]cursorState, 0, len(job.original))
		}
		for count := 0; job.at < len(job.original) && count < 16; count++ {
			if !deadline.IsZero() && !time.Now().Before(deadline) {
				return false, nil
			}
			s := job.original[job.at].selection
			r := s.Range()
			first, last, err := editor.SelectedLines(v.document.model, s)
			if err != nil {
				return false, err
			}
			if last-first+1+len(job.results) > maxSelections {
				return false, ErrOperationLimit
			}
			primaryStart := len(job.results)
			for row := first; row <= last; row++ {
				line, _ := v.document.LineRange(row)
				a, b := max(r.Start, line.Start), min(r.End, line.End)
				if a > b {
					a = b
				}
				sel := TextSelection{Anchor: a, Caret: b}
				if s.Anchor > s.Caret {
					sel.Anchor, sel.Caret = sel.Caret, sel.Anchor
				}
				job.results = append(job.results, cursorState{selection: sel})
			}
			if job.at == 0 {
				active := min(last, v.document.model.LineAt(s.Caret)) - first + primaryStart
				job.results[primaryStart], job.results[active] = job.results[active], job.results[primaryStart]
			}
			job.at++
		}
		if job.at < len(job.original) {
			return false, nil
		}
		v.setCursorStatesKeepingCompletion(normalizeCursors(job.results), true, nil)
		return true, nil
	case CommandColumnSelectLeft, CommandColumnSelectRight, CommandColumnSelectUp, CommandColumnSelectDown:
		if !job.column {
			if err := v.beginKeyboardColumn(c); err != nil {
				return false, err
			}
			job.column = true
		}
		column := v.columnSelection
		if column == nil {
			return false, ErrOperationChanged
		}
		done, err := v.advanceColumn(column, deadline)
		if err == view.ErrColumnDirection || err == ErrOperationLimit {
			v.columnSelection = nil
		}
		return done, err
	}
	// Literal occurrence selection scans bounded source windows. It does not
	// populate a whole-document match cache or change the find panel's query.
	if job.pattern == "" {
		s := job.original[0].selection
		if s.Anchor == s.Caret {
			r := editor.WordRange(v.document.model, s.Caret, v.definitionAt(s.Caret).IsWord)
			if r.Start == r.End {
				return true, nil
			}
			if c == CommandSelectNextOccurrence {
				job.original[0].selection = TextSelection{Anchor: r.Start, Caret: r.End}
				v.setCursorStatesKeepingCompletion(normalizeCursors(job.original), true, nil)
				return true, nil
			}
			s = TextSelection{Anchor: r.Start, Caret: r.End}
		}
		if s.Range().End-s.Range().Start > 64<<10 {
			return false, ErrOperationLimit
		}
		job.original[0].selection = s
		text, err := v.document.Slice(s.Range())
		if err != nil {
			return false, err
		}
		if text == "" {
			return true, nil
		}
		job.pattern = text
		job.end = v.document.Len()
		if c == CommandSelectNextOccurrence {
			job.at = s.Range().End
			job.results = job.original
		} else {
			// An explicit primary match can overlap the global scan's first
			// occurrence. Keep it intact and scan either side of its range.
			job.results = []cursorState{{selection: s}}
		}
	}
	for count := 0; count < 4; count++ {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return false, nil
		}
		if job.at >= job.end {
			if c == CommandSelectNextOccurrence && job.phase == 0 {
				job.phase = 1
				job.at = 0
				job.end = job.original[0].selection.Range().Start
				continue
			}
			if c == CommandSelectAllOccurrences && len(job.results) > 0 {
				primary := 0
				for i, s := range job.results {
					if s.selection.Range() == job.original[0].selection.Range() {
						primary = i
						break
					}
				}
				after := append([]cursorState{job.results[primary]}, job.results[:primary]...)
				after = append(after, job.results[primary+1:]...)
				v.setCursorStatesKeepingCompletion(normalizeCursors(after), true, nil)
			}
			return true, nil
		}
		end := min(job.end, job.at+64*1024)
		for end < job.end && !v.document.IsRuneBoundary(end) {
			end++
		}
		readEnd := min(v.document.Len(), end+len(job.pattern))
		for readEnd < v.document.Len() && !v.document.IsRuneBoundary(readEnd) {
			readEnd++
		}
		text, err := v.document.Slice(TextRange{Start: job.at, End: readEnd})
		if err != nil {
			return false, err
		}
		base := job.at
		from := 0
		for from < len(text) {
			at := strings.Index(text[from:], job.pattern)
			if at < 0 {
				break
			}
			at += from
			start := base + at
			if start >= end || start >= job.end {
				break
			}
			match := TextSelection{Anchor: start, Caret: start + len(job.pattern)}
			if c == CommandSelectAllOccurrences {
				primary := job.original[0].selection.Range()
				if match.Anchor < primary.End && match.Caret > primary.Start {
					from = primary.End - base
					continue
				}
				if len(job.results) >= maxSelections {
					return false, ErrOperationLimit
				}
				job.results = append(job.results, cursorState{selection: match})
			} else {
				ranges := v.selectionRanges()
				index := sort.Search(len(ranges), func(i int) bool { return ranges[i].End > match.Anchor })
				overlap := index < len(ranges) && ranges[index].Start < match.Caret
				if !overlap {
					if len(job.original) >= maxSelections {
						return false, ErrOperationLimit
					}
					after := append([]cursorState{{selection: match}}, job.original...)
					v.setCursorStatesKeepingCompletion(normalizeCursors(after), true, nil)
					return true, nil
				}
			}
			from = at + len(job.pattern)
		}
		job.at = max(end, base+from)
	}
	return false, nil
}
func (v *SourceView) beginKeyboardColumn(command Command) error {
	c := v.columnSelection
	if c != nil && c.layout != v.layoutEpoch {
		c = nil
	}
	if c != nil {
		copy := *c
		c = &copy // Do not partially advance an intent if destination geometry is pending.
	}
	if c == nil {
		primary := v.cursors()[0]
		p, g, _, err := v.cursorGeometry(primary)
		if err != nil {
			return err
		}
		if p == nil || g == nil {
			return ErrNotMounted
		}
		rect, ok := v.cachedCaretRect()
		if !ok {
			return errSelectionGeometry
		}
		x := float64(rect.X-v.gutter-padding) + float64(v.offset.X)
		pos := textcore.Position{Offset: primary.selection.Caret, Upstream: primary.upstream}
		c = &columnSelection{state: v.operationState(), layout: v.layoutEpoch, anchor: pos, target: pos, left: x, right: x, before: v.cursors()}
	}
	c.before = v.cursors()
	c.state = v.operationState()
	c.layout = v.layoutEpoch
	c.mouse = false
	c.initialized = false
	c.results = nil
	c.pending = true
	if command == CommandColumnSelectUp || command == CommandColumnSelectDown {
		key := events.KeyArrowUp
		if command == CommandColumnSelectDown {
			key = events.KeyArrowDown
		}
		target := cursorState{selection: TextSelection{Anchor: c.target.Offset, Caret: c.target.Offset}, upstream: c.target.Upstream, preferredX: c.right, preferredSet: true}
		next, err := v.navigateCursor(target, key, false)
		if err != nil {
			return err
		}
		c.target = textcore.Position{Offset: next.selection.Caret, Upstream: next.upstream}
	} else {
		p, g, x, r, err := v.columnFrame(c.anchor.Offset)
		if err != nil {
			return err
		}
		if v.wrap == WrapNone && p.Paged != nil {
			frame, ready, err := p.Paged.ColumnGeometry(c.right, c.right)
			if err != nil {
				return err
			}
			if !ready {
				return errSelectionGeometry
			}
			from := frame.Geometry.HitLine(0, float32(c.right-frame.X))
			// Prepare the active edge and its actual neighboring stops. The anchor
			// can remain many native pages away while the rectangle grows or shrinks.
			detail := p.Paged.GeometryAt(from.Offset)
			if detail == nil {
				return errSelectionGeometry
			}
			g, x = &detail.Geometry, detail.X
		}
		index := g.LineIndex(textcore.Position{Offset: c.anchor.Offset - r.Start, Upstream: c.anchor.Upstream})
		from := g.HitLine(index, float32(c.right-x))
		next, ok := g.Visual(from, command == CommandColumnSelectRight)
		if ok && g.LineIndex(next) == index {
			c.right = x + float64(g.Caret(next).X)
		}
	}
	v.columnSelection = c
	return nil
}
