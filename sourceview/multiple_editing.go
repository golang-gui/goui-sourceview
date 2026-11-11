package sourceview

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui-sourceview/language"
)

func (v *SourceView) startVectorEdit(plan vectorEditPlanner) error {
	return v.startVectorEditRange(TextRange{End: v.document.Len()}, plan)
}
func (v *SourceView) startVectorEditRange(span TextRange, plan vectorEditPlanner) error {
	if err := v.startEditRange(span, nil); err != nil {
		return err
	}
	v.editQueue.active.vector = plan
	return nil
}
func (v *SourceView) startMultipleCleanup(kind string) error {
	cursors := v.cursors()
	return v.startVectorEdit(func(ctx context.Context, text string) ([]TextEdit, []cursorState, error) {
		return multipleCleanupPlan(ctx, text, cursors, kind)
	})
}
func multipleCleanupPlan(ctx context.Context, text string, cursors []cursorState, kind string) ([]TextEdit, []cursorState, error) {
	model := textcore.NewModel(text)
	type rows struct{ first, last int }
	spans := make([]rows, 0, len(cursors))
	for _, c := range cursors {
		first, last, err := editor.SelectedLines(model, c.selection)
		if err != nil {
			return nil, nil, err
		}
		if c.selection.Anchor == c.selection.Caret {
			if kind == "trim" {
				spans = []rows{{0, model.LineCount() - 1}}
				break
			} else {
				last = min(first+1, model.LineCount()-1)
			}
		}
		spans = append(spans, rows{first, last})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].first < spans[j].first })
	merged := spans[:0]
	for _, r := range spans {
		if len(merged) > 0 && r.first <= merged[len(merged)-1].last {
			merged[len(merged)-1].last = max(merged[len(merged)-1].last, r.last)
		} else {
			merged = append(merged, r)
		}
	}
	var edits []TextEdit
	for _, r := range merged {
		part, err := editor.CleanupEdits(ctx, text, model, r.first, r.last, kind)
		if err != nil {
			return nil, nil, err
		}
		if len(part) > maxPlannedEdits-len(edits) {
			return nil, nil, ErrOperationLimit
		}
		edits = append(edits, part...)
	}
	return edits, mappedCursors(cursors, edits), nil
}
func (v *SourceView) startMultipleCase(mode CaseMode) error {
	if mode != CaseUpper && mode != CaseLower {
		return fmt.Errorf("sourceview: invalid case mode")
	}
	cursors := v.cursors()
	definitions := make([]*language.Language, len(cursors))
	for i, c := range cursors {
		definitions[i] = v.definitionAt(c.selection.Caret)
	}
	return v.startVectorEdit(func(ctx context.Context, text string) ([]TextEdit, []cursorState, error) {
		model := textcore.NewModel(text)
		plans := make([]editor.Plan, len(cursors))
		for i, c := range cursors {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			r := c.selection.Range()
			if r.Start == r.End {
				r = editor.WordRange(model, c.selection.Caret, definitions[i].IsWord)
			}
			p, err := editor.CasePlan(ctx, text[r.Start:r.End], c.selection, r, mode == CaseUpper)
			if err != nil {
				return nil, nil, err
			}
			plans[i] = p
		}
		return combinePlans(plans)
	})
}
func (v *SourceView) startMultipleReindent() error {
	cursors := v.cursors()
	groups, err := v.selectedLineGroups(cursors)
	if err != nil {
		return err
	}
	definitions := make([]*language.Language, len(groups))
	provided := false
	for i, g := range groups {
		definitions[i] = v.definitionAt(cursors[g.members[0]].selection.Caret)
		if definitions[i].Editing().Reindent != nil {
			provided = true
		}
	}
	if !provided {
		return nil
	}
	options := v.indent
	return v.startVectorEdit(func(ctx context.Context, text string) ([]TextEdit, []cursorState, error) {
		var edits []TextEdit
		model := textcore.NewModel(text)
		for i, g := range groups {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			fn := definitions[i].Editing().Reindent
			if fn == nil {
				continue
			}
			results, err := fn(ctx, language.ReindentContext{Text: text, StartLine: g.first, EndLine: g.last, TabWidth: options.TabWidth, IndentWidth: options.IndentWidth, InsertSpaces: options.InsertSpaces})
			if err != nil {
				return nil, nil, err
			}
			part, err := editor.ReindentEdits(ctx, text, model, g.first, g.last, results)
			if err != nil {
				return nil, nil, err
			}
			if len(part) > maxPlannedEdits-len(edits) {
				return nil, nil, ErrOperationLimit
			}
			edits = append(edits, part...)
		}
		return edits, mappedCursors(cursors, edits), nil
	})
}

func (v *SourceView) pasteMultipleReindented(text string) error {
	return v.startVectorEditRange(TextRange{}, v.reindentedPastePlanner(text))
}
func (v *SourceView) reindentedPastePlanner(text string) vectorEditPlanner {
	text = textcore.Normalize(text)
	cursors := v.cursors()
	options := v.indent
	// Distribution has the same document ordering as ordinary paste, including
	// its trailing empty segment. Only multiline per-cursor values need shifting.
	parts := strings.Split(text, "\n")
	values := make(map[int]string, len(cursors))
	for i, s := range v.orderedSelections() {
		value := text
		if len(parts) == len(cursors) {
			value = parts[i]
		}
		values[s.Range().Start] = value
	}
	type target struct {
		cursor        cursorState
		value, prefix string
		definition    *language.Language
		reindent      bool
	}
	targets := make([]target, len(cursors))
	for i, c := range cursors {
		r := c.selection.Range()
		ctx, known := v.lexicalContext(r.Start)
		prefix, _ := v.lineContext(r)
		definition := v.definitionAt(c.selection.Caret)
		targets[i] = target{c, values[r.Start], prefix, definition, known && !ctx.String && !ctx.Comment && definition.Editing().Reindent != nil && strings.Trim(prefix, " \t") == ""}
	}
	return func(ctx context.Context, _ string) ([]TextEdit, []cursorState, error) {
		plans := make([]editor.Plan, len(targets))
		for i, t := range targets {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			value := t.value
			if t.reindent && strings.ContainsRune(value, '\n') {
				var err error
				value, err = editor.ReindentedPaste(ctx, value, t.prefix, editor.IndentOptions(options), t.definition)
				if err != nil {
					return nil, nil, err
				}
			}
			plans[i] = replacementPlan(t.cursor, value)
		}
		return combinePlans(plans)
	}
}
