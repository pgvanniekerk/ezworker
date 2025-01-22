package pool

// Pool defines the interface for a generic worker pool.
// A worker pool manages a limited number of goroutines to process tasks concurrently.
// It provides lifecycle methods to start, stop, resize, and close the pool.
// Implementations of this interface must ensure thread safety and proper resource handling.
type Pool interface {

	// Start begins the worker pool's processing loop.
	// It allows the pool to start accepting tasks and distributing them to worker goroutines.
	// If the pool has already been started or closed, this method should be idempotent.
	Start()

	// Stop gracefully stops the worker pool.
	// It signals all active workers to finish processing their current tasks and prevents new tasks from being processed.
	// The pool remains in a reusable state, allowing it to be started again if necessary.
	Stop()

	// Resize adjusts the concurrency level of the pool.
	// This method dynamically changes the number of workers or goroutines the pool uses to process tasks.
	// An implementation should ensure resizing does not disrupt or lose any in-progress tasks.
	// The method returns an error if the provided size is invalid (e.g., less than 1) or if the pool has been closed.
	Resize(uint16) error

	// Close terminates the pool and releases all resources associated with it.
	// This includes stopping all workers, waiting for any in-progress tasks to complete, and cleaning up any internal resources.
	// Once closed, the pool cannot be restarted or resized, and all further calls to its methods should be no-ops.
	Close()
}
