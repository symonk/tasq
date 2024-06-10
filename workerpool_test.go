package workerpool

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSize(t *testing.T) {
	t.Parallel()
	assert.Equal(t, NewWorkerPool(WithMaxWorkers(100)).MaxWorkers(), 100)
}

func TestValidateMaxWorkers(t *testing.T) {
	t.Parallel()
	assert.Equal(t, NewWorkerPool(WithMaxWorkers(0)).MaxWorkers(), 1)
}

func TestNegativeMaxWorkers(t *testing.T) {
	t.Parallel()
	assert.Equal(t, NewWorkerPool(WithMaxWorkers(-100)).MaxWorkers(), 1)
}

func TestIdleWorkoutTieout(t *testing.T) {
	t.Parallel()
	assert.Equal(t, NewWorkerPool(WithIdleTimeout(time.Second)).idleCheckPeriod, time.Second)
}

func TestWaitingQueueSize(t *testing.T) {
	t.Parallel()
	pool := NewWorkerPool()
	assert.Zero(t, pool.WaitQueueSize())
}

func TestTasksAreActuallyProcessed(t *testing.T) {
	t.Parallel()
	pool := NewWorkerPool(WithMaxWorkers(1), WithIdleTimeout(3*time.Second))
	start := time.Now()
	for i := 0; i < 10; i++ {
		_ = pool.Enqueue(func() {
			time.Sleep(time.Microsecond)
		})
	}
	pool.Shutdown()
	elapsedDuration := int(time.Since(start).Seconds())
	assert.Less(t, elapsedDuration, 1)
}

func TestTaskCanBeEnqueueBlocked(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	wg.Add(1)
	pool := NewWorkerPool()
	_ = pool.EnqueueWait(context.Background(), func() { defer wg.Done() })
	pool.Shutdown()
	wg.Wait()
}

func TestErrorOnNilTaskEnqueue(t *testing.T) {
	t.Parallel()
	pool := NewWorkerPool()
	var task TaskFunc
	err := pool.Enqueue(task)
	assert.ErrorIs(t, err, ErrNilTask)
	assert.ErrorContains(t, err, "cannot submit a nil task to the pool")
}

func TestErrorOnNilTaskEnqueueWait(t *testing.T) {
	t.Parallel()
	pool := NewWorkerPool()
	var task TaskFunc
	err := pool.EnqueueWait(context.Background(), task)
	assert.ErrorIs(t, err, ErrNilTask, "cannot submit a nil task to the workerpool")
	assert.ErrorContains(t, err, "cannot submit a nil task to the pool")
}

func TestPoolPausingBlocksSuccessfully(t *testing.T) {
	t.Parallel()
	size := 20_000
	task := func() {
		time.Sleep(time.Millisecond)
	}
	pool := NewWorkerPool(WithMaxWorkers(size/10), WithBufferSize(size))
	for i := 0; i < size; i++ {
		_ = pool.Enqueue(task)
	}
	// Let the pool get off the ground!
	stall, cancel := context.WithTimeout(context.Background(), time.Duration(5)*time.Second)
	defer cancel()
	// TODO: We can have too many workers yet to receive a Pause task
	// but internally Pause() sends a task to maxWorkers
	// See gh-issue #1
	time.Sleep(time.Second)
	// Pause the pool for the duration
	pool.Stall(stall)
	// Wait until all the tasks have been fully processed.
	pool.Shutdown()
}

func TestMaxWorkersCannotExceedWaitingQueueBuffer(t *testing.T) {
	t.Parallel()
	pool := NewWorkerPool(WithMaxWorkers(1000), WithBufferSize(500))
	assert.Equal(t, pool.MaxWorkers(), 500)
}

// TODO: Test ideas
// Pausing a pool before tasks are submitted
// Enqueueing tasks on a paused pool
