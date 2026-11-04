package sourceview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
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
		return reindentPlan(ctx, text, s, first, last, results)
	})
}
func reindentPlan(ctx context.Context, text string, s textedit.Selection, first, last int, results []language.LineIndent) (editor.Plan, error) {
	p := editor.Plan{Before: s, After: s}
	m := textedit.NewModel(text)
	previous := first - 1
	for _, r := range results {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		if r.Line <= previous || r.Line < first || r.Line > last || len(r.Text) > 64*1024 || strings.Trim(r.Text, " \t") != "" {
			return p, fmt.Errorf("sourceview: invalid line indentation")
		}
		previous = r.Line
		line, _ := m.LineRange(r.Line)
		n := len(text[line.Start:line.End]) - len(strings.TrimLeft(text[line.Start:line.End], " \t"))
		if text[line.Start:line.Start+n] != r.Text {
			p.Edits = append(p.Edits, textedit.Edit{Range: textedit.Range{Start: line.Start, End: line.Start + n}, Text: r.Text})
		}
	}
	p.After = textedit.Selection{Anchor: editor.MapPosition(s.Anchor, p.Edits), Caret: editor.MapPosition(s.Caret, p.Edits)}
	return p, nil
}

// PasteReindented explicitly shifts a multiline paste's common indentation to
// the target line. Ordinary Paste is unchanged. Unknown/literal contexts and
// languages without a reindent provider use literal paste.
func (v *SourceView) PasteReindented() error { return v.paste(true) }

func shiftedPaste(text, prefix string, o IndentOptions) string {
	text = textedit.Normalize(text)
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return text
	}
	columns := func(s string) int {
		n := 0
		for _, r := range s {
			if r == ' ' {
				n++
			} else if r == '\t' {
				n += o.TabWidth - n%o.TabWidth
			} else {
				break
			}
		}
		return n
	}
	base := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := columns(line)
		if base < 0 || n < base {
			base = n
		}
	}
	if base < 0 {
		return text
	}
	target := columns(prefix)
	for i, line := range lines {
		n := len(line) - len(strings.TrimLeft(line, " \t"))
		if n == len(line) {
			continue
		}
		col := max(0, columns(line)-base+target)
		if i == 0 {
			col = max(0, col-target)
		}
		indent := strings.Repeat(" ", col)
		if !o.InsertSpaces {
			indent = strings.Repeat("\t", col/o.TabWidth) + strings.Repeat(" ", col%o.TabWidth)
		}
		lines[i] = indent + line[n:]
	}
	return strings.Join(lines, "\n")
}

func reindentedPaste(ctx context.Context, text, prefix string, o IndentOptions, l *language.Language) (string, error) {
	text = textedit.Normalize(text)
	t := l.Tokenizer()
	if t == nil {
		return text, nil
	}
	state := t.InitialState()
	for at := 0; at <= len(text); {
		if state.Context().String || state.Context().Comment {
			return text, nil
		}
		end := strings.IndexByte(text[at:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += at
		}
		line := text[at:end]
		for offset := 0; ; {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			limit := min(4096, len(line)-offset)
			for offset+limit < len(line) && !utf8.RuneStart(line[offset+limit]) {
				limit++
			}
			result, err := t.Scan(language.Input{Text: line[offset:], Offset: offset, Limit: limit, LineEnd: true, DocumentEnd: end == len(text)}, state)
			if err != nil {
				return text, nil
			}
			if result.Consumed < 0 || result.Consumed > len(line)-offset || result.Consumed == 0 && offset < len(line) {
				return text, nil
			}
			state = result.State
			offset += result.Consumed
			if offset == len(line) {
				break
			}
		}
		if end == len(text) {
			break
		}
		at = end + 1
	}
	if state.Context().String || state.Context().Comment {
		return text, nil
	}
	return shiftedPaste(text, prefix, o), nil
}
