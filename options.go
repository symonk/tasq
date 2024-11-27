package tasq

import "time"

type Option func(t *Tasq)

// WithMaxWorkers sets the maximum number of workers
// available for the pool.  Workers are spun down
// when load is low and increased upto max when busy.
func WithMaxWorkers(max int) Option {
	return func(t *Tasq) {
		t.maxWorkers = max
	}
}

// WithWorkerCheckDuration allows configuration of the window (duration)
// where workers are automatically scaled down due to inactivity.  Workers
// can be scaled down to zero.
func WithWorkerCheckDuration(dur time.Duration) Option {
	return func(t *Tasq) {
		t.idleDurationWindow = dur
	}
}
