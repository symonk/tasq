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
	p := New(WithMaxWorkers(20))
	for i := 0; i < 100; i++ {
		p.Submit(func() {
			time.Sleep(time.Millisecond)
		})
	}
	time.Sleep(time.Second * 5)
	p.Stop()
}
