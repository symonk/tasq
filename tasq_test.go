package tasq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaximumWorkersIsCorrect(t *testing.T) {
	limit := 10
	pool := New(WithMaxWorkers(limit))
	assert.Equal(t, pool.maxWorkers, limit)
}
