package workerpool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSize(t *testing.T) {
	pool := New(100)
	assert.Equal(t, pool.Length(), 100)
}
