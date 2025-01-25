package workerpool

import (
	"context"
	"errors"
	"github.com/pgvanniekerk/ezworker/concurrency"
	"sync"
	"sync/atomic"
)

// WorkerPool is a generic implementation of a worker workerpool.
// It executes tasks (of type INPUT) concurrently using a limited number of worker goroutines.
// The workerpool manages its lifecycle through Start, Stop, and Close operations and can dynamically
// resize the number of concurrent workers using Resize.
// Thread-safe mechanisms such as atomic operations, mutexes, and wait groups are used to ensure
// safe access to shared resources and proper synchronization during lifecycle transitions.
type WorkerPool[INPUT any] struct {

	// slotGrp controls the number of concurrent workers that can process inputs.
	// It enforces a maximum concurrency level for the workerpool.
	slotGrp SlotGroup

	// started is an atomic flag that tracks whether the workerpool has been started.
	started *atomic.Bool

	// closed is an atomic flag that tracks whether the workerpool has been closed.
	// Once closed, the workerpool cannot be restarted or reused.
	closed *atomic.Bool

	// workerFunc is the function executed by the workers for each input.
	// This function must handle processing logic for the provided input of type INPUT.
	workerFunc WorkFunc[INPUT]

	// stopCtx is used to propagate cancellation signals to stop the workerpool and worker goroutines.
	stopCtx context.Context

	// stopFunc is the associated cancel function for stopCtx, used to signal that the workerpool should stop.
	stopFunc context.CancelFunc

	// inputChan is the external input channel from which the workerpool receives tasks to operate on.
	inputChan <-chan INPUT

	// errChan is the output channel used to report errors encountered during task execution.
	// This channel must be consumed to avoid blocking the workerpool's operations.
	errChan chan<- error

	// resizeMutex ensures thread-safe resizing of the workerpool, preventing concurrent resizing operations.
	resizeMutex *sync.Mutex

	// startMutex is used to synchronize and ensure thread-safe operations when starting the workerpool.
	startMutex *sync.Mutex

	// stopMutex is used to ensure thread-safe operations when stopping the workerpool, preventing concurrent Stop calls.
	stopMutex *sync.Mutex

	// closeMutex is used to synchronize access to the Close method, ensuring thread-safe operation during shutdown.
	closeMutex *sync.Mutex

	// runningThreadWG tracks all active worker goroutines to ensure they finish before the workerpool shuts down.
	runningThreadWG *sync.WaitGroup

	// inputBuffer is an internal buffered channel that temporarily holds tasks
	// when worker slots are unavailable or when the workerpool is in the process of stopping.
	inputBuffer chan INPUT
}

//region Implementation

// Start initializes the worker workerpool and starts the main processing loop.
// It spawns a goroutine for handling incoming input messages and distributing them to workers.
// If the workerpool is already started or closed, the method exits without performing any actions.
func (p *WorkerPool[INPUT]) Start() {
	p.startMutex.Lock()
	defer p.startMutex.Unlock()

	// If the worker pool has been closed, panic as the caller should not be attempting
	// to use the pool once closed.
	if p.isClosed() {
		panic(errors.New("workerpool is closed"))
	}

	// Do nothing if the worker pool has already been started.
	if p.isStarted() {
		return
	}

	// Set the state of p to "started".
	p.started.Store(true)

	// Set up the stopSignal
	p.stopCtx, p.stopFunc = context.WithCancel(context.Background())

	// Run the message processing loop asynchronously.
	go p.processInputMessages()

}

// Stop gracefully stops the worker workerpool by signaling all active workers to finish their tasks.
// Any goroutines still running are waited upon using the internal wait group.
// If the workerpool is already closed or not started, the method exits without performing any actions.
func (p *WorkerPool[INPUT]) Stop() {
	p.stopMutex.Lock()
	defer p.stopMutex.Unlock()

	// If the worker pool has been closed, panic as the caller should not be attempting
	// to use the pool once closed.
	if p.isClosed() {
		panic(errors.New("workerpool is closed"))
	}

	// Do nothing if the worker pool has already been stopped
	if !p.isStarted() {
		return
	}

	// Set the state of p to "stopped".
	p.started.Store(false)

	// Notify the "message processing loop" that is needs to.
	p.stopFunc()

	// Wait for any running threads to finalize before returning.
	p.runningThreadWG.Wait()

}

// Resize dynamically adjusts the number of concurrent workers allowed by the workerpool.
// It stops the workerpool, waits for all active workers to finish, creates a new slotGrp with the
// specified size, and restarts the workerpool with the updated conclimiter configuration.
// Returns an error if the specified size is 0 or if the workerpool has already been closed.
func (p *WorkerPool[INPUT]) Resize(size uint16) error {
	p.resizeMutex.Lock()
	defer p.resizeMutex.Unlock()

	// If the worker pool has been closed, panic as the caller should not be attempting
	// to use the pool once closed.
	if p.isClosed() {
		panic(errors.New("workerpool is closed"))
	}

	// Panic if size == 0.
	if size == 0 {
		panic(errors.New("workerpool size must be greater than 0"))
	}

	// Stop input processing and wait for running threads to complete before
	// setting the new slotGrp instance.
	p.Stop()

	// Create a new slotGrp with the required size.
	l := concurrency.NewSlotGroup(size)
	p.slotGrp = l

	// Start input processing again with the new slotGrp that has the size specified.
	p.Start()

	return nil
}

// Close shuts down the workerpool and cleans up all resources.
// It stops all active worker goroutines, waits for them to finish, and closes internal buffers.
// Once closed, the workerpool cannot be started or reused.
// This method is idempotent and can be called multiple times safely.
func (p *WorkerPool[INPUT]) Close() {
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

	// Free resources to allow garbage collection of the WorkerPool's fields.
	close(p.inputBuffer)
	p.slotGrp.Close()
	p.runningThreadWG = nil
	p.inputChan = nil
	p.errChan = nil
	p.workerFunc = nil
	p.stopCtx = nil
	p.stopFunc = nil

}

//endregion

//region Helpers

// processInputMessages continuously manages input and stop signals for the workerpool, processing tasks or halting as needed.
func (p *WorkerPool[INPUT]) processInputMessages() {

	// Continue looping until Stop() is called.
	for p.isStarted() && !p.isClosed() {

		p.runningThreadWG.Add(1)

		// Wait for either the Stop signal or an input message.
		select {

		// Handle the stop signal.
		case <-p.stopCtx.Done():
			p.runningThreadWG.Done()
			break

		// Get a slot from the slot group
		case <-p.slotGrp.Acquire():

			select {

			// Handle messages from the input channel.
			case input, open := <-p.inputChan:

				// If the input channel is closed, close the WorkerPool.
				if !open {
					return
				}
				go p.processInput(input)

			// Handle messages from the input buffer
			case input := <-p.inputBuffer:
				go p.processInput(input)

			// Handle the stop signal.
			case <-p.stopCtx.Done():
				p.runningThreadWG.Done()
				break
			}

		}

	}

}

// processInput processes a single input using the configured worker function.
// It manages the execution lifecycle by releasing the slotGrp slot and signaling completion
// via WaitGroup after the task.
func (p *WorkerPool[INPUT]) processInput(input INPUT) {
	defer p.slotGrp.Release()
	defer p.runningThreadWG.Done()

	err := p.workerFunc(input)
	if err != nil {
		p.errChan <- err
	}
}

// isClosed is a helper method that checks if the workerpool has been closed.
// It returns true if the closed flag is set.
func (p *WorkerPool[INPUT]) isClosed() bool {
	return p.closed.Load()
}

// isStarted is a helper method that checks if the workerpool has been started.
// It returns true if the started flag is set.
func (p *WorkerPool[INPUT]) isStarted() bool {
	return p.started.Load()
}

//endregion

//region Constructor

func NewWorkerPool[INPUT any](workFunc WorkFunc[INPUT], inputChan <-chan INPUT, errChan chan<- error, slotGrp SlotGroup) *WorkerPool[INPUT] {
	return &WorkerPool[INPUT]{
		slotGrp:         slotGrp,
		started:         &atomic.Bool{},
		closed:          &atomic.Bool{},
		workerFunc:      workFunc,
		inputChan:       inputChan,
		errChan:         errChan,
		resizeMutex:     &sync.Mutex{},
		startMutex:      &sync.Mutex{},
		closeMutex:      &sync.Mutex{},
		stopMutex:       &sync.Mutex{},
		runningThreadWG: &sync.WaitGroup{},
		inputBuffer:     make(chan INPUT),
	}
}

//endregion
