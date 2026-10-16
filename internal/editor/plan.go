// Package editor plans source edits without depending on Widgets or native resources.
package editor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-gui/goui/gui/textedit"
)

var ErrOperationLimit = errors.New("sourceview: operation budget exceeded")

// IndentOptions controls logical indentation. Distances are not font metrics.
type IndentOptions struct {
	TabWidth, IndentWidth int
	InsertSpaces          bool
}

func (o IndentOptions) Normalized() (IndentOptions, error) {
	if o.TabWidth == 0 {
		o.TabWidth = 4
	}
	if o.IndentWidth == 0 {
		o.IndentWidth = o.TabWidth
	}
	if o.TabWidth < 1 || o.TabWidth > 16 || o.IndentWidth < 1 || o.IndentWidth > 16 {
		return o, fmt.Errorf("sourceview: indentation widths must be between 1 and 16")
	}
	return o, nil
}

// Plan contains original-document edits and explicit history selections.
type Plan struct {
	Edits         []textedit.Edit
	Before, After textedit.Selection
}

func (p Plan) Apply(m *textedit.Model, source *textedit.EditSource) error {
	return m.ApplyBatch(p.Edits, textedit.BatchOptions{Source: source, BeforeSelection: &p.Before, AfterSelection: &p.After})
}
func MapPosition(pos int, edits []textedit.Edit) int {
	delta := 0
	for _, edit := range edits {
		if pos < edit.Range.Start {
			break
		}
		if pos <= edit.Range.End {
			return edit.Range.Start + delta + len(edit.Text)
		}
		delta += len(edit.Text) - (edit.Range.End - edit.Range.Start)
	}
	return pos + delta
}
func mapSelection(s textedit.Selection, edits []textedit.Edit) textedit.Selection {
	return textedit.Selection{Anchor: MapPosition(s.Anchor, edits), Caret: MapPosition(s.Caret, edits)}
}

// Indent plans a single undo unit, retaining selection direction. An end exactly
// at the next line's start excludes that line from a nonempty selection.
func Indent(m *textedit.Model, s textedit.Selection, options IndentOptions, outdent bool) (Plan, error) {
	p := Plan{Before: s, After: s}
	o, err := options.Normalized()
	if err != nil {
		return p, err
	}
	r := s.Range()
	if _, err := m.Slice(r); err != nil {
		return p, err
	}
	first, last := m.LineAt(r.Start), m.LineAt(r.End)
	if r.End > r.Start && last > first {
		lastRange, _ := m.LineRange(last)
		if r.End == lastRange.Start {
			last--
		}
	}
	for row := first; row <= last; row++ {
		rng, _ := m.LineRange(row)
		end := min(rng.End, rng.Start+o.IndentWidth+1)
		for end > rng.Start && !m.IsRuneBoundary(end) {
			end--
		}
		prefix, err := m.Slice(textedit.Range{Start: rng.Start, End: end})
		if err != nil {
			return p, err
		}
		edit := textedit.Edit{Range: textedit.Range{Start: rng.Start, End: rng.Start}}
		if outdent {
			n := 0
			if strings.HasPrefix(prefix, "\t") {
				n = 1
			} else {
				for n < len(prefix) && n < o.IndentWidth && prefix[n] == ' ' {
					n++
				}
			}
			if n == 0 {
				continue
			}
			edit.Range.End += n
		} else if o.InsertSpaces {
			edit.Text = strings.Repeat(" ", o.IndentWidth)
		} else {
			edit.Text = "\t"
		}
		p.Edits = append(p.Edits, edit)
	}
	p.After = mapSelection(s, p.Edits)
	return p, nil
}

// LeadingIndent returns the line's original leading whitespace, bounded at caret.
func LeadingIndent(m *textedit.Model, caret int) string {
	row := m.LineAt(caret)
	r, _ := m.LineRange(row)
	r.End = min(r.End, caret)
	text, err := m.Slice(r)
	if err != nil {
		return ""
	}
	i := 0
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	return text[:i]
}
