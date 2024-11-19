package contract

import "context"

// Pooler is the interface for the underlying worker pool
type Pooler interface {
	Enqueue(task func()) string
	EnqueueWait(task func()) string

	Stop()
	Drain()

	Throttle(ctx context.Context)
}
