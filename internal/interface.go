package tasq

import "context"

type Pooler interface {
	Enqueue(task func())
	EnqueueWait(task func())

	Stop()
	Drain()

	Pause(ctx context.Context)
}
