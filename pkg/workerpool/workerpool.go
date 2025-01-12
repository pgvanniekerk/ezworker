package workerpool

// WorkerPool defines an interface designed to manage and coordinate a fixed number of
// concurrent workers or instances. It provides mechanisms to control the flow of threads
// or goroutines competing for a limited set of resources, enabling more efficient utilization
// of system resources.
//
// This type is especially useful in scenarios where controlling the number of concurrently
// executing tasks (e.g., goroutines) is required to prevent resource saturation (like CPU,
// memory, or network limits). By leveraging the `Acquire`, `Release`, and `Close` functions,
// WorkerPool ensures a straightforward and controlled execution strategy.
//
// Example usage:
//  1. When the number of allowed concurrent workers is exceeded, `Acquire` blocks and waits
//     until a worker becomes available.
//  2. When a worker finishes its task, `Release` is used to signal availability.
//  3. `Close` releases resources when the WorkerPool is no longer needed, and its usage is
//     completed.
type WorkerPool interface {

	// Acquire blocks the calling thread or goroutine until a worker/instance slot becomes
	// available. If the total number of workers is at capacity, `Acquire` ensures the calling
	// thread is paused until a worker signals availability through `Release`.
	//
	// This function is designed to handle scenarios where tasks must not proceed unless
	// a worker is available, ensuring adherence to the defined concurrency limit.
	//
	// Calling `Acquire` after `Close` will result in a panic. Ensure proper lifecycle
	// management of the WorkerPool to avoid such errors.
	//
	// Usage:
	//    - Before initiating a task, call `Acquire` to block the execution until a worker
	//      slot becomes available.
	Acquire()

	// Release is called to notify the WorkerPool that a worker or instance has completed
	// its task and is now available for reuse. By releasing the worker slot, `Release`
	// signals threads waiting on the `Acquire` call to continue execution.
	//
	// Ensure that `Release` is called after a previously acquired worker is no longer
	// needed. Failing to release a worker may lead to deadlock conditions where tasks
	// cannot proceed since all worker slots are indefinitely occupied.
	//
	// This function is particularly critical to maintaining resource flow in systems with
	// high contention for a limited pool of workers.
	//
	// Usage:
	//    - After completing a task, call `Release` to notify the system that a worker can
	//      be reassigned or reused.
	Release()

	// Close releases all resources associated with the WorkerPool and invalidates it for
	// further use. Once `Close` is called, invoking `Acquire` or `Release` will result in a
	// panic.
	//
	// This function should be used to clean up resources when a WorkerPool is no longer needed.
	// Properly closing a WorkerPool ensures that channels, semaphores, or any other associated
	// resources are freed, avoiding resource leaks.
	//
	// Key Notes:
	//    - After calling `Close`, the WorkerPool becomes unusable.
	//    - Ensure all pending tasks are completed, and no further `Acquire` or `Release` calls
	//      are made once `Close` is invoked.
	//
	// Usage:
	//    - When the lifecycle of a WorkerPool ends (e.g., all tasks complete), call `Close`
	//      to release its resources.
	Close()
}
