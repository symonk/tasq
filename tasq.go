package tasq

import (
	"sync"
)

// Worker is responsible for processing tasks
type Worker struct {
}

type Tasq struct {

	// queue specifics
	waitingQueueSize int
	waitingCh        chan func()

	processingQueueSize int
	processingCh        chan func()

	// worker specifics
	maxWorkers  int
	currWorkers int
	stopped     bool
	stoppedMu   sync.Mutex
}

// New instantiates a new Tasq instance and applies the
// appropriate functional options to it.
// Returns the new instances of Tasq
func New(opts ...Option) *Tasq {
	t := &Tasq{}
	for _, opt := range opts {
		opt(t)
	}
	t.waitingCh = make(chan func(), t.waitingQueueSize)
	t.processingCh = make(chan func(), t.processingQueueSize)
	go t.dispatch()
	return t
}

// dispatch is the core implementation of the worker pool
// and is responsible for handling and executing tasks.
// TODO: This needs a lot of work
func (t *Tasq) dispatch() {
	for {
		select {
		case t := <-t.waitingCh:
			_ = t
		case t2 := <-t.processingCh:
			_ = t2
		}
	}
}

// MaxWorkers returns the maximum number of workers.
// workers can be scaled depending on demand, so while
// use ActiveWorkers() to get the current actual number
// of active workers
func (t *Tasq) MaxWorkers() int {
	return t.maxWorkers
}

// ActiveWorkers returns the number of current workers
// in the pool.
func (t *Tasq) ActiveWorkers() int {
	return t.currWorkers
}

// Stop drains all tasks in the queue and terminates the pool.
// When the pool is in a stopped state no work tasks will be
// enqueued.
func (t *Tasq) Stop() {
	t.stoppedMu.Lock()
	defer t.stoppedMu.Unlock()
	t.stopped = true
}

// Enqueue is responsible for preparing a user defined task to
// be consumed by the pool (at some point in future).  This
// is not blocking, if you wish to wait until the task has been
// processed, use EnqueueWait() instead.
func (t *Tasq) Enqueue(task func()) string {
	return ""
}

func (t *Tasq) EnqueueWait(task func()) string {
	return ""
}
