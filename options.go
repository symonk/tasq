package tasq

type Option func(t *Tasq)

// WithMaxWorkers sets the maximum number of workers
// available for the pool.  Workers are spun down
// when load is low and increased upto max when busy.
func WithMaxWorkers(max int) Option {
	return func(t *Tasq) {
		t.maxWorkers = max
	}
}

// WithWaitingQueueSize configures the total buffer for
// tasks that can be stored in the 'waiting' queue.  The
// waiting queue is tasks waiting to be picked up by a worker
func WithWaitingQueueSize(size int) Option {
	return func(t *Tasq) {
		t.waitingQueueSize = size
	}
}

func WithActiveQueueSize(size int) Option {
	return func(t *Tasq) {
		t.processingQueueSize = size
	}
}
