package sourceview

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

// CaseMode selects locale-independent Unicode case conversion.
type CaseMode uint8

const (
	CaseUpper CaseMode = iota
	CaseLower
)

// Bound the final synchronous Model.ApplyBatch transaction. Larger requests
// fail atomically instead of splitting history or blocking on an unbounded batch.
const maxPlannedEdits = 4096

type editPlanner func(context.Context, string) (editor.Plan, error)
type editingJob struct {
	state  operationState
	indent IndentOptions
	plan   editPlanner
	ctx    context.Context
	cancel context.CancelFunc
	chunks []string
	copied int
	end    int
}
type editingQueue struct {
	owner  *SourceView
	active *editingJob
	busy   bool
}

// startEdit snapshots the model in event-loop slices and calculates one atomic
// undo plan off GUI. Only one worker and the latest request are retained per view.
func (v *SourceView) startEdit(plan editPlanner) error {
	return v.startEditRange(textedit.Range{End: v.document.model.Len()}, plan)
}
func (v *SourceView) startEditRange(span textedit.Range, plan editPlanner) error {
	if err := v.editable(); err != nil {
		return err
	}
	if !v.mounted || gui.App == nil {
		return ErrNotMounted
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return ErrOperationChanged
	}
	v.cancelEdit()
	v.cancelPaste()
	if v.editQueue == nil {
		v.editQueue = &editingQueue{owner: v}
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.editQueue.active = &editingJob{state: v.operationState(), indent: v.indent, plan: plan, ctx: ctx, cancel: cancel, copied: span.Start, end: span.End}
	v.document.requestWork()
	return nil
}
func (v *SourceView) cancelEdit() {
	if q := v.editQueue; q != nil && q.active != nil {
		q.active.cancel()
		q.active.state = operationState{}
		q.active = nil
	}
}
func (v *SourceView) editPending() bool {
	return v.editQueue != nil && v.editQueue.active != nil && !v.editQueue.busy
}
func (v *SourceView) editStep() {
	q := v.editQueue
	if q == nil || q.active == nil {
		return
	}
	job := q.active
	if !job.state.valid(v) || job.indent != v.indent || v.editable() != nil {
		v.cancelEdit()
		return
	}
	if q.busy {
		return
	}
	m := v.document.model
	end := min(job.end, job.copied+64*1024)
	for !m.IsRuneBoundary(end) {
		end--
	}
	part, _ := m.Slice(textedit.Range{Start: job.copied, End: end})
	job.chunks = append(job.chunks, part)
	job.copied = end
	if end < job.end {
		v.document.requestWork()
		return
	}
	q.busy = true
	post := gui.App.Post
	go func() {
		text := strings.Join(job.chunks, "")
		job.chunks = nil
		p, err := job.plan(job.ctx, text)
		if err == nil && len(p.Edits) > maxPlannedEdits {
			err = ErrOperationLimit
		}
		if err == nil {
			err = job.ctx.Err()
		}
		post(func() {
			q.busy = false
			v := q.owner
			if v == nil {
				return
			}
			if q.active == job {
				q.active = nil
				job.cancel()
				if job.state.valid(v) && job.indent == v.indent && v.editable() == nil {
					if err == nil {
						err = v.applyPlan(p)
					}
					if !errors.Is(err, context.Canceled) {
						v.report(err)
					}
				}
			}
			if v.editPending() {
				v.document.requestWork()
			}
		})
	}()
}

// ReindentSelection requests language indentation for selected logical lines,
// or the current line. Nil means accepted (or no provider); delayed errors use
// ConnectError. Any change of revision, selection, options or mount cancels it.
func (v *SourceView) ReindentSelection() error {
	if err := v.editable(); err != nil {
		return err
	}
	fn := v.definitionAt(v.Selection().Caret).Editing().Reindent
	if fn == nil {
		return nil
	}
	s, o := v.Selection(), v.indent
	first, last, err := editor.SelectedLines(v.document.model, s)
	if err != nil {
		return err
	}
	return v.startEdit(func(ctx context.Context, text string) (editor.Plan, error) {
		results, err := fn(ctx, language.ReindentContext{Text: text, StartLine: first, EndLine: last, TabWidth: o.TabWidth, IndentWidth: o.IndentWidth, InsertSpaces: o.InsertSpaces})
		if err != nil {
			return editor.Plan{}, err
		}
		return editor.ReindentPlan(ctx, text, s, first, last, results)
	})
}

// JoinLines joins selected logical lines, or the current and following line.
// It is an explicit text operation; it does not preserve language semantics.
func (v *SourceView) JoinLines() error {
	s := v.Selection()
	return v.startEdit(func(ctx context.Context, text string) (editor.Plan, error) {
		return editor.CleanupPlan(ctx, text, s, "join")
	})
}

// TrimTrailingWhitespace removes ASCII space/tab suffixes from selected logical
// lines, or the whole document for an empty selection. LF and NBSP are retained.
func (v *SourceView) TrimTrailingWhitespace() error {
	s := v.Selection()
	return v.startEdit(func(ctx context.Context, text string) (editor.Plan, error) {
		return editor.CleanupPlan(ctx, text, s, "trim")
	})
}

// TransformCase converts the selection, or the language word at the caret.
// Unicode conversion may change byte length; the result is one undo item.
func (v *SourceView) TransformCase(mode CaseMode) error {
	if mode != CaseUpper && mode != CaseLower {
		return fmt.Errorf("sourceview: invalid case mode")
	}

	s := v.Selection()
	word := v.definitionAt(s.Caret).IsWord
	if s.Anchor != s.Caret {
		r := s.Range()
		return v.startEditRange(r, func(ctx context.Context, text string) (editor.Plan, error) {
			return editor.CasePlan(ctx, text, s, r, mode == CaseUpper)
		})
	}
	return v.startEdit(func(ctx context.Context, text string) (editor.Plan, error) {
		r := editor.WordRange(textedit.NewModel(text), s.Caret, word)
		return editor.CasePlan(ctx, text[r.Start:r.End], s, r, mode == CaseUpper)
	})
}

// PasteReindented explicitly shifts a multiline paste's common indentation to
// the target line. Ordinary Paste is unchanged. Unknown/literal contexts and
// languages without a reindent provider use literal paste.
func (v *SourceView) PasteReindented() error { return v.paste(true) }
