package workerpool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

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
		w.scalingTimeout = timeout
	}
}

// WithWaitingQueueBuffer is a functional option to control the
// buffer size for the worker queue.
func WithWaitingQueueBuffer(size int) Option {
	return func(w *WorkerPool) {
		w.waitingQueue = make(chan Task, size)
	}
}

// Task is an encapsulation of a callable piece of work
type Task func()

// Scheduler is the core interface for something which can
// take and process tasks in a distributed manner.
type Scheduler interface {
	Shutdown()
	Stopped() bool
	Stall(ctx context.Context)
	Stalled() bool
	Enqueue(task Task)
	EnqueueWait(ctx context.Context, task Task)
}

// WorkerPool is the core scheduler.  It internally manages
// a task queue and various workers up to the worker count.
// The Workerpool currently does not (yet) support a buffered
// task queue and the interim queue can grow unbounded.  Be
// careful with memory consumption.  The plan is too expose
// new options to configure buffers (if desired) in future.
type WorkerPool struct {
	maximumWorkers int
	scalingTimeout time.Duration
	// The initial Queue for tasks enqueued (unbuffered)
	incomingQueue chan Task
	// The interim holding pen before workers can process them
	waitingQueue     chan Task
	waitingQueueSize int32
	// Tasks for workers that have been moved from the interim queue
	workerQueue chan Task

	stopSignal chan struct{}
	stopped    bool
	stalled    bool
	wpMutex    sync.Mutex
	completed  chan struct{}
	wg         sync.WaitGroup
}

// Verify the workerpool adheres to the Scheduler interface
// at compile time.
var _ Scheduler = (*WorkerPool)(nil)

// New returns a new instance (ptr) of a worker pool and
// schedules it to start accepting tasks in parallel.
func New(opts ...Option) *WorkerPool {
	wp := &WorkerPool{
		// Can be configured with WithMaxWorkerCount option
		maximumWorkers: 1,
		incomingQueue:  make(chan Task),
		// Can be configured with WithWaitingQueueSize option
		waitingQueue: make(chan Task, 10),
		workerQueue:  make(chan Task),
		stopSignal:   make(chan struct{}),
		completed:    make(chan struct{}),
	}

	for _, opt := range opts {
		opt(wp)
	}
	go wp.start()

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

// Start initialises the worker pool ready to accept
// work from the client.  This is automatically invoked
// during initialisation and is run in a goroutine.
func (w *WorkerPool) start() {
	defer close(w.completed)
	var canDownScale bool
	idleChecker := time.NewTimer(w.scalingTimeout)

	var currentWorkers int
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

core:
	// The core worker pool loop
	for {
		select {
		case task, ok := <-w.incomingQueue:
			if !ok {
				// The worker incomingQueue has been closed.
				break core
			}
			select {
			// Attempt to store the task directly into the worker queue
			case w.workerQueue <- task:
			default:
				if currentWorkers < w.maximumWorkers {
					// The worker queue was blocking (full), launch another worker and pass it the
					// task to handle directly, it will continue polling for future work from the
					// worker queue.
					w.wg.Add(1)
					go w.worker(ctx, task)
					currentWorkers++
				} else {
					// We have the full capacity of workers and the worker queue is blocking.  Store
					// the task in the waiting queue to be picked up by a worker when available.
					// atomically keep the queue size accurate.
					w.workerQueue <- task
					atomic.StoreInt32(&w.waitingQueueSize, w.WaitQueueSize())
				}
			}
			canDownScale = false

		case <-idleChecker.C:
			// Continue for now; not sure how to actually scale down workers
			// What if a worker actually has work in their queue?
			continue
			if canDownScale && currentWorkers > 0 {
				currentWorkers--
				idleChecker.Reset(w.scalingTimeout)
				canDownScale = true
			}
		}
	}
	// Wait for all workers to clear down their queues.
	w.wg.Wait()
}

// Shutdown prevents more work from being pushed on to the worker pool
// and waits for all workers to clear down their work and the
// remaining task queue before gracefully exiting.
func (w *WorkerPool) Shutdown() {
	close(w.incomingQueue)
	w.wg.Wait()
}

// Stall prevents workers from carrying out task execution.
// Useful if you need your workloads to be delayed for a duration.
// This does not prevent more tasks being enqueued onto the task
// queue, it simply puts all workers into an idle state after
// they have completed their pending task
func (w *WorkerPool) Stall(ctx context.Context) {
}

// Enqueue registers a task to the task queue ready to be picked
// up when workers are available.  Ensures that nil values cannot
// find their way into the queues.
func (w *WorkerPool) Enqueue(task Task) {
	if task != nil {
		w.incomingQueue <- task
	}
}

// EnqueueWait registers a task to the task queue but is blocking
// until the task has been completed by a worker.  A context can be
// provided to break out when required should the processing be
// taking longer than expected.  Ensures nil values cannot make their
// way onto the queues.
func (w *WorkerPool) EnqueueWait(ctx context.Context, task Task) {
	if task == nil {
		return
	}
	done := make(chan struct{})
	w.incomingQueue <- func() {
		task()
		fmt.Println("completed a task!")
		done <- struct{}{}
		close(done)
	}
	for {
		select {
		// The worker pool has finished the task
		case <-done:
			return
		// The task took too long, consider it aborted.
		case <-ctx.Done():
			return
		}
	}
}

// FlushedQueuedTasks ensures the task queue is completely flushed
// through to the workers and finalised.
func (w *WorkerPool) flushTaskQueue() {

}

// worker continiously pulls work off the worker queue after it has received
// its first task directly from the core start loop.  It will sit idling on
// the workerQueue for future work.
func (w *WorkerPool) worker(ctx context.Context, task Task) {
	defer w.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for task != nil {
				task()
				task = <-w.workerQueue
			}
		}
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
