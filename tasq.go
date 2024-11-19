package tasq

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/symonk/tasq/internal/contract"
)

type Tasq struct {

	// queue specifics
	waitingQueueSize int
	waitingCh        chan func()
	// Use slice for now, but reconsider data structures for future.
	holdingPen []func()
	penMu      sync.RWMutex

	processingQueueSize int
	processingCh        chan func()

	// worker specifics
	maxWorkers  int
	currWorkers int
	stopped     bool
	stoppedMu   sync.Mutex

	// shutdown specifics
	terminated         chan struct{}
	workerIdleDuration time.Duration
}

// Ensure Tasq implements Pooler
var _ contract.Pooler = (*Tasq)(nil)

// New instantiates a new Tasq instance and applies the
// appropriate functional options to it.
// Returns the new instances of Tasq
func New(opts ...Option) *Tasq {
	t := &Tasq{}
	t.workerIdleDuration = 3 * time.Second
	for _, opt := range opts {
		opt(t)
	}
	// TODO: Don't make these buffered, use another data structure
	// to build a backpressure mechanism.
	t.waitingCh = make(chan func(), t.waitingQueueSize)
	t.processingCh = make(chan func(), t.processingQueueSize)
	go t.dispatch()
	return t
}

// dispatch is the core implementation of the worker pool
// and is responsible for handling and executing tasks.
// TODO: This needs a lot of work
func (t *Tasq) dispatch() {
	workerKiller := time.NewTimer(3 * time.Second)
	isOversized := false
	_ = isOversized

	// TODO: Channel specifics on shutdown etc.

Core:
	for {

		// If our slice of tasks is backfilling, there is pressure already on the
		// channel queues, keep buffering the tasks as talking directly to our other
		// queues will be blocked at this point. (implement a double ended Q)
		// TODO: To be accurate here do we need a mutex lock around this, perhaps a
		// read only lock check? tho the performance implications are not subtle possibly.

		select {
		case t1 := <-t.waitingCh:
			t.processingCh <- t1
		case t2 := <-t.processingCh:
			fmt.Println("processing task")
			_ = t2
		case <-workerKiller.C:
			workerKiller.Reset(3 * time.Second)
			// TODO: Scale down our workers by -1, something is idle!
		}
		break Core
	}
}

// HasBackPressure checks if the queue for holding tasks to be
// processed in future is populated.  If it is, there is no point
// going directly to the channels, this check is utilised to know
// if we should push onto the holden pen deque in future.
func (t *Tasq) HasBackPressure() bool {
	t.penMu.RLock()
	defer t.penMu.RUnlock()
	return len(t.holdingPen) > 0
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

// Drain prevents new tasks being enqueued and performs
// a graceful shutdown of the worker pool after all tasks
// in flight have been processed.
func (t *Tasq) Drain() {
}

// Throttle causes blocking across the workers until
// the given context is cancelled/timed out.  This allows
// temporarily throttling the queue tasks.  Right now the
// tasks the workers have accepted prior to this being called
// will be invoked, so this is not an immediate halt, N number
// of tasks will remain attempted until the halt propagates.
// TODO: Consider a way of doing this with immediate halting
func (t *Tasq) Throttle(ctx context.Context) {

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

// ScaleUp adds a new worker into the pool to cope with
// higher demand.  If workers have set idle for a period
// of time, A timer/tick check ensues and scales down where
// appropriate.  Thi is configurable via the WithWorkerCheck(dur)
// option.
func (t *Tasq) ScaleUp() {

}

// ScaleDown removes an idle worker from the pool, down to
// zero (0) workers when the pool is completely unutilised.
// the cost for worker creation is trivial in the larger
// scheme of things.
func (t *Tasq) ScaleDown() {

}

// ----- Worker specifics ----- //

type Worker struct {
}

func (w *Worker) Run() {

}

func (w *Worker) Terminate() {

}
