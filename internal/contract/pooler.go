package contract

import "context"

// Pooler is the interface for the underlying worker pool
type Pooler interface {
	Submit(task func())
	SubmitWait(task func())

	Stop()
	Drain()

	Throttle(ctx context.Context)
}
