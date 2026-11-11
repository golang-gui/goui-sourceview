package sourceview

import (
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"slices"
	"sort"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/gui"
)

type selectedLineGroup struct {
	first, last int
	members     []int
}

func (v *SourceView) selectedLineGroups(cursors []cursorState) ([]selectedLineGroup, error) {
	groups := make([]selectedLineGroup, 0, len(cursors))
	for i, c := range cursors {
		first, last, err := editor.SelectedLines(v.document.model, c.selection)
		if err != nil {
			return nil, err
		}
		groups = append(groups, selectedLineGroup{first, last, []int{i}})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].first < groups[j].first })
	out := groups[:0]
	for _, g := range groups {
		if len(out) > 0 && g.first <= out[len(out)-1].last+1 {
			last := &out[len(out)-1]
			last.last = max(last.last, g.last)
			last.members = append(last.members, g.members...)
		} else {
			out = append(out, g)
		}
	}
	return out, nil
}

// Plan each intersecting/adjacent logical block once. Individual cursor offsets
// and directions are then retained inside its result, with full before history.
func (v *SourceView) multipleLineCommand(kind string, down bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	cursors := v.cursors()
	groups, err := v.selectedLineGroups(cursors)
	if err != nil {
		return err
	}
	plans := make([]editor.Plan, len(cursors))
	for _, g := range groups {
		a, _ := v.document.LineRange(g.first)
		b, _ := v.document.LineRange(g.last)
		sel := TextSelection{Anchor: a.Start, Caret: b.End}
		var p editor.Plan
		switch kind {
		case "move":
			p, err = editor.MoveLines(v.document.model, sel, down)
		case "duplicate":
			p, err = editor.DuplicateLines(v.document.model, sel, down)
		case "delete":
			p, err = editor.DeleteLines(v.document.model, sel)
		}
		if err != nil {
			return err
		}
		resultLength := v.document.Len()
		for _, e := range p.Edits {
			resultLength += len(e.Text) - (e.Range.End - e.Range.Start)
		}
		delta := p.After.Anchor - p.Before.Anchor
		for _, i := range g.members {
			own := p
			own.Before = cursors[i].selection
			switch kind {
			case "delete":
				own.After = p.After
			default:
				own.After = TextSelection{Anchor: min(resultLength, own.Before.Anchor+delta), Caret: min(resultLength, own.Before.Caret+delta)}
			}
			plans[i] = own
		}
	}
	edits, after, err := combinePlans(plans)
	if err != nil {
		return err
	}
	return v.applyMultiple(edits, after)
}
func (v *SourceView) moveMultipleWord(forward, extend, part bool) {
	if v.Destroyed() || !gui.IsEnabled(v) {
		return
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return
	}
	out := v.cursors()
	for i, c := range out {
		s := c.selection
		at := s.Caret
		if part && !extend && s.Anchor != s.Caret {
			at = s.Range().Start
			if forward {
				at = s.Range().End
			}
		} else if part {
			at = editor.WordPartBoundary(v.document.model, at, forward, false, v.definitionAt(at).IsWord)
		} else {
			at = editor.WordBoundaryWith(v.document.model, at, forward, false, v.definitionAt(at).IsWord)
		}
		at = v.foldNavigationTarget(at, forward)
		c.selection.Caret = at
		if !extend {
			c.selection.Anchor = at
		}
		c.upstream = false
		c.preferredSet = false
		out[i] = c
	}
	v.document.model.BreakUndoGroup()
	v.setCursorStatesKeepingCompletion(normalizeCursors(out), true, nil)
}
func (v *SourceView) selectMultipleLines() {
	if v.Destroyed() || !gui.IsEnabled(v) {
		return
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return
	}
	out := v.cursors()
	for i, c := range out {
		r := c.selection.Range()
		a, _ := v.document.LineRange(v.document.model.LineAt(r.Start))
		b := v.lineSelection(r.End)
		out[i] = cursorState{selection: TextSelection{Anchor: a.Start, Caret: b.End}}
	}
	v.document.model.BreakUndoGroup()
	v.setCursorStatesKeepingCompletion(normalizeCursors(out), true, nil)
}

func (v *SourceView) multipleLineComments() error {
	if err := v.editable(); err != nil {
		return err
	}
	cursors := v.cursors()
	groups, err := v.selectedLineGroups(cursors)
	if err != nil {
		return err
	}
	var edits []TextEdit
	all := true
	for _, g := range groups {
		for row := g.first; row <= g.last; row++ {
			line, _ := v.document.LineRange(row)
			marker := v.definitionAt(line.Start).Editing().Comments.Line
			if marker == "" {
				continue
			}
			indent := editor.LeadingIndent(v.document.model, line.End)
			at := line.Start + len(indent)
			if at == line.End {
				continue
			}
			end := min(line.End, at+len(marker)+1)
			for !v.document.IsRuneBoundary(end) {
				end--
			}
			prefix, _ := v.document.Slice(TextRange{Start: at, End: end})
			edit := TextEdit{Range: TextRange{Start: at, End: at}, Text: marker + " "}
			if strings.HasPrefix(prefix, marker) {
				edit.Range.End += len(marker)
				if strings.HasPrefix(prefix[len(marker):], " ") {
					edit.Range.End++
				}
			} else {
				all = false
			}
			if len(edits) == maxPlannedEdits {
				return ErrOperationLimit
			}
			edits = append(edits, edit)
		}
	}
	for i := range edits {
		if all {
			edits[i].Text = ""
		} else {
			edits[i].Range.End = edits[i].Range.Start
		}
	}
	return v.applyMultiple(edits, mappedCursors(cursors, edits))
}
func mappedCursors(cursors []cursorState, edits []TextEdit) []cursorState {
	points := make([]int, 0, len(cursors)*2)
	for _, c := range cursors {
		points = append(points, c.selection.Anchor, c.selection.Caret)
	}
	changes := slices.Clone(edits)
	slices.Reverse(changes)
	points = textcore.MapOffsets(points, changes)
	out := slices.Clone(cursors)
	for i := range out {
		out[i].selection = TextSelection{Anchor: points[2*i], Caret: points[2*i+1]}
		out[i].upstream = false
		out[i].preferredSet = false
	}
	return out
}
