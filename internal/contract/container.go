package contract

// Container is the interface for something which can be used as the storage
// middleware for a Tasq instance.  This enables using a deque where performance
// is critical or a priority queue where tasks are prioritised.
type Container[T any] interface {
	PushLeft(element T)
	PopRight() (T, error)
}
