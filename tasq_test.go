package tasq

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMaximumWorkersIsCorrect(t *testing.T) {
	limit := 10
	p := New(limit)
	assert.Equal(t, p.MaximumWorkers(), limit)
}

func TestProofOfConcept(t *testing.T) {
	p := New(20)
	for i := 0; i < 100; i++ {
		p.Submit(func() {
			time.Sleep(time.Millisecond)
		})
	}
	time.Sleep(time.Second * 5)
	p.Stop()
}
