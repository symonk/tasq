package tasq

import (
	"context"
	"sync"
	"time"

	"github.com/symonk/tasq/internal/contract"
)

// Task is a simple function without args or a return value.
// Task types are submitted to the Tasq for processing.
// At present, users should handle return types and throttling
// on the client side.
type Task func()

const (
	// workerIdleTimeout is the default duration that the pool checks for
	// dormant/idle workers to cause them to be shutdown.
	workerIdleTimeout = time.Second * 3
)

// Tasq is the worker pool implementation.  It has
// three main queues.
// The task Q is a channel that accepts all tasks and stores them in the interim holding pen
// Tasks are then taken from the holding pen, into the active queue
type Tasq struct {

	// queue specifics
	incomingQueue chan func()
	// Uslice for now, but reconsider data structures for future.
	interimQueue []func()
	penMu        sync.RWMutex

	processingQueue chan func()

	// worker specifics
	maxWorkers    int
	currWorkers   int
	stopped       bool
	stoppingMutex sync.Mutex

	// shutdown specifics
	done               chan struct{}
	workerIdleDuration time.Duration

	terminate      sync.Once
	isShuttingDown bool
}

// Ensure Tasq implements Pooler
var _ contract.Pooler = (*Tasq)(nil)

// New instantiates a new Tasq instance and applies the
// appropriate functional options to it.
// Returns the new instances of Tasq
func New(opts ...Option) *Tasq {
	t := &Tasq{}
	t.workerIdleDuration = workerIdleTimeout
	for _, opt := range opts {
		opt(t)
	}
	// TODO: Don't make these buffered, use another data structure
	// to build a backpressure mechanism.
	t.incomingQueue = make(chan func())
	t.processingQueue = make(chan func())
	go t.dispatch()
	return t
}

// dispatch is the core implementation of the worker pool
// and is responsible for handling and executing tasks.
func (t *Tasq) dispatch() {
	defer close(t.done)
	workerKiller := time.NewTimer(3 * time.Second)
	processedTasks := false
	var wg sync.WaitGroup

core:
	for {
		// If our slice of tasks is backfilling, there is pressure already on the
		// channel queues, keep buffering the tasks as talking directly to our other
		// queues will be blocked at this point. (implement a double ended Q)
		if t.IsOverflowingToHoldingQueue() {
			t.processHoldingQueue()
			break core
		}

		// There is no tasks currently in the queued queue.  We can directly
		// insert the tasks to the waiting queue or process tasks from the
		// waiting queue into the processing queue.
		select {
		case inboundTask, ok := <-t.incomingQueue:
			// Attempt to move a task from a waiting state, into a processing one.
			// If the channel has been closed, cause an exit.
			if !ok {
				break core
			}
			select {
			case t.incomingQueue <- inboundTask:
				// Perform a worker check here, we may need to scale the workers
				// towards maximum configured capacity.
				if t.currWorkers < t.maxWorkers {
					t.scaleUp(inboundTask, t.processingQueue, &wg)
					// only the main goroutine running the tasq instance will be modifying this
					// internal state, no need to synchronise.
				}
				// We have been processing work, there is no need to be scaling down the workers.
				processedTasks = false
			}
		case <-workerKiller.C:
			// There have been no processed tasks for the entire duration of the idle checking duration
			// scale down one worker, down to zero.
			if processedTasks && t.currWorkers > 0 {
				t.scaleDown(&wg)
			}
			workerKiller.Reset(3 * time.Second)
			processedTasks = true
		}
	}
	wg.Wait()
}

// IsOverflowingToHoldingQueue checks if the queue for holding tasks to be
// processed in future is populated.  If it is, there is no point
// going directly to the channels, this check is utilised to know
// if we should push onto the holden pen deque in future.
// TODO: RW locking is unnecessary, only tasq routine changes this state.
func (t *Tasq) IsOverflowingToHoldingQueue() bool {
	t.penMu.RLock()
	defer t.penMu.RUnlock()
	return len(t.interimQueue) > 0
}

// processHoldingQueue is responsible for getting the backfilled tasks
// out of the queue buffer and into the core waiting/processing internal
// machinery.
// nil checks etc?
func (t *Tasq) processHoldingQueue() {
	ele := t.interimQueue[len(t.interimQueue)-1]
	t.incomingQueue <- ele
}

func (t *Tasq) processWaitingTask(task Task) {
	t.processingQueue <- task
}

// MaximumConfiguredWorkers returns the maximum number of workers.
// workers can be scaled depending on demand, so while
// use ActiveWorkers() to get the current actual number
// of active workers
func (t *Tasq) MaximumConfiguredWorkers() int {
	return t.maxWorkers
}

// CurrentWorkerCount returns the number of current workers
// in the pool.
func (t *Tasq) CurrentWorkerCount() int {
	return t.currWorkers
}

// Stop drains all tasks in the queue and terminates the pool.
// When the pool is in a stopped state no work tasks will be
// enqueued.
func (t *Tasq) Stop() {
	t.stoppingMutex.Lock()
	defer t.stoppingMutex.Unlock()
	t.terminate.Do(func() {
		t.isShuttingDown = true

		close(t.incomingQueue)
		close(t.processingQueue)
	})
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
func (t *Tasq) Enqueue(task func()) {
	if task != nil {
		t.incomingQueue <- task
	}
}

// EnqueueWait pushes a new task onto the task queue and wraps the
// task with a done channel that waits until the task has been processed
// through all internal queues and been executed.  No return value is handed
// back to the caller here, the user should wrap their task in a closure that
// is responsible for writing results onto a channel or some other synchronisation
// mechanism.
func (t *Tasq) EnqueueWait(task func()) {
	if task != nil {
		done := make(chan struct{})
		t.incomingQueue <- func() {
			defer close(done)
			t.Enqueue(task)
		}
		<-done
	}
}

// scaleUp spawns a new worker in a goroutine ready when handle tasks
// from the internal processing queue.
func (t *Tasq) scaleUp(task func(), processingQ <-chan func(), wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker(task, processingQ, wg)
	}()
	t.currWorkers++
}

// scaleDown removes an idle worker from the pool, down to
// zero (0) workers when the pool is completely unutilised.
// the cost for worker creation is trivial in the larger
// scheme of things.
func (t *Tasq) scaleDown(wg *sync.WaitGroup) {
	t.processingQueue <- nil
	t.currWorkers--
}

// worker is responsible for processing tasks on the processing
// channel and exiting gracefully when a shutdown has been triggered.
// sending a nil task to a worker causes the worker to exit.
// a nil task is sent to a worker in order to get them to terminate/shutdown.
// user defined enqueuing does not allow nil tasks, this is special behaviour
// internally.
func worker(task func(), activeQ <-chan func(), wg *sync.WaitGroup) {
	defer wg.Done()
	for task != nil {
		task()
		task = <-activeQ
	}
}
