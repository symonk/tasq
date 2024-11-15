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
