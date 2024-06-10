package workerpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Worker encapsulates an individual worker
type Worker struct {
	work chan TaskFunc
}

// NewWorker instantiates an new worker object.
func NewWorker(queue chan TaskFunc) *Worker {
	return &Worker{work: queue}
}

// Stall blocks
func Stall(ctx context.Context) {

}

// Functional Options
type Option func(*WorkerPool)

// WithMaxWorkers is a functional option to control the maximum
// number of workers in the pool.
func WithMaxWorkers(workers int) Option {
	return func(w *WorkerPool) {
		workers = validateMaxWorkers(workers)
		w.maximumWorkers = workers
	}
}

// WithIdleTimeout is a functional option to control the maximum
// time a worker can be idle without performing a task before
// they are shutdown.
func WithIdleTimeout(timeout time.Duration) Option {
	return func(w *WorkerPool) {
		w.idleCheckPeriod = timeout
	}
}

// WithBufferSize is a functional option to control the
// buffer size for the worker queue.
func WithBufferSize(size int) Option {
	return func(w *WorkerPool) {
		w.waitingQueue = make(chan TaskFunc, size)
	}
}

// TaskFunc is an encapsulation of a callable piece of work
type TaskFunc func()

// Scheduler is the core interface for something which can
// take and process tasks in a distributed manner.
type Scheduler interface {
	Shutdown()
	Stopped() bool
	Stall(ctx context.Context)
	Stalled() bool
	Enqueue(task TaskFunc) error
	EnqueueWait(ctx context.Context, task TaskFunc) error
}

var ErrSubmittedNilTask = errors.New("cannot submit a nil task to the pool")

// WorkerPool is the core scheduler.  It internally manages
// a task queue and various workers up to the worker count.
// The Workerpool currently does not (yet) support a buffered
// task queue and the interim queue can grow unbounded.  Be
// careful with memory consumption.  The plan is too expose
// new options to configure buffers (if desired) in future.
type WorkerPool struct {
	// Queues (inbound, holding pen, worker queue)
	incomingQueue chan TaskFunc
	waitingQueue  chan TaskFunc
	workerQueue   chan TaskFunc

	// Configuration
	maximumWorkers   int
	idleCheckPeriod  time.Duration
	waitingQueueSize int32
	// State Tracking
	stopped bool
	stalled bool

	// Concurrency
	stallMutex       sync.Mutex
	poolMutex        sync.Mutex
	spawnedWorkersWg sync.WaitGroup

	activeWorkers int32
}

// Verify the workerpool adheres to the Scheduler interface
// at compile time.
var _ Scheduler = (*WorkerPool)(nil)

// NewWorkerPool returns a new instance (ptr) of a worker pool and
// schedules it to start accepting tasks in parallel.
func NewWorkerPool(opts ...Option) *WorkerPool {
	wp := &WorkerPool{
		// Can be configured with WithMaxWorkerCount option
		maximumWorkers: 1,
		incomingQueue:  make(chan TaskFunc),
		// Can be configured with WithWaitingQueueSize option
		waitingQueue: make(chan TaskFunc, 1024),
		// can be overwritten with the WithScalingTimeout option
		idleCheckPeriod: 5 * time.Second,
		workerQueue:     make(chan TaskFunc),
	}

	// Apply functional options after defaults have been configured
	for _, opt := range opts {
		opt(wp)
	}
	go wp.dispatch()

	return wp
}

// MaxWorkers returns the total number of maximum
// workers that can handle work in the pool.
func (w *WorkerPool) MaxWorkers() int {
	return w.maximumWorkers
}

// ActiveWorkers returns the total number of currently
// spawned workers in play.  Note: This should be
// considered an approximation, workers can currently be
// spawned during the period of asking for this.
func (w *WorkerPool) ActiveWorkers() int32 {
	return w.activeWorkers
}

// WaitQueueSize returns the total number of tasks currently
// in the interim (wait) queue.  Those that have been
// processed from the internal task queue but are waiting
// for a worker to be free.
func (w *WorkerPool) WaitQueueSize() int32 {
	return atomic.LoadInt32(&w.waitingQueueSize)
}

// Stopped returns if the workerpool is in a stopped
// state
func (w *WorkerPool) Stopped() bool {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()
	return w.stopped
}

// Stalled returns if the workerpool is in a throttled
// state
func (w *WorkerPool) Stalled() bool {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()
	return w.stalled
}

// dispatch initialises the worker pool ready to accept
// work from the client.  This is automatically invoked
// during initialisation and is run in a asynchronously.
func (w *WorkerPool) dispatch() {
	var currentWorkers int

	// For now we haven't implemented any auto scaling
	idleTicker := time.NewTicker(time.Minute)

loop:
	for {
		// While the holding pen actually has some work to be processed
		// new tasks are enqueued there and workers will tasks will be
		// shifted from the waiting area into the worker queues.
		if currentWorkers != 0 && w.WaitQueueSize() > 0 {
			if !w.shiftTasks() {
				break loop
			}
			continue
		}

		select {
		case incomingTask, ok := <-w.incomingQueue:
			if !ok {
				break loop
			}
			select {
			case w.workerQueue <- incomingTask:
			default:
				if currentWorkers < w.maximumWorkers {
					w.spawnedWorkersWg.Add(1)
					go w.worker(incomingTask)
					currentWorkers++
					atomic.StoreInt32(&w.activeWorkers, int32(currentWorkers))
				} else {
					w.waitingQueue <- incomingTask
					atomic.StoreInt32(&w.waitingQueueSize, int32(len(w.waitingQueue)))
				}
			}
		case <-idleTicker.C:
			// TODO: implement this to allow downsizing of workers.
		}
	}
}

// shiftTasks can either take a task
// off the incomingQueue and store it in the waitingQueue
// or take a task off the waitingQueue and move it to the
// workerQueue.  If any of the incoming or waiting channels
// have been closed it returns false causing a start() exit.
func (w *WorkerPool) shiftTasks() bool {
	select {
	case incomingTask, ok := <-w.incomingQueue:
		if !ok {
			return false
		}
		w.waitingQueue <- incomingTask
	case waitingTask, ok := <-w.waitingQueue:
		if !ok {
			return false
		}
		w.workerQueue <- waitingTask
	}
	atomic.StoreInt32(&w.waitingQueueSize, int32(w.waitingQueueSize))
	return true
}

// Shutdown prevents more work from being pushed on to the worker pool
// and waits for all workers to clear down their work and the
// remaining task queue before gracefully exiting.
func (w *WorkerPool) Shutdown() {
	defer w.spawnedWorkersWg.Wait()
	defer close(w.incomingQueue)
	if !w.stopped {
		w.signalWorkerShutdown()
		w.stopped = true
	}
}

// Stall blocks until the context has been cancelled.
// It will gradually block all workers in the pool
// but they may process other tasks between the call to
// Stall
func (w *WorkerPool) Stall(ctx context.Context) {
	w.stallMutex.Lock()
	defer w.stallMutex.Unlock()
	defer func() { w.stalled = false }()
	w.stalled = true

	var readyWorkers sync.WaitGroup
	readyWorkers.Add(w.maximumWorkers)

	for i := 0; i < w.maximumWorkers; i++ {
		w.waitingQueue <- func() {
			defer readyWorkers.Done()
			<-ctx.Done()
		}

	}

	// Wait until all workers have finished waiting for the context.
	// We should of pushed (and they received) a task that solely
	// blocks until the context is cancelled.
	// TODO: race condition if stall is called before all workers are
	// spawned

	readyWorkers.Wait()

}

// signalWorkerShutdown causes the queues to flush without allowing any new
// work to enter the pool, preparing for a graceful exit.
func (w *WorkerPool) signalWorkerShutdown() {
	for i := 0; i < w.maximumWorkers; i++ {
		w.incomingQueue <- nil
	}
}

// Enqueue registers a task to the task queue ready to be picked
// up when workers are available.  Ensures that nil values cannot
// find their way into the queues.
func (w *WorkerPool) Enqueue(task TaskFunc) error {
	if task == nil {
		return ErrSubmittedNilTask
	}
	w.incomingQueue <- task
	return nil
}

// EnqueueWait registers a task to the task queue but is blocking
// until the task has been completed by a worker.  A context can be
// provided to break out when required should the processing be
// taking longer than expected.  Ensures nil values cannot make their
// way onto the queues.
func (w *WorkerPool) EnqueueWait(ctx context.Context, task TaskFunc) error {
	if task == nil {
		return ErrSubmittedNilTask
	}
	done := make(chan struct{})
	w.incomingQueue <- func() {
		task()
		done <- struct{}{}
		close(done)
	}
	for {
		select {
		// The worker pool has finished the task
		case <-done:
			return nil
		// The task took too long, consider it aborted.
		case <-ctx.Done():
			return nil
		}
	}
}

// worker continiously pulls work off the worker queue after it has received
// its first task directly from the core start loop.  It will sit idling on
// the workerQueue for future work.  The worker goroutine `nil` specifics
// serve two purposes.  The workerpool will internally Enqueue nil tasks
// for every worker when it is time to shutdown, causing them to process
// what they have and then exit.
func (w *WorkerPool) worker(task TaskFunc) {
	defer w.spawnedWorkersWg.Done()
	for task != nil {
		task()
		task = <-w.workerQueue
	}
}

// validateMaxWorkers ensures the worker pool is correctly configured
// with atleast a single worker.
func validateMaxWorkers(workers int) int {
	if workers < 1 {
		return 1
	}
	return workers
}
