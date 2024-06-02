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
	assert.Equal(t, New(WithIdleTimeout(time.Second)).idleTimeout, time.Second)
}

func TestCiFail(t *testing.T) {
	assert.False(t, true)
}
