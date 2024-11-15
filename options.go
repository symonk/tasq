package tasq

type Option func(t *Tasq)

func WithMaxWorkers(max int) Option {
	return func(t *Tasq) {
		t.maxWorkers = max
	}
}
