package pool

import (
	"context"
	"errors"
	"github.com/pgvanniekerk/ezworker/internal/concurrency"
	"github.com/pgvanniekerk/ezworker/worker"
	"sync"
	"sync/atomic"
)

// Pool is a generic implementation of a worker pool.
// It executes tasks (of type INPUT) concurrently using a limited number of worker goroutines.
// The pool manages its lifecycle through Start, Stop, and Close operations and can dynamically
// resize the number of concurrent workers using Resize.
// Thread-safe mechanisms such as atomic operations, mutexes, and wait groups are used to ensure
// safe access to shared resources and proper synchronization during lifecycle transitions.
type Pool[INPUT any] struct {

	// limiter controls the number of concurrent workers that can process inputs.
	// It enforces a maximum concurrency level for the pool.
	limiter *concurrency.Limiter

	// started is an atomic flag that tracks whether the pool has been started.
	started *atomic.Bool

	// closed is an atomic flag that tracks whether the pool has been closed.
	// Once closed, the pool cannot be restarted or reused.
	closed *atomic.Bool

	// workerFunc is the function executed by the workers for each input.
	// This function must handle processing logic for the provided input of type INPUT.
	workerFunc worker.Func[INPUT]

	// stopCtx is used to propagate cancellation signals to stop the pool and worker goroutines.
	stopCtx context.Context

	// stopFunc is the associated cancel function for stopCtx, used to signal that the pool should stop.
	stopFunc context.CancelFunc

	// inputChan is the external input channel from which the pool receives tasks to operate on.
	inputChan <-chan INPUT

	// errChan is the output channel used to report errors encountered during task execution.
	// This channel must be consumed to avoid blocking the pool's operations.
	errChan chan<- error

	// resizeMutex ensures thread-safe resizing of the pool, preventing concurrent resizing operations.
	resizeMutex *sync.Mutex

	// startMutex is used to synchronize and ensure thread-safe operations when starting the pool.
	startMutex *sync.Mutex

	// stopMutex is used to ensure thread-safe operations when stopping the pool, preventing concurrent Stop calls.
	stopMutex *sync.Mutex

	// closeMutex is used to synchronize access to the Close method, ensuring thread-safe operation during shutdown.
	closeMutex *sync.Mutex

	// runningThreadWG tracks all active worker goroutines to ensure they finish before the pool shuts down.
	runningThreadWG *sync.WaitGroup

	// inputBuffer is an internal buffered channel that temporarily holds tasks
	// when worker slots are unavailable or when the pool is in the process of stopping.
	inputBuffer chan INPUT
}

//region Implementation

// Start initializes the worker pool and starts the main processing loop.
// It spawns a goroutine for handling incoming input messages and distributing them to workers.
// If the pool is already started or closed, the method exits without performing any actions.
func (p *Pool[INPUT]) Start() {
	p.startMutex.Lock()
	defer p.startMutex.Unlock()

	// Ensure the pool isn't closed or already started
	if p.isStarted() || p.isClosed() {
		return
	}

	// Set the state of p to "started".
	p.started.Store(true)

	// Set up the stopSignal
	p.stopCtx, p.stopFunc = context.WithCancel(context.Background())

	// Run the message processing loop asynchronously.
	go p.processMessages()

}

// Stop gracefully stops the worker pool by signaling all active workers to finish their tasks.
// Any goroutines still running are waited upon using the internal wait group.
// If the pool is already closed or not started, the method exits without performing any actions.
func (p *Pool[INPUT]) Stop() {
	p.stopMutex.Lock()
	defer p.stopMutex.Unlock()

	// Ensure the pool is not closed or is stopped
	if p.isClosed() || !p.isStarted() {
		return
	}

	// Set the state of p to "stopped".
	p.started.Store(false)

	// Notify the "message processing loop" that is needs to.
	p.stopFunc()

	// Wait for any running threads to finalize before returning.
	p.runningThreadWG.Wait()

}

// Resize dynamically adjusts the number of concurrent workers allowed by the pool.
// It stops the pool, waits for all active workers to finish, creates a new limiter with the
// specified size, and restarts the pool with the updated concurrency configuration.
// Returns an error if the specified size is 0 or if the pool has already been closed.
func (p *Pool[INPUT]) Resize(size uint16) error {
	p.resizeMutex.Lock()
	defer p.resizeMutex.Unlock()

	// Ensure p is not closed.
	if p.isClosed() {
		return nil
	}

	// Fail if size == 0.
	if size == 0 {
		return errors.New("pool size must be greater than 0")
	}

	// Stop input processing and wait for running threads to complete before
	// setting the new limiter instance.
	p.Stop()

	// Create a new limiter with the required size.
	l := concurrency.NewLimiter(size)
	p.limiter = l

	// Start input processing again with the new limiter that has the size specified.
	p.Start()

	return nil
}

// Close shuts down the pool and cleans up all resources.
// It stops all active worker goroutines, waits for them to finish, and closes internal buffers.
// Once closed, the pool cannot be started or reused.
// This method is idempotent and can be called multiple times safely.
func (p *Pool[INPUT]) Close() {
	p.closeMutex.Lock()
	defer p.closeMutex.Unlock()

	// Ensure p is not already closed.
	if p.isClosed() {
		return
	}

	// Set the state of p to "closed".
	p.closed.Store(true)

	// Signal running go-routines that they need to stop.
	p.Stop()

	// Wait for running go-routines to finish before freeing resources.
	p.runningThreadWG.Wait()

	// Free resources to allow garbage collection of the Pool's fields.
	close(p.inputBuffer)
	p.limiter.Close()
	p.runningThreadWG = nil
	p.inputChan = nil
	p.errChan = nil
	p.workerFunc = nil
	p.stopCtx = nil
	p.stopFunc = nil

}

//endregion

//region Helpers

// processMessages continuously manages input and stop signals for the pool, processing tasks or halting as needed.
func (p *Pool[INPUT]) processMessages() {

	// Continue looping until Stop() is called.
	for p.isStarted() {

		// Wait for either the Stop signal or an input message.
		select {

		// Handle the stop signal.
		case <-p.stopCtx.Done():
			break

		// Get a slot from the concurrency limiter
		case <-p.limiter.Acquire():

			p.runningThreadWG.Add(1)

			select {

			// Handle messages from the input channel.
			case input, open := <-p.inputChan:

				// If the input channel is closed, close the Pool.
				if !open {
					p.Close()
					return
				}
				go p.processInput(input)

			// Handle messages from the input buffer
			case input := <-p.inputBuffer:
				go p.processInput(input)
			}

		}

	}

}

// processInput processes a single input using the configured worker function.
// It manages the execution lifecycle by releasing the limiter slot and signaling completion
// via WaitGroup after the task.
func (p *Pool[INPUT]) processInput(input INPUT) {
	defer p.limiter.Release()
	defer p.runningThreadWG.Done()

	err := p.workerFunc(input)
	if err != nil {
		p.errChan <- err
	}
}

// isClosed is a helper method that checks if the pool has been closed.
// It returns true if the closed flag is set.
func (p *Pool[INPUT]) isClosed() bool {
	return p.closed.Load()
}

// isStarted is a helper method that checks if the pool has been started.
// It returns true if the started flag is set.
func (p *Pool[INPUT]) isStarted() bool {
	return p.started.Load()
}

//endregion

//region Constructor

func NewPool[INPUT any](workerFunc worker.Func[INPUT], inputChan <-chan INPUT, errChan chan<- error, size uint16) *Pool[INPUT] {
	return &Pool[INPUT]{
		limiter:         concurrency.NewLimiter(size),
		started:         &atomic.Bool{},
		closed:          &atomic.Bool{},
		workerFunc:      workerFunc,
		inputChan:       inputChan,
		errChan:         errChan,
		resizeMutex:     &sync.Mutex{},
		startMutex:      &sync.Mutex{},
		closeMutex:      &sync.Mutex{},
		stopMutex:       &sync.Mutex{},
		runningThreadWG: &sync.WaitGroup{},
		inputBuffer:     make(chan INPUT, size),
	}
}

//endregion
