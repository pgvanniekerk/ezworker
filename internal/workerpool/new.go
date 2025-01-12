package workerpool

import (
	"errors"
	"sync"
)

// New creates and initializes a new WorkerPool with the specified number of workers.
// The WorkerPool allows controlling concurrency by managing a limited pool of workers
// that can be acquired and released across goroutines. All synchronization and signaling
// are internally managed using a mutex and a condition variable.
//
// Parameters:
//   - workers: The total number of workers allowed in the pool. This value determines
//     the maximum concurrency that the pool can handle. It must be greater than 0.
//
// Returns:
//   - A pointer to a newly initialized WorkerPool instance.
//
// Panics:
//   - If the 'workers' parameter is set to 0, the function panics with an error message
//     indicating that a WorkerPool cannot be created with 0 workers.
func New(workers uint16) *WorkerPool {

	if workers == 0 {
		panic(errors.New("cannot create a WorkerPool with 0 workers"))
	}

	mu := &sync.Mutex{}

	workerPool := &WorkerPool{
		poolSize:  workers,
		poolAvail: workers,
		mu:        mu,
		cond:      sync.NewCond(mu),
	}

	return workerPool
}
