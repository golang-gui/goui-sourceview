package sourceview

import (
	"fmt"
	"sort"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/gui"
)

// ApplyEditsWithSelections commits one original-coordinate transaction and its
// complete post-edit selection set. Invalid results leave text/history unchanged.
func (v *SourceView) ApplyEditsWithSelections(document *Document, revision uint64, edits []TextEdit, after []TextSelection) error {
	if document != v.document || revision != document.Revision() {
		return ErrOperationChanged
	}
	if len(after) == 0 {
		return fmt.Errorf("sourceview: empty result selections")
	}
	if len(after) > maxSelections || len(edits) > maxPlannedEdits {
		return ErrOperationLimit
	}
	cursors := make([]cursorState, len(after))
	for i, s := range after {
		cursors[i].selection = s
	}
	return v.applyMultiple(edits, normalizeCursors(cursors))
}

func (v *SourceView) applyMultiple(edits []TextEdit, after []cursorState) error {
	return v.applyMultipleInput(edits, after, textcore.MergeNone)
}
func (v *SourceView) applyMultipleInput(edits []TextEdit, after []cursorState, merge textcore.BatchMerge) error {
	v.columnSelection = nil
	if err := v.editable(); err != nil {
		return err
	}
	if len(edits) > maxPlannedEdits || len(after) > maxSelections {
		return ErrOperationLimit
	}
	generated := 0
	for _, e := range edits {
		if len(e.Text) > (64<<20)-generated {
			return ErrOperationLimit
		}
		generated += len(e.Text)
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.editingValid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	commit := &selectionCommit{state: state, after: normalizeCursors(after)}
	previous := v.selectionCommit
	v.selectionCommit = commit
	defer func() {
		if v.selectionCommit == commit {
			v.selectionCommit = previous
		}
	}()
	err := v.document.model.ApplyBatch(edits, textcore.BatchOptions{Merge: merge, Source: &v.source, BeforeSelections: v.historySelections(), AfterSelections: cursorHistory(commit.after)})
	if err != nil {
		return err
	}
	if v.document != state.document || v.mountEpoch != state.mount || v.Destroyed() {
		return nil
	}
	if commit.applied {
		if v.selectionEpoch == state.selection+1 && v.document.Revision() == state.revision+1 {
			v.resetBlink()
			v.RevealSelection()
		}
	} else if v.document.Revision() == state.revision+1 && v.selectionEpoch == state.selection {
		v.setCursorStatesKeepingCompletion(commit.after, true, nil)
	} else if len(edits) == 0 && state.valid(v) {
		v.setCursorStatesKeepingCompletion(commit.after, true, nil)
	}
	return nil
}

// combinePlans reconciles independently planned original-coordinate edits.
// Each plan's After uses that plan's own result; map it back to its owned edit
// then forward through the combined transaction, retaining direction.
func combinePlans(plans []editor.Plan) ([]TextEdit, []cursorState, error) {
	type owned struct {
		edit  TextEdit
		owner int
	}
	all := make([]owned, 0, len(plans))
	total := 0
	for i, p := range plans {
		for _, e := range p.Edits {
			if e.Range.Start == e.Range.End && e.Text == "" {
				continue
			}
			e.Text = textcore.Normalize(e.Text)
			all = append(all, owned{e, i})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i].edit.Range, all[j].edit.Range
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.End < b.End
	})
	edits := make([]TextEdit, 0, len(all))
	for _, o := range all {
		e := o.edit
		if len(edits) > 0 {
			last := &edits[len(edits)-1]
			if e.Range == last.Range && e.Text == last.Text {
				continue
			}
			if e.Range.Start < last.Range.End || e.Range.Start == last.Range.Start {
				if e.Text == "" && last.Text == "" {
					last.Range.End = max(last.Range.End, e.Range.End)
					continue
				}
				return nil, nil, fmt.Errorf("sourceview: conflicting selection edits")
			}
		}
		edits = append(edits, e)
	}
	for _, e := range edits {
		if len(e.Text) > (64<<20)-total {
			return nil, nil, ErrOperationLimit
		}
		total += len(e.Text)
	}
	if len(edits) > maxPlannedEdits {
		return nil, nil, ErrOperationLimit
	}
	prefix := make([]int, len(edits)+1)
	for i, e := range edits {
		prefix[i+1] = prefix[i] + len(e.Text) - (e.Range.End - e.Range.Start)
	}
	mapOriginal := func(pos int) int {
		index := sort.Search(len(edits), func(i int) bool { return edits[i].Range.End >= pos })
		if index == len(edits) {
			return pos + prefix[index]
		}
		e := edits[index]
		if pos < e.Range.Start {
			return pos + prefix[index]
		}
		return e.Range.Start + prefix[index] + len(e.Text)
	}
	after := make([]cursorState, len(plans))
	for i, p := range plans {
		mapEnd := func(pos int) int {
			delta := 0
			for _, e := range p.Edits {
				begin := e.Range.Start + delta
				size := len(textcore.Normalize(e.Text))
				end := begin + size
				if pos < begin {
					break
				}
				if pos <= end {
					if size == 0 {
						return mapOriginal(e.Range.Start)
					}
					index := sort.Search(len(edits), func(i int) bool { return edits[i].Range.Start >= e.Range.Start })
					if index < len(edits) && edits[index].Range.Start == e.Range.Start {
						return e.Range.Start + prefix[index] + pos - begin
					}
					return mapOriginal(e.Range.Start)
				}
				delta += size - (e.Range.End - e.Range.Start)
			}
			return mapOriginal(pos - delta)
		}
		after[i].selection = TextSelection{Anchor: mapEnd(p.After.Anchor), Caret: mapEnd(p.After.Caret)}
	}

	return edits, after, nil
}
func (v *SourceView) multiplePlans(plan func(cursorState) (editor.Plan, error)) error {
	if err := v.editable(); err != nil {
		return err
	}
	state := v.operationState()
	plans := make([]editor.Plan, 0, len(v.additional)+1)
	for _, s := range v.cursors() {
		p, err := plan(s)
		if err != nil {
			return err
		}
		if !state.editingValid(v) || v.Destroyed() {
			return ErrOperationChanged
		}
		plans = append(plans, p)
	}
	edits, after, err := combinePlans(plans)
	if err != nil {
		return err
	}
	return v.applyMultiple(edits, after)
}
func (v *SourceView) replaceMultiple(texts []string) error {
	byStart := make(map[int]string, len(v.additional)+1)
	for i, s := range v.orderedSelections() {
		text := texts[0]
		if len(texts) > 1 {
			text = texts[i]
		}
		byStart[s.Range().Start] = text
	}
	return v.multiplePlans(func(c cursorState) (editor.Plan, error) {
		return replacementPlan(c, byStart[c.selection.Range().Start]), nil
	})
}

func (v *SourceView) multiplePaste(text string) error {
	text = textcore.Normalize(text)
	parts := strings.Split(text, "\n")
	if len(parts) != len(v.additional)+1 {
		parts = []string{text}
	}
	return v.replaceMultiple(parts)
}
func (v *SourceView) multipleDelete(forward bool) error { return v.multipleDeleteKind(forward, 0) }

func (v *SourceView) indentUnit() string {
	if v.indent.InsertSpaces {
		return strings.Repeat(" ", v.indent.IndentWidth)
	}
	return "\t"
}
func (v *SourceView) copyMultiple() error {
	selected := false
	for _, s := range v.Selections() {
		if s.Anchor != s.Caret {
			selected = true
		}
	}
	if !selected {
		return nil
	}
	if !v.mounted || gui.App == nil {
		return ErrNotMounted
	}
	state := v.operationState()
	parts := make([]string, 0, len(v.additional)+1)
	total := len(v.additional)
	for _, s := range v.orderedSelections() {
		r := s.Range()
		if r.End-r.Start > (64<<20)-total {
			return ErrOperationLimit
		}
		total += r.End - r.Start
		text, err := v.document.Slice(r)
		if err != nil {
			return err
		}
		parts = append(parts, text)
	}
	clipboard := gui.App.Clipboard()
	if clipboard == nil {
		return ErrClipboardUnavailable
	}
	if !state.editingValid(v) {
		return ErrOperationChanged
	}
	return clipboard.SetText(strings.Join(parts, "\n"))
}
func (v *SourceView) cutMultiple() error {
	if err := v.editable(); err != nil {
		return err
	}
	state := v.operationState()
	selected := false
	for _, s := range v.Selections() {
		if s.Anchor != s.Caret {
			selected = true
		}
	}
	if !selected {
		return nil
	}
	if err := v.copyMultiple(); err != nil {
		return err
	}
	if !state.editingValid(v) || !v.mounted || v.Destroyed() {
		return ErrOperationChanged
	}
	return v.replaceMultiple([]string{""})
}

func multipleCommandReady(c Command) bool {
	return c != CommandComplete
}
