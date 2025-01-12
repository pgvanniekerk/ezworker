package workerpool

import (
	"errors"
	"sync"
)

// WorkerPool is a structure that manages a pool of workers to limit concurrency.
// It provides mechanisms to acquire and release workers within the pool and
// handles synchronization and signaling between goroutines using a condition variable.
type WorkerPool struct {

	// mu is a mutex that protects access to shared state in the WorkerPool,
	// such as the available worker count (poolAvail), poolSize, and 'closed' state.
	mu *sync.Mutex

	// poolSize specifies the total number of workers allowed in the pool.
	// This defines the capacity of the pool.
	poolSize uint16

	// poolAvail indicates the current number of available workers in the pool
	// that can be acquired. It is decremented or incremented when workers are
	// acquired or released, respectively.
	poolAvail uint16

	// cond is a condition variable used to signal waiting goroutines
	// when a worker becomes available or when the pool is closed. It allows
	// concurrent threads to wait efficiently until resources are ready.
	cond *sync.Cond

	// closed indicates whether the WorkerPool has been closed. Once closed,
	// no further Acquire or Release calls are allowed, and waiting threads
	// are unblocked with an appropriate signal to allow proper handling.
	closed bool
}

// Acquire blocks until a worker is available for use in the pool. If no workers
// are available, it waits until either a worker is released or until the pool is closed.
// If the WorkerPool is already closed, it panics with an error.
func (w *WorkerPool) Acquire() {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Panic if the WorkerPool has already been closed, as the channel will
	// be closed.
	if w.closed {
		panic(errors.New("cannot call Acquire on a closed WorkerPool"))
	}

	// Wait until a worker is available or the pool is closed
	if w.poolAvail == 0 {
		w.cond.Wait() // Block until signaled
		if w.closed {
			panic(errors.New("cannot call Acquire on a closed WorkerPool"))
		}
	}

	w.poolAvail-- // Remove an available worker from the pool
}

// Release returns a worker back into the pool, allowing another goroutine to acquire it.
// If the WorkerPool is closed or if releasing exceeds the maximum pool size, it panics.
func (w *WorkerPool) Release() {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Panic if the WorkerPool has already been closed, as the channel will
	// be closed.
	if w.closed {
		panic(errors.New("cannot call Release on a closed WorkerPool"))
	}

	// Ensure we do not release more than the pool size allows
	if w.poolAvail >= w.poolSize {
		panic(errors.New("cannot release more workers than are available"))
	}

	w.cond.Signal() // Signal the mutex that it can unblock.
	w.poolAvail++   // Add a worker back into the pool

}

// Close permanently closes the WorkerPool, making further Acquire and Release calls impossible.
// Any threads currently waiting on Acquire are unblocked, allowing them to check the closed state
// and handle it accordingly.
func (w *WorkerPool) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Panic if the WorkerPool has already been closed, as the channel will
	// be closed.
	if w.closed {
		panic(errors.New("cannot call Close on a closed WorkerPool"))
	}

	// Set the available workers to 0 to allow any calls currently blocking on Acquire
	// to panic.
	w.poolAvail = 0

	// Mark the pool as closed
	w.closed = true

	// Wake up all waiting goroutines
	w.cond.Broadcast()
}
