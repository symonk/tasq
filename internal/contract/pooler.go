package contract

import "context"

type Pooler interface {
	Enqueue(task func()) string
	EnqueueWait(task func()) string

	Stop()
	Drain()

	Throttle(ctx context.Context)
}
