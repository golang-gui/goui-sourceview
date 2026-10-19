package sourceview

import (
	"context"
	"errors"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/gui"
)

type foldingJob struct {
	revision, epoch uint64
	definition      *language.Language
	cancel          context.CancelFunc
}
type foldingQueue struct {
	owner  *Document
	busy   bool
	active *foldingJob
}

func (d *Document) requestFolding(immediate bool) {
	if d.folding != nil && d.folding.active != nil {
		d.folding.active.cancel()
		d.folding.active = nil
	}
	d.foldGeneration++
	d.foldReady = false
	d.foldDue = time.Now().Add(150 * time.Millisecond)
	if immediate {
		d.foldDue = time.Now()
	}
	d.requestWork()
}
func (d *Document) foldingStep() {
	if d.foldReady || d.definition.Folding() == nil || gui.App == nil {
		return
	}
	wanted := false
	for v := range d.views {
		wanted = wanted || v.foldingEnabled
	}
	if !wanted {
		return
	}
	if d.folding == nil {
		d.folding = &foldingQueue{owner: d}
	}
	q := d.folding
	if q.busy || time.Now().Before(d.foldDue) {
		d.requestWork()
		return
	}
	d.foldReady = true // active job owns this revision; later edits request again
	ctx, cancel := context.WithCancel(context.Background())
	job := &foldingJob{revision: d.model.Revision(), epoch: d.foldGeneration, definition: d.definition, cancel: cancel}
	text, fn, post := d.model.Text(), d.definition.Folding(), gui.App.Post
	q.active = job
	q.busy = true
	go func() { ranges, err := fn(ctx, text); post(func() { q.finish(ranges, err) }) }()
}
func (q *foldingQueue) finish(ranges []language.FoldRange, err error) {
	q.busy = false
	job := q.active
	q.active = nil
	d := q.owner
	if job != nil {
		job.cancel()
		if d != nil && len(d.views) > 0 && d.model.Revision() == job.revision && d.foldGeneration == job.epoch && d.definition == job.definition {
			if err == nil {
				ranges, err = normalizeFolds(ranges, d.model.LineCount())
			}
			if !errors.Is(err, context.Canceled) {
				if err != nil {
					ranges = nil
				}
				d.foldRanges = ranges
				d.foldRevision = job.revision
				d.foldReady = true
				views := make([]*SourceView, 0, len(d.views))
				for v := range d.views {
					views = append(views, v)
				}
				for _, v := range views {
					if !v.mounted || v.document != d || d.model.Revision() != job.revision || d.definition != job.definition {
						continue
					}
					if err != nil {
						v.clearFolds()
						v.report(err)
					} else {
						v.acceptFolds(ranges)
					}
				}
			}
		}
	}
	if d != nil {
		d.requestWork()
	}
}
func (d *Document) stopFolding() {
	if d.folding != nil {
		if d.folding.active != nil {
			d.folding.active.cancel()
			d.folding.active = nil
		}
		d.folding.owner = nil
		d.folding = nil
	}
	d.foldRanges = nil
	d.foldReady = false
	d.foldGeneration++
}
