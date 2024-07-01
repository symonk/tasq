package tasq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	assert.Equal(t, "tasq", Name)
	assert.Equal(t, "v0.0.1", Version)
}
