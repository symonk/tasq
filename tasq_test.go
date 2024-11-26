package tasq

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMaximumWorkersIsCorrect(t *testing.T) {
	limit := 10
	pool := New(WithMaxWorkers(limit))
	assert.Equal(t, pool.maxWorkers, limit)
}

func TestProofOfConcept(t *testing.T) {
	p := New(WithMaxWorkers(1), WithWorkerCheckDuration(1*time.Minute))
	p.SubmitWait(func() {
		time.Sleep(time.Second)
	})
	p.Stop()
}
