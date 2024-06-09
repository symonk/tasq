package workerpool

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSize(t *testing.T) {
	assert.Equal(t, NewWorkerPool(WithMaxWorkers(100)).Length(), 100)
}

func TestValidateMaxWorkers(t *testing.T) {
	assert.Equal(t, NewWorkerPool(WithMaxWorkers(0)).Length(), 1)
}

func TestNegativeMaxWorkers(t *testing.T) {
	assert.Equal(t, NewWorkerPool(WithMaxWorkers(-100)).Length(), 1)
}

func TestIdleWorkoutTieout(t *testing.T) {
	assert.Equal(t, NewWorkerPool(WithIdleTimeout(time.Second)).scalingTimeout, time.Second)
}

func TestWaitingQueueSize(t *testing.T) {
	pool := NewWorkerPool()
	assert.Zero(t, pool.WaitQueueSize())
}

func TestTasksAreActuallyProcessed(t *testing.T) {
	pool := NewWorkerPool(WithMaxWorkers(10), WithIdleTimeout(3*time.Second))
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
	var wg sync.WaitGroup
	wg.Add(1)
	pool := NewWorkerPool()
	_ = pool.EnqueueWait(context.Background(), func() { defer wg.Done() })
	pool.Shutdown()
	wg.Wait()
}

func TestWorkerPoolCanBeStalled(t *testing.T) {
	t.Skip()
	pool := NewWorkerPool(WithMaxWorkers(10))
	start := time.Now()
	dur := 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	defer cancel()
	for i := 0; i < 10; i++ {
		_ = pool.Enqueue(func() { time.Sleep(time.Millisecond) })
	}
	pool.Stall(ctx)
	now := time.Since(start)
	assert.Greater(t, now.Seconds(), 5*time.Second.Seconds())

}

func TestErrorOnNilTaskEnqueue(t *testing.T) {
	pool := NewWorkerPool()
	var task TaskFunc
	err := pool.Enqueue(task)
	assert.ErrorIs(t, err, ErrSubmittedNilTask)
	assert.ErrorContains(t, err, "cannot submit a nil task to the pool")
}

func TestErrorOnNilTaskEnqueueWait(t *testing.T) {
	pool := NewWorkerPool()
	var task TaskFunc
	err := pool.EnqueueWait(context.Background(), task)
	assert.ErrorIs(t, err, ErrSubmittedNilTask, "cannot submit a nil task to the workerpool")
	assert.ErrorContains(t, err, "cannot submit a nil task to the pool")
}
