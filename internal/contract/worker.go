package contract

// Worker is the interface for something which can process
// user defined tasks.
type Worker interface {
	Run()
	Terminate()
}
