package sourceview

import (
	"context"
	"errors"

	"github.com/golang-gui/goui/gui"
)

// All queue fields belong to the GUI thread. A worker sees only copied request
// data, its context and the provider function. Even an uncooperative provider
// cannot create a growing set of concurrent workers for one view.
type completionJob struct {
	state    operationState
	epoch    uint64
	request  CompletionContext
	complete func(context.Context, CompletionContext) ([]Completion, error)
	context  context.Context
	cancel   context.CancelFunc
}
type completionQueue struct {
	owner        *SourceView
	busy         bool
	active, next *completionJob
}

func (q *completionQueue) pending() bool { return q.active != nil || q.next != nil }
func (q *completionQueue) cancel() {
	if q.active != nil {
		q.active.cancel()
		q.active = nil
	}
	if q.next != nil {
		q.next.cancel()
		q.next = nil
	}
}
func (v *SourceView) queueCompletion(state operationState, epoch uint64, c CompletionContext, fn func(context.Context, CompletionContext) ([]Completion, error)) {
	if v.completionQueue == nil {
		v.completionQueue = &completionQueue{}
	}
	q := v.completionQueue
	q.owner = v
	ctx, cancel := context.WithCancel(context.Background())
	job := &completionJob{state: state, epoch: epoch, request: c, complete: fn, context: ctx, cancel: cancel}
	if q.busy {
		q.next = job
		return
	}
	q.start(job, gui.App.Post)
}
func (q *completionQueue) start(job *completionJob, post func(func())) {
	q.busy = true
	q.active = job
	// Do not capture job: cancellation releases its Document/operationState while
	// the provider is still running. The posted callback reads queue state on GUI.
	request, ctx, fn := job.request, job.context, job.complete
	go func() {
		items, err := fn(ctx, request)
		items = sanitizeCompletions(items)
		post(func() { q.finish(items, err, post) })
	}()
}
func (q *completionQueue) finish(items []Completion, err error, post func(func())) {
	q.busy = false
	job := q.active
	q.active = nil
	if job != nil {
		job.cancel()
		if v := q.owner; v != nil && job.state.valid(v) && v.completionEpoch == job.epoch && v.mounted && v.editable() == nil && v.Focused() {
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					v.report(err)
				}
			} else {
				v.report(v.presentCompletions(job.state, job.epoch, job.request, items))
			}
		}
	}
	if q.busy {
		return
	}
	next := q.next
	q.next = nil
	if next != nil {
		if q.owner != nil && next.state.valid(q.owner) && q.owner.completionEpoch == next.epoch {
			q.start(next, post)
		} else {
			next.cancel()
		}
	}
}
