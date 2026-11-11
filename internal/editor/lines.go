package editor

import (
	"fmt"

	textcore "github.com/golang-gui/goui-sourceview/internal/text"
)

const lineInsertLimit = 32 << 20

// SelectedLines returns logical rows. A nonempty selection ending at the start
// of a later line does not include that line. Validation never copies its text.
func SelectedLines(m *textcore.Model, s textcore.Selection) (first, last int, err error) {
	r := s.Range()
	if r.Start < 0 || r.End > m.Len() || !m.IsRuneBoundary(r.Start) || !m.IsRuneBoundary(r.End) {
		return 0, 0, fmt.Errorf("sourceview: invalid line selection")
	}
	first, last = m.LineAt(r.Start), m.LineAt(r.End)
	if r.End > r.Start && last > first {
		end, _ := m.LineRange(last)
		if r.End == end.Start {
			last--
		}
	}
	return first, last, nil
}

func selectedLineSpan(m *textcore.Model, s textcore.Selection) (first, last int, span textcore.Range, err error) {
	first, last, err = SelectedLines(m, s)
	if err == nil {
		a, _ := m.LineRange(first)
		b, _ := m.LineRange(last)
		span = textcore.Range{Start: a.Start, End: b.End}
	}
	return
}

func shiftLineSelection(s textcore.Selection, delta, length int) textcore.Selection {
	// A selection may include the LF after the block. When moving it to EOF,
	// there is no following LF to include; preserve its direction and clamp it.
	return textcore.Selection{Anchor: min(length, s.Anchor+delta), Caret: min(length, s.Caret+delta)}
}

// MoveLines exchanges the selected block with one adjacent logical line. Edits
// move the neighbor instead of replacing the block, retaining interior anchors
// in other views under Model's ordinary change mapping. Indentation is literal.
func MoveLines(m *textcore.Model, s textcore.Selection, down bool) (Plan, error) {
	p := Plan{Before: s, After: s}
	first, last, span, err := selectedLineSpan(m, s)
	if err != nil {
		return p, err
	}
	if !down && first == 0 || down && last == m.LineCount()-1 {
		return p, nil
	}
	row := first - 1
	if down {
		row = last + 1
	}
	neighbor, _ := m.LineRange(row)
	if down && span.Start == span.End {
		// An empty block would otherwise insert and delete at the same byte.
		// Move just its LF across the neighbor, leaving the neighbor untouched.
		p.Edits = []textcore.Edit{
			{Range: textcore.Range{Start: span.Start, End: span.Start + 1}},
			{Range: textcore.Range{Start: neighbor.End, End: neighbor.End}, Text: "\n"},
		}
	} else {
		if neighbor.End-neighbor.Start >= lineInsertLimit {
			return p, ErrOperationLimit
		}
		text, err := m.Slice(neighbor)
		if err != nil {
			return p, err
		}
		if down {
			p.Edits = []textcore.Edit{
				{Range: textcore.Range{Start: span.Start, End: span.Start}, Text: text + "\n"},
				{Range: textcore.Range{Start: span.End, End: neighbor.End}},
			}
		} else {
			p.Edits = []textcore.Edit{
				{Range: textcore.Range{Start: neighbor.Start, End: span.Start}},
				{Range: textcore.Range{Start: span.End, End: span.End}, Text: "\n" + text},
			}
		}
	}
	delta := neighbor.End - neighbor.Start + 1
	if !down {
		delta = -delta
	}
	p.After = shiftLineSelection(s, delta, m.Len())
	return p, nil
}

// DuplicateLines copies complete logical lines, keeping the corresponding
// selection in the new copy. It does not read or change the clipboard.
func DuplicateLines(m *textcore.Model, s textcore.Selection, down bool) (Plan, error) {
	p := Plan{Before: s, After: s}
	_, _, span, err := selectedLineSpan(m, s)
	if err != nil {
		return p, err
	}
	if span.End-span.Start >= lineInsertLimit {
		return p, ErrOperationLimit
	}
	text, err := m.Slice(span)
	if err != nil {
		return p, err
	}
	edit := textcore.Edit{Range: textcore.Range{Start: span.Start, End: span.Start}, Text: text + "\n"}
	if down {
		edit.Range = textcore.Range{Start: span.End, End: span.End}
		edit.Text = "\n" + text
		p.After = shiftLineSelection(s, len(edit.Text), m.Len()+len(edit.Text))
	}
	p.Edits = []textcore.Edit{edit}
	return p, nil
}

// DeleteLines removes complete logical lines and collapses the selection at the
// deletion boundary. At EOF it consumes the preceding LF, avoiding a spare row.
func DeleteLines(m *textcore.Model, s textcore.Selection) (Plan, error) {
	p := Plan{Before: s, After: s}
	first, last, span, err := selectedLineSpan(m, s)
	if err != nil {
		return p, err
	}
	if last+1 < m.LineCount() {
		span.End++
	} else if first > 0 {
		span.Start--
	}
	if span.Start == span.End {
		return p, nil
	}
	p.Edits = []textcore.Edit{{Range: span}}
	p.After = textcore.Selection{Anchor: span.Start, Caret: span.Start}
	return p, nil
}
