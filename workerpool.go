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
	stopSignal       chan struct{}
	stopped          bool
	stalled          bool
	stallMutex       sync.Mutex
	wpMutex          sync.Mutex
	completed        chan struct{}
	wg               sync.WaitGroup
}

// Verify the workerpool adheres to the Scheduler interface
// at compile time.
var _ Scheduler = (*WorkerPool)(nil)

// NewWorkerPool returns a new instance (ptr) of a worker pool and
// schedules it to start accepting tasks in parallel.
func NewWorkerPool(opts ...Option) *WorkerPool {
	wp := &WorkerPool{
		// Can be configured with WithMaxWorkerCount option
		maximumWorkers: 5,
		incomingQueue:  make(chan TaskFunc),
		// Can be configured with WithWaitingQueueSize option
		waitingQueue: make(chan TaskFunc, 1024),
		// can be overwritten with the WithScalingTimeout option
		idleCheckPeriod: 5 * time.Second,
		workerQueue:     make(chan TaskFunc),
		stopSignal:      make(chan struct{}),
		completed:       make(chan struct{}),
	}

	// Apply functional options after defaults have been configured
	for _, opt := range opts {
		opt(wp)
	}
	go wp.dispatch()

	return wp
}

// Length returns the total number of maximum
// workers that can handle work in the pool.
func (w *WorkerPool) Length() int {
	return w.maximumWorkers
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
	w.wpMutex.Lock()
	defer w.wpMutex.Unlock()
	return w.stopped
}

// Stalled returns if the workerpool is in a throttled
// state
func (w *WorkerPool) Stalled() bool {
	w.wpMutex.Lock()
	defer w.wpMutex.Unlock()
	return w.stalled
}

// dispatch initialises the worker pool ready to accept
// work from the client.  This is automatically invoked
// during initialisation and is run in a asynchronously.
func (w *WorkerPool) dispatch() {
	defer close(w.completed)
	var isIdle bool
	scaleCheckTicker := time.NewTicker(w.idleCheckPeriod)

	var currentWorkers int

core:
	// The core worker pool loop
	for {
		// Try and flow work from either the incoming queue to the
		// waiting queue, or take a task from the waiting queue to
		// the worker queue.  If any of the other channels are closed
		// this will return false and cause an exit.
		if currentWorkers != 0 && w.WaitQueueSize() > 0 {
			if !w.flushWaitingToWorkerQueue() {
				break core
			}
			continue
		}

		select {
		case task, ok := <-w.incomingQueue:
			if !ok {
				break core
			}
			select {
			// If the (unbuffered) worker queue will accept the task, store it
			// immediately; bypassing the interim waiting queue.
			case w.workerQueue <- task:
			default:
				// check if we have spawned workers to capacity, if not simply launch a
				// worker and give it a task.  This will start the worker loop for that
				// goroutine.
				if currentWorkers < w.maximumWorkers {
					w.wg.Add(1)
					go w.worker(task)
					currentWorkers++
				} else {
					// The pool is working at capacity and the worker queue would be blocking on the send
					// Store the task in the waitingQueue to be picked up when workers become available.
					// atomically update the size of the wait queue.
					w.waitingQueue <- task
					w.waitingQueueSize = int32(len(w.waitingQueue))
				}
			}
			isIdle = false

		case <-scaleCheckTicker.C:
			// Continue for now; not sure how to actually scale down workers
			// What if a worker actually has work in their queue?
			if isIdle && currentWorkers > 0 {
				if w.terminateWorker() {
					currentWorkers--
					isIdle = true
				}
			}
		}
	}
	// Wait for all workers to clear down their queues.
	w.wg.Wait()
}

// flushWaitingToWorkerQueue can either take a task
// off the incomingQueue and store it in the waitingQueue
// or take a task off the waitingQueue and move it to the
// workerQueue.  If any of the incoming or waiting channels
// have been closed it returns false causing a start() exit.
func (w *WorkerPool) flushWaitingToWorkerQueue() bool {
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

	var idle sync.WaitGroup
	idle.Add(w.maximumWorkers)

	for i := 0; i < w.maximumWorkers; i++ {
		w.waitingQueue <- func() {
			defer idle.Done()
			<-ctx.Done()
		}

	}

	// Wait until all workers have finished waiting for the context.
	// We should of pushed (and they received) a task that solely
	// blocks until the context is cancelled.
	// TODO: race condition if stall is called before all workers are
	// spawned

	idle.Wait()

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
// the workerQueue for future work.
func (w *WorkerPool) worker(task TaskFunc) {
	defer w.wg.Done()
	for task != nil {
		task()
		task = <-w.workerQueue
	}
}

// terminateWorker attempts to break a worker out of the
// infinite for loop by sending them a nil task
func (w *WorkerPool) terminateWorker() bool {
	return false
}

// validateMaxWorkers ensures the worker pool is correctly configured
// with atleast a single worker.
func validateMaxWorkers(workers int) int {
	if workers < 1 {
		return 1
	}
	return workers
}
