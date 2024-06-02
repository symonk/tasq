package workerpool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSize(t *testing.T) {
	assert.Equal(t, New(WithMaxWorkers(100)).Length(), 100)
}

func TestValidateMaxWorkers(t *testing.T) {
	assert.Equal(t, New(WithMaxWorkers(0)).Length(), 1)
}

func TestNegativeMaxWorkers(t *testing.T) {
	assert.Equal(t, New(WithMaxWorkers(-100)).Length(), 1)
}

func TestIdleWorkoutTieout(t *testing.T) {
	assert.Equal(t, New(WithIdleTimeout(time.Second)).scalingTimeout, time.Second)
}

func TestWaitingQueueSize(t *testing.T) {
	pool := New()
	assert.Zero(t, pool.WaitQueueSize())
}

func TestTasksAreActuallyProcessed(t *testing.T) {
	pool := New(WithMaxWorkers(10), WithIdleTimeout(3*time.Second))
	start := time.Now()
	for i := 0; i < 10; i++ {
		pool.Enqueue(func() {
			time.Sleep(time.Microsecond)
		})
	}
	pool.Shutdown()
	elapsedDuration := int(time.Since(start).Seconds())
	assert.Less(t, elapsedDuration, 1)
}
