package tasq

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/symonk/tasq/internal/contract"
	"github.com/symonk/tasq/internal/deque"
)

// Task is a simple function without args or a return value.
// Task types are submitted to the Tasq for processing.
// At present, users should handle return types and throttling
// on the client side.
type Task func() error

const (
	// workerIdleTimeout is the default duration that the pool checks for
	// dormant/idle workers to cause them to be shutdown.
	workerIdleTimeout = time.Second * 3
)

// Result is the generic result merged from all the worker goroutines
// and made available through the Tasq Results() channel.
type Result[T any] struct {
	Value T
	Err   error
}

// Tasq is the worker pool implementation.  It has
// three main queues.
// The task Q is a channel that accepts all tasks and stores them in the interim holding pen
// Tasks are then taken from the holding pen, into the active queue
type Tasq struct {

	// initial queue specifics, attributes relating to the queue that accepts
	// user submitted tasks initially.
	submittedTaskQueue chan func()

	// interimTaskQueue specifics, attributes relating to the store that has backpressure
	// when the processing (final) queue is blocking/full.
	interimTaskQueue *deque.Deque[func()]

	// worker queue specifics, attributes relating to the final queue that are ranged
	// over by spawned workers.
	workerTaskQueue     chan func()
	tasksInInterimQueue int32

	// worker specifics
	maxWorkers  int
	currWorkers int

	// shutdown specifics
	done          chan struct{}
	graceful      bool
	stopped       bool
	stoppingMutex sync.Mutex

	// miscallaneous specifics
	results            chan any
	once               sync.Once
	idleDurationWindow time.Duration
	breakout           chan struct{}
}

// Ensure Tasq implements Pooler
var _ contract.Pooler = (*Tasq)(nil)

// New instantiates a new Tasq instance and applies the
// appropriate functional options to it.
// Returns the new instances of Tasq
func New(maxWorkers int, opts ...Option) *Tasq {
	t := &Tasq{}

	if maxWorkers < 1 {
		t.maxWorkers = 1
	} else {
		t.maxWorkers = maxWorkers
	}

	t.idleDurationWindow = workerIdleTimeout

	for _, opt := range opts {
		opt(t)
	}

	t.submittedTaskQueue = make(chan func())
	t.interimTaskQueue = deque.New[func()]()
	t.workerTaskQueue = make(chan func())
	t.breakout = make(chan struct{})
	t.results = make(chan any)
	t.done = make(chan struct{})
	go t.begin()
	return t
}

// Results returns the results event channel where all workers are writing their
// results too.
func (t *Tasq) Results() chan any {
	return t.results
}

// begin is the core implementation of the worker pool
// and is responsible for handling and executing tasks.
func (t *Tasq) begin() {
	defer close(t.done)
	workerIdleDuration := time.NewTimer(3 * time.Second)
	completedTasks := false
	var workerWg sync.WaitGroup

core:
	for {
		// The internal interim queue is growing, move a task from there into
		// the processing queue if possible.
		if t.interimQueueSize() > 0 {
			if !t.processInterimQueueTask() {
				break core
			}
			// we processed a task successfully, try again.
			continue
		}

		// There is currently no tasks in the interim queue backing up, we can safely
		// look to read one from the incoming queue and slot it directly onto our
		// processing queue.
		select {
		case inTask, hasNotBeenStopped := <-t.submittedTaskQueue:
			if !hasNotBeenStopped {
				// Stop()/Abort() has been invoked at some point, the submitted channel has been closed,
				// get out and wait for workers to be finalized, nil tasks are distributed to all
				// of them.
				break core
			}
			select {
			// Queue the task directly to workers if not blocking
			// if no workers have been spawned, this won't be selected.
			case t.workerTaskQueue <- inTask:
			default:
				// Push the task onto the interim queue ready for processing in future.
				// the processing queue is not able to accept tasks at the moment.
				if t.currWorkers < t.maxWorkers {
					t.startNewWorker(inTask, t.workerTaskQueue, &workerWg)
				} else {
					// Enqueue the task at the tail of the internal deque
					t.interimTaskQueue.PushLeft(inTask)
					atomic.StoreInt32(&t.tasksInInterimQueue, int32(t.interimQueueSize()))
				}
			}
			completedTasks = true
		case <-workerIdleDuration.C:
			// There have been no processed tasks for the entire duration of the idle checking duration
			// scale down one worker, down to zero.
			if completedTasks && t.currWorkers > 0 {
				t.stopWorker()
			}
			workerIdleDuration.Reset(t.idleDurationWindow)
			completedTasks = false
		}
	}
	// Graceful teardown, wait for all workers to finalize
	if t.graceful {
		// send nil tasks until all workers have been stopped
		t.Drain()
	}

	// Terminate all current workers
	for t.currWorkers > 0 {
		t.stopWorker()
	}
	workerWg.Wait()
	workerIdleDuration.Stop()
	close(t.results)
}

// interimQueueSize returns the total number of tasks in the interim
// queue.
func (t *Tasq) interimQueueSize() int {
	return t.interimTaskQueue.Length()
}

// processInterimQueueTask is responsible for moving a task from the waiting
// interim queue into the worker queue (or building up the interim queue internally).
// if the instance has been stopped, returns false.
func (t *Tasq) processInterimQueueTask() bool {
	select {
	// check if we have an incoming task and enqueue it
	case task, ok := <-t.submittedTaskQueue:
		if !ok {
			return false
		}
		t.interimTaskQueue.PushLeft(task)
	// Pop off the deque and store it directly in the worker task queue
	case t.workerTaskQueue <- t.interimTaskQueue.PopRight():
	}
	atomic.StoreInt32(&t.tasksInInterimQueue, int32(t.interimQueueSize()))
	return true
}

// MaximumWorkers returns the maximum number of workers.
// workers can be scaled depending on demand, so while
// use ActiveWorkers() to get the current actual number
// of active workers
func (t *Tasq) MaximumWorkers() int {
	return t.maxWorkers
}

// CurrentWorkerCount returns the number of current workers
// in the pool.
func (t *Tasq) CurrentWorkerCount() int {
	return t.currWorkers
}

// Stop performs a graceful shutdown of the pool, preventing any
// new tasks from being enqueued but waiting until all queued tasks
// have been executed to completion.
func (t *Tasq) Stop() {
	t.terminate(true)
}

// StopAndWait immediately, abort queued tasks and exit.
func (t *Tasq) StopAndWait() {
	t.terminate(false)
}

// terminate shuts down the Tasq instance.  Depending on various
// states, the shutdown can be graceful (wait for queued tasks to
// be processed) or immediate (stop all workers and discard tasks).
func (t *Tasq) terminate(graceful bool) {
	t.once.Do(func() {
		t.stoppingMutex.Lock()
		t.stopped = true
		t.stoppingMutex.Unlock()
		t.graceful = graceful
		close(t.breakout)
		close(t.submittedTaskQueue)
	})
	// wait for begin() to exit.
	<-t.done
}

// Stopped returns the stopped state of the Tasq instance.
// This function is synchronised.
func (t *Tasq) Stopped() bool {
	t.stoppingMutex.Lock()
	defer t.stoppingMutex.Unlock()
	return t.stopped
}

// Drain prevents new tasks being enqueued and performs
// a graceful shutdown of the worker pool after all tasks
// in flight have been processed.
func (t *Tasq) Drain() {
	// flush out the interim queue until it is completely empty
	for t.interimQueueSize() > 0 {
		t.workerTaskQueue <- t.interimTaskQueue.PopRight()
		atomic.StoreInt32(&t.tasksInInterimQueue, int32(t.interimQueueSize()))
	}
}

// IsStopped returns the stopped state of the instance.
// This method is sychronised internally.
func (t *Tasq) IsStopped() bool {
	t.stoppingMutex.Lock()
	defer t.stoppingMutex.Unlock()
	return t.stopped
}

// Throttle causes blocking across the workers until
// the given context is cancelled/timed out.  This allows
// temporarily throttling the queue tasks.  Right now the
// tasks the workers have accepted prior to this being called
// will be invoked, so this is not an immediate halt, N number
// of tasks will remain attempted until the halt propagates.
// TODO: Consider a way of doing this with immediate halting
func (t *Tasq) Throttle(ctx context.Context) {
	if t.IsStopped() {
		return
	}

	var stalled sync.WaitGroup

	for i := 0; i < t.maxWorkers; i++ {
		stalled.Add(1)
		t.Submit(func() {
			defer stalled.Done()
			<-ctx.Done()
		})
	}
	stalled.Wait()
}

// Submit is responsible for preparing a user defined task to
// be consumed by the pool (at some point in future).  This
// is not blocking, if you wish to wait until the task has been
// processed, use EnqueueWait() instead.
func (t *Tasq) Submit(task func()) {
	if task != nil {
		t.submittedTaskQueue <- task
	}
}

// SubmitWait pushes a new task onto the task queue and wraps the
// task with a done channel that waits until the task has been processed
// through all internal queues and been executed.  No return value is handed
// back to the caller here, the user should wrap their task in a closure that
// is responsible for writing results onto a channel or some other synchronisation
// mechanism.
func (t *Tasq) SubmitWait(task func()) {
	if task == nil {
		return
	}
	done := make(chan struct{})
	t.submittedTaskQueue <- (func() {
		task()
		close(done)
	})
	<-done
}

// startNewWorker spawns a new worker in a goroutine ready when handle tasks
// from the internal processing queue.
func (t *Tasq) startNewWorker(task func(), processingQ <-chan func(), wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		worker(task, processingQ, wg)
	}()
	t.currWorkers++
}

// stopWorker removes an idle worker from the pool, down to
// zero (0) workers when the pool is completely unutilised.
// the cost for worker creation is trivial in the larger
// scheme of things.
func (t *Tasq) stopWorker() bool {
	select {
	case t.workerTaskQueue <- nil:
		t.currWorkers--
		return true
	default:
		return false
	}
}

// worker is responsible for processing tasks on the processing
// channel and exiting gracefully when a shutdown has been triggered.
// sending a nil task to a worker causes the worker to exit.
// a nil task is sent to a worker in order to get them to terminate/shutdown.
// user defined enqueuing does not allow nil tasks, this is special behaviour
// internally.
func worker(task func(), workerQueue <-chan func(), wg *sync.WaitGroup) {
	defer wg.Done()
	for task != nil {
		task()
		task = <-workerQueue
	}
}
