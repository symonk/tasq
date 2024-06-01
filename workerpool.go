package workerpool

import (
	"context"
	"sync"
)

type Task func()

type Scheduler interface {
	start()
	Stop()
	Stopped() bool
	Throttle(ctx context.Context)
	Throttled() bool
	Enqueue(task Task)
	EnqueueWait(ctx context.Context, task Task)
}

// WorkerPool is the core scheduler.  It internally manages
// a task queue and various workers up to the worker count.
type WorkerPool struct {
	workerCount int
	taskQueue   chan Task
	workerQueue chan Task
	stopper     sync.Mutex
	finished    chan struct{}
}

// Verify the workerpool adheres to the Scheduler interface
// at compile time.
var _ Scheduler = (*WorkerPool)(nil)

// New returns a new instance (ptr) of a worker pool and
// schedules it to start accepting tasks in parallel.
func New(maxWorkers int) *WorkerPool {
	wp := &WorkerPool{
		workerCount: maxWorkers,
		taskQueue:   make(chan Task),
		workerQueue: make(chan Task),
		finished:    make(chan struct{}),
	}
	go wp.start()

	return wp
}

// Length returns the total number of maximum
// workers that can handle work in the pool.
func (w *WorkerPool) Length() int {
	return w.workerCount
}

// Stopped returns if the workerpool is in a stopped
// state
func (w *WorkerPool) Stop() bool {
	return false
}

// Throttled returns if the workerpool is in a throttled
// state
func (w *WorkerPool) Throttled() bool {
	return false
}

// Start initialises the worker pool ready to accept
// work from the client.  This is automatically invoked
// during initialisation and is run in a goroutine.
func (w *WorkerPool) start() {

}

// Stop prevents more work from being pushed on to the worker pool
// and waits for all workers to clear down their work and the
// remaining task queue before gracefully exiting.
func (w *WorkerPool) Stop() {

}

// Throttle prevents workers from carrying out task execution.
// Useful if you need your workloads to be delayed for a duration.
// This does not prevent more tasks being enqueued onto the task
// queue, it simply puts all workers into an idle state after
// they have completed their pending task
func (w *WorkerPool) Throttle(ctx context.Context) {
}

// Enqueue registers a task to the task queue ready to be picked
// up when workers are available.
func (w *WorkerPool) Enqueue(task Task) {

}

// EnqueueWait registers a task to the task queue but is blocking
// until the task has been completed by a worker.  A context can be
// provided to break out when required should the processing be
// taking longer than expected.
func (w *WorkerPool) EnqueueWait(ctx context.Context, task Task) {
	done := make(chan struct{})
	<-done
}
