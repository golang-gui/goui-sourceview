package analysis

import (
	"context"
	"sync"
	"time"

	source "github.com/golang-gui/goui-sourceview"
)

// Service owns one analysis worker and at most one queued full-file snapshot.
// Submit and Close are GUI-side operations. Provider and the worker only read
// copied data. Post must deliver to the application's existing event loop.
type Service struct {
	mu         sync.Mutex
	latest     Snapshot
	generation uint64
	cancel     context.CancelFunc
	closed     bool
	jobs       chan job
	stop       chan struct{}
	done       chan struct{}
	post       func(func())
	publish    func(Result, error)
}
type job struct {
	snapshot   Snapshot
	generation uint64
	ctx        context.Context
	delay      time.Duration
}

func NewService(post func(func()), publish func(Result, error)) *Service {
	s := &Service{jobs: make(chan job, 1), stop: make(chan struct{}), done: make(chan struct{}), post: post, publish: publish}
	go s.run()
	return s
}
func (s *Service) Submit(snapshot Snapshot, delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.latest = snapshot
	s.generation++
	select {
	case <-s.jobs:
	default:
	}
	s.jobs <- job{snapshot, s.generation, ctx, delay}
}
func (s *Service) run() {
	defer close(s.done)
	for {
		select {
		case <-s.stop:
			return
		case j := <-s.jobs:
			timer := time.NewTimer(j.delay)
			select {
			case <-j.ctx.Done():
				timer.Stop()
				continue
			case <-timer.C:
			}
			result, err := Analyze(j.ctx, j.snapshot)
			if j.ctx.Err() != nil {
				continue
			}
			s.post(func() {
				s.mu.Lock()
				valid := !s.closed && s.generation == j.generation
				s.mu.Unlock()
				if valid {
					s.publish(result, err)
				}
			})
		}
	}
}
func (s *Service) Provider() *source.CompletionProvider {
	return &source.CompletionProvider{TriggerCharacters: ".", Complete: func(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
		s.mu.Lock()
		snapshot, closed := s.latest, s.closed
		s.mu.Unlock()
		if closed {
			return nil, nil
		}
		return Complete(ctx, snapshot, c)
	}}
}
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	close(s.stop)
	s.latest = Snapshot{}
}

// Wait is for shutdown/tests outside the GUI callback; Close itself never waits
// for a non-cancellable go/types check to finish.
func (s *Service) Wait() { <-s.done }
