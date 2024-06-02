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
		w.workerCount = workers
	}
}

// WithIdleTimeout is a functional option to control the maximum
// time a worker can be idle without performing a task before
// they are shutdown.
func WithIdleTimeout(timeout time.Duration) Option {
	return func(w *WorkerPool) {
		w.idleTimeout = timeout
	}
}

// Task is an encapsulation of a callable piece of work
type Task func()

// Scheduler is the core interface for something which can
// take and process tasks in a distributed manner.
type Scheduler interface {
	Shutdown()
	Stopped() bool
	Throttle(ctx context.Context)
	Throttled() bool
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
	workerCount  int
	idleTimeout  time.Duration
	taskQueue    chan Task
	workerQueue  chan Task
	waitingQueue chan Task
	waiting      int64
	stopSignal   chan struct{}
	stopped      bool
	throttled    bool
	wpMutex      sync.Mutex
	finished     chan struct{}
}

// Verify the workerpool adheres to the Scheduler interface
// at compile time.
var _ Scheduler = (*WorkerPool)(nil)

// New returns a new instance (ptr) of a worker pool and
// schedules it to start accepting tasks in parallel.
func New(opts ...Option) *WorkerPool {
	wp := &WorkerPool{
		workerCount: 1,
		taskQueue:   make(chan Task),
		workerQueue: make(chan Task),
		stopSignal:  make(chan struct{}),
		finished:    make(chan struct{}),
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
	return w.workerCount
}

// Waiting returns the total number of tasks currently
// in the interim (wait) queue.  Those that have been
// processed from the internal task queue but are waiting
// for a worker to be free.
func (w *WorkerPool) Waiting() int64 {
	return atomic.LoadInt64(&w.waiting)
}

// Stopped returns if the workerpool is in a stopped
// state
func (w *WorkerPool) Stopped() bool {
	w.wpMutex.Lock()
	defer w.wpMutex.Unlock()
	return w.stopped
}

// Throttled returns if the workerpool is in a throttled
// state
func (w *WorkerPool) Throttled() bool {
	w.wpMutex.Lock()
	defer w.wpMutex.Unlock()
	return w.throttled
}

// Start initialises the worker pool ready to accept
// work from the client.  This is automatically invoked
// during initialisation and is run in a goroutine.
func (w *WorkerPool) start() {
	defer close(w.finished)
	idle := time.NewTimer(w.idleTimeout)
	var wg sync.WaitGroup
	defer wg.Wait()

	var runningCount int
	exit := context.WithoutCancel(context.Background())

core:
	// The core worker pool loop
	for {
		select {
		case task, ok := <-w.taskQueue:
			if !ok {
				break core
			}
			// We are not running at worker capacity; there is no
			// need to store the tasks; spawn a new worker and directly
			// have it process the task.
			if runningCount < w.workerCount {
				wg.Add(1)
				go w.worker(exit, &wg)
				runningCount++
			} else {
				// All workers are active, push the task onto a queue for the
				// worker to process later.
				w.waitingQueue <- task
				atomic.StoreInt64(&w.waiting, w.Waiting())
			}

			// TODO: What to do if the taskQueue channel was closed?
			// We still need to wait for work to be finished in the other queues
		case <-idle.C:
			fmt.Println("Worker is idle!")
		}

	}
}

// Shutdown prevents more work from being pushed on to the worker pool
// and waits for all workers to clear down their work and the
// remaining task queue before gracefully exiting.
func (w *WorkerPool) Shutdown() {

}

// Throttle prevents workers from carrying out task execution.
// Useful if you need your workloads to be delayed for a duration.
// This does not prevent more tasks being enqueued onto the task
// queue, it simply puts all workers into an idle state after
// they have completed their pending task
func (w *WorkerPool) Throttle(ctx context.Context) {
}

// Enqueue registers a task to the task queue ready to be picked
// up when workers are available.  Ensures that nil values cannot
// find their way into the queues.
func (w *WorkerPool) Enqueue(task Task) {
	if task != nil {
		w.taskQueue <- task
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
	w.taskQueue <- func() {
		task()
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

// worker is responsible for retrieve worker tasks off the
// queue and executing them.  Worker blocks if there are not
// tasks for it to process.
func (w *WorkerPool) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case task := <-w.workerQueue:
			task()
		// TODO: This is no good; could leave worker queue tasks unprocessed?
		case <-ctx.Done():
			return
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
