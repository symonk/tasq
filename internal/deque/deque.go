// deque is a package that provides a basic implementation of a double ended queue.
package deque

import (
	"errors"
	"sync"
)

var (
	// ErrEmptyDeque is returned when the deque is empty.
	ErrEmptyDeque = errors.New("deque is empty")
)

// Deque is a basic implementation of a double ended queue
// with an unlimited max length (for now).
type Deque[T any] struct {
	mu       sync.RWMutex
	internal []T
}

// New returns a new pointer to an insrtance of a Deque.
func New[T any]() *Deque[T] {
	return &Deque[T]{internal: make([]T, 0)}
}

// PushLeft puts a new item at the tail of the deque.
func (d *Deque[T]) PushLeft(element T) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.internal = append([]T{element}, d.internal...)
}

// PopRight removes the head element of the deque.
// This is synchronised internally.
// TODO: not erroring here because of case return types
// and we are only calling it after checking len
func (d *Deque[T]) PopRight() T {
	d.mu.Lock()
	item := d.internal[len(d.internal)-1]
	d.internal = d.internal[:len(d.internal)-1]
	d.mu.Unlock()
	return item
}

// Length returns the length of the Deque.
func (d *Deque[T]) Length() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.internal)
}
