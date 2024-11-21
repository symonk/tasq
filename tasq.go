package tasq

import (
	"context"
	"fmt"
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
	submittedQueue chan func()
	// Uslice for now, but reconsider data structures for future.
	interimQueue    []func()
	interimMutex    sync.Locker
	processingQueue chan func()

	// worker specifics
	maxWorkers    int
	currWorkers   int
	stopped       bool
	stoppingMutex sync.Mutex

	// shutdown specifics
	done               chan struct{}
	workerIdleDuration time.Duration

	terminate sync.Once
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
	t.submittedQueue = make(chan func())
	t.processingQueue = make(chan func())
	go t.begin()
	return t
}

// begin is the core implementation of the worker pool
// and is responsible for handling and executing tasks.
func (t *Tasq) begin() {
	defer close(t.done)
	workerIdleDuration := time.NewTimer(3 * time.Second)
	work := make(chan func())
	completedTasks := false
	var wg sync.WaitGroup

core:
	for {
		// If our slice of tasks is backfilling, there is pressure already on the
		// channel queues, keep buffering the tasks as talking directly to our other
		// queues will be blocked at this point. (implement a double ended Q)
		// There MUST be workers at this point as this queue cannot grow until the
		// processing queue has had a task and subsequently caused a worker to be spawned.
		if t.checkForBackPressure() {
			if !t.processHoldingQueue() {
				break core
			}
			// we processed a task successfully, try again.
			continue
		}

		// There is currently no tasks in the interim queue backing up, we can safely
		// look to read one from the incoming queue and slot it directly onto our
		// processing queue.
		select {
		case inboundTask, ok := <-t.submittedQueue:
			// There is a a task, some scenarios can exist now:
			// 1. Attempt to directly move the task onto the processing queue if it will accept it.
			// 2. Store the task in the interim queue if the processing queue is blocking.
			// 3. We have a task on the processing queue, do worker checks and hand the task off to the workers.
			// 4. We are in a shutting down state, so need to terminate.
			if !ok {
				// Stop() has been invoked, gracefully exit
				// TODO: implement this.
				break core
			}
			select {
			case t.processingQueue <- inboundTask:
				fmt.Println("submitted a task directly for processing!")
			case executableTask := <-t.processingQueue:
				// 3, a task needs processed, but first we need to do various different worker checks
				// such as scaling etc.  We also can update the idle checks here as we will of carried
				// out a task in the timer window.
				completedTasks = true
				if t.currWorkers < t.maxWorkers {
					t.startNewWorker(inboundTask, work, &wg)
				}
				work <- executableTask
			}
		case <-workerIdleDuration.C:
			// There have been no processed tasks for the entire duration of the idle checking duration
			// scale down one worker, down to zero.
			if completedTasks && t.currWorkers > 0 {
				t.stopWorker()
			}
			workerIdleDuration.Reset(3 * time.Second)
			completedTasks = false
		}
	}
	// teardown support
	// TODO: Implement this logic, be cautious of shutting down etc.
	wg.Wait()
}

// checkForBackPressure checks if the queue for holding tasks to be
// processed in future is populated.  If it is, there is no point
// going directly to the channels, this check is utilised to know
// if we should push onto the holden pen deque in future.
func (t *Tasq) checkForBackPressure() bool {
	return len(t.interimQueue) > 0
}

// processHoldingQueue causes the pool to process tasks that in
// in the interim queue to workers and subsequently adding new incoming
// tasks
func (t *Tasq) processHoldingQueue() bool {
	// Lock around the critical section only
	t.interimMutex.Lock()
	ele := t.interimQueue[len(t.interimQueue)-1]
	t.interimMutex.Unlock()

	if ele == nil {
		return false
	}
	t.submittedQueue <- ele
	return true
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
	t.stopped = true
	t.stoppingMutex.Unlock()

	t.terminate.Do(func() {
		close(t.submittedQueue)
		close(t.processingQueue)
		<-t.done
	})
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

// Submit is responsible for preparing a user defined task to
// be consumed by the pool (at some point in future).  This
// is not blocking, if you wish to wait until the task has been
// processed, use EnqueueWait() instead.
func (t *Tasq) Submit(task func()) {
	if task != nil && !t.stopped {
		t.submittedQueue <- task
	}
}

// SubmitWait pushes a new task onto the task queue and wraps the
// task with a done channel that waits until the task has been processed
// through all internal queues and been executed.  No return value is handed
// back to the caller here, the user should wrap their task in a closure that
// is responsible for writing results onto a channel or some other synchronisation
// mechanism.
func (t *Tasq) SubmitWait(task func()) {
	if task != nil && !t.stopped {
		done := make(chan struct{})
		t.submittedQueue <- func() {
			defer close(done)
			t.Submit(task)
		}
		<-done
	}
}

// startNewWorker spawns a new worker in a goroutine ready when handle tasks
// from the internal processing queue.
func (t *Tasq) startNewWorker(task func(), processingQ <-chan func(), wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker(task, processingQ, wg)
	}()
	t.currWorkers++
}

// stopWorker removes an idle worker from the pool, down to
// zero (0) workers when the pool is completely unutilised.
// the cost for worker creation is trivial in the larger
// scheme of things.
func (t *Tasq) stopWorker() {
	t.processingQueue <- nil
	t.currWorkers--
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
