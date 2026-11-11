// Copyright (c) 2026 xuges@qq.com
// SPDX-License-Identifier: MIT
//
// Adapted from GOUI v0.7.0 public text facilities; maintained by SourceView.

package text

import (
	"fmt"
	"slices"
	"sort"
	"unicode/utf8"
)

// BatchOptions records view-independent history metadata. BeforeSelection uses
// the original document; AfterSelection uses the normalized resulting document.
// Selections are optional and copied. A batch never merges with typing history.
type BatchOptions struct {
	Source                            *EditSource
	BeforeSelection                   *Selection
	AfterSelection                    *Selection
	BeforeSelections, AfterSelections []SelectionState
}

// ApplyBatch applies sorted, non-overlapping edits in original-document byte
// coordinates. It validates everything before changing content, then commits
// one revision and undo item. Same-position insertions must be combined first.
// Change.Edits records the actual right-to-left execution order.
func (m *Model) ApplyBatch(edits []Edit, options BatchOptions) error {
	work := make([]Edit, 0, len(edits))
	resultLength := m.Len()
	for i, edit := range edits {
		if err := m.validateRange(edit.Range); err != nil {
			return err
		}
		if i > 0 {
			prev := edits[i-1].Range
			if edit.Range.Start < prev.End || edit.Range.Start < prev.Start ||
				edit.Range.Start == prev.Start {
				return fmt.Errorf("text model: batch edits must be sorted and unambiguous")
			}
		}
		edit.Text = Normalize(edit.Text)
		resultLength -= edit.Range.End - edit.Range.Start
		if len(edit.Text) > int(^uint(0)>>1)-resultLength {
			return fmt.Errorf("text model: batch result is too large")
		}
		resultLength += len(edit.Text)
		if edit.Range.Start != edit.Range.End || edit.Text != "" {
			work = append(work, edit)
		}
	}
	if s := options.BeforeSelection; s != nil {
		if err := m.validateRange(s.Range()); err != nil {
			return fmt.Errorf("text model: batch before selection: %w", err)
		}
	}
	if s := options.AfterSelection; s != nil {
		for _, pos := range []int{s.Anchor, s.Caret} {
			if pos < 0 || pos > resultLength || !m.batchBoundary(pos, work) {
				return fmt.Errorf("text model: invalid batch after selection")
			}
		}
	}
	before, after := slices.Clone(options.BeforeSelections), slices.Clone(options.AfterSelections)
	if len(before) == 0 && options.BeforeSelection != nil {
		before = []SelectionState{{Selection: *options.BeforeSelection}}
	}
	if len(after) == 0 && options.AfterSelection != nil {
		after = []SelectionState{{Selection: *options.AfterSelection}}
	}
	for _, s := range before {
		if err := m.validateRange(s.Range()); err != nil {
			return fmt.Errorf("text model: batch before selections: %w", err)
		}
	}
	boundary := m.batchBoundaries(work)
	for _, s := range after {
		for _, pos := range []int{s.Anchor, s.Caret} {
			if pos < 0 || pos > resultLength || !boundary(pos) {
				return fmt.Errorf("text model: invalid batch after selections")
			}
		}
	}
	if len(work) == 0 {
		return nil
	}
	step := textStep{
		batch:            make([]textStep, 0, len(work)),
		beforeSelections: before,
		afterSelections:  after,
	}
	changes := make([]Edit, 0, len(work))
	for i := len(work) - 1; i >= 0; i-- {
		edit := work[i]
		part := textStep{start: edit.Range.Start, oldEnd: edit.Range.End}
		if edit.Range.End > edit.Range.Start {
			part.removed = m.tree.deleteRange(edit.Range.Start, edit.Range.End)
		}
		if edit.Text != "" {
			part.inserted = m.tree.insertText(edit.Range.Start, edit.Text)
		}
		step.batch = append(step.batch, part)
		changes = append(changes, edit)
	}
	m.groupOpen = false
	m.lastEditor = options.Source
	m.undoSteps = append(m.undoSteps, step)
	m.redoSteps = nil
	m.revision++
	m.notify(Change{Revision: m.revision, Edits: changes})
	return nil
}

func copySelection(s *Selection) *Selection {
	if s == nil {
		return nil
	}
	result := *s
	return &result
}

// Check a boundary in the virtual result without constructing that result or
// mutating the tree. Gaps retain their original UTF-8 boundaries.
func (m *Model) batchBoundary(pos int, edits []Edit) bool {
	old, current := 0, 0
	for _, edit := range edits {
		gap := edit.Range.Start - old
		if pos <= current+gap {
			return m.IsRuneBoundary(old + pos - current)
		}
		current += gap
		if pos <= current+len(edit.Text) {
			off := pos - current
			return off == len(edit.Text) || utf8.RuneStart(edit.Text[off])
		}
		current += len(edit.Text)
		old = edit.Range.End
	}
	return m.IsRuneBoundary(old + pos - current)
}

func (m *Model) applyBatchHistory(step textStep, undo bool) HistoryResult {
	changes := make([]Edit, 0, len(step.batch))
	for index := range step.batch {
		i := index
		if undo {
			i = len(step.batch) - 1 - index
		}
		part := step.batch[i]
		end, insert := part.oldEnd, part.inserted
		if undo {
			end = part.start + pieceRefsLen(part.inserted)
			insert = part.removed
		}
		if end > part.start {
			m.tree.deleteRange(part.start, end)
		}
		m.tree.insertPiecesAt(part.start, insert)
		changes = append(changes, Edit{Range: Range{part.start, end}, Text: m.tree.textOfRefs(insert)})
	}
	var selection *Selection
	var selections []SelectionState
	if undo {
		m.redoSteps = append(m.redoSteps, step)
		selections = step.beforeSelections
	} else {
		m.undoSteps = append(m.undoSteps, step)
		selections = step.afterSelections
	}
	if len(selections) > 0 {
		primary := selections[0].Selection
		selection = &primary
	}
	m.revision++
	result := HistoryResult{Applied: true, Revision: m.revision, Selection: copySelection(selection), Selections: slices.Clone(selections)}
	m.notify(Change{Revision: m.revision, Edits: changes})
	return result
}

// Map final positions to original gaps or inserted strings in O(log edits),
// without materializing the result or re-scanning the batch for each cursor.
func (m *Model) batchBoundaries(edits []Edit) func(int) bool {
	type segment struct {
		start, end, old int
		text            string
		inserted        bool
	}
	segments := make([]segment, 0, len(edits)*2+1)
	old, at := 0, 0
	for _, e := range edits {
		n := e.Range.Start - old
		if n > 0 {
			segments = append(segments, segment{start: at, end: at + n, old: old})
			at += n
		}
		if len(e.Text) > 0 {
			segments = append(segments, segment{start: at, end: at + len(e.Text), text: e.Text, inserted: true})
			at += len(e.Text)
		}
		old = e.Range.End
	}
	segments = append(segments, segment{start: at, end: at + m.Len() - old, old: old})
	return func(pos int) bool {
		i := sort.Search(len(segments), func(i int) bool { return segments[i].end >= pos })
		if i == len(segments) || pos < segments[i].start {
			return false
		}
		s := segments[i]
		off := pos - s.start
		if s.inserted {
			return off == len(s.text) || utf8.RuneStart(s.text[off])
		}
		return m.IsRuneBoundary(s.old + off)
	}
}
