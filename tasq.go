package tasq

type Tasq struct {
	maxWorkers int
}

// New instantiates a new Tasq instance and applies the
// appropriate functional options to it.
// Returns the new instances of Tasq
func New(opts ...Option) *Tasq {
	t := &Tasq{}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t Tasq) MaxWorkers() int {
	return t.maxWorkers
}

// Stop drains all tasks in the queue and terminates the pool.
// When the pool is in a stopped state no work tasks will be
// enqueued.
func (t *Tasq) Stop() {

}
