package workerpool

import "github.com/pgvanniekerk/ezworker/internal/workerpool"

// New creates and initializes a new instance of WorkerPool, which manages and limits the
// number of concurrently executing workers (goroutines) through a semaphore-based implementation.
//
// The WorkerPool is particularly useful in scenarios where you need to control the number
// of simultaneous tasks to prevent resource exhaustion (e.g., CPU, memory, or network limits).
// It provides a concurrency-controlled environment where tasks can wait for available workers,
// execute, and release worker slots when done.
//
// Arguments:
//   - workers (uint16): Specifies the maximum number of concurrent workers allowed.
//     Must be greater than 0, or an error will be returned.
//
// Returns:
//   - WorkerPool: An initialized WorkerPool instance implementing the WorkerPool interface.
//   - error: An error if the provided `workers` is 0.
//
// Usage Example:
//
//	// Create a WorkerPool to limit the concurrency of goroutines to 5.
//	wg, err := workerpool.New(5)
//	if err != nil {
//	    log.Fatalf("Failed to create WorkerPool: %v", err)
//	}
//
//	// Simulate concurrent tasks
//	for i := 0; i < 10; i++ {
//	    wg.Acquire() // Acquire until a worker is available
//
//	    go func(taskID int) {
//	        defer wg.Release() // Release the worker slot when done
//
//	        // Perform the task (example logic here)
//	        fmt.Printf("Processing task: %d\n", taskID)
//	        time.Sleep(time.Second)
//	    }(i)
//	}
//
//	// Close ensures no more tasks access the WorkerPool resources
//	wg.Close()
//
// Notes:
//   - Always call `Release` after `Acquire` for each worker to avoid deadlocks.
//     A failure to call `Release` will result in a bottleneck where no further tasks can proceed.
//   - After calling `Close`, the WorkerPool becomes invalid, and subsequent calls to `Acquire` or
//     `Release` will result in a runtime panic. Be sure to manage the lifecycle of the WorkerPool carefully.
//   - Properly choose the `workers` parameter based on the level of acceptable concurrency for
//     your specific workload.
func New(workers uint16) WorkerPool {
	return workerpool.New(workers)
}
