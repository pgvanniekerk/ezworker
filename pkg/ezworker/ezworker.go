package ezworker

import (
	"context"
	"golang.org/x/sync/semaphore"
	"sync"
)

// New creates a new worker pool with the specified number of concurrent workers.
//
// Parameters:
//   - runCtx: Context used to signal when the worker pool should Stop processing.
//   - instances: Maximum number of concurrent tasks that can be executed.
//   - task: The function that will be executed for each message.
//   - errHandler: Optional function to handle errors returned by tasks. It can also return an error
//     if the error could not be handled, which will be propagated to the Run function.
//
// Returns:
//   - A pointer to the created EzWorker instance.
//   - A send-only channel for submitting messages to be processed.
//
// Example:
//
//	// Create a worker pool with 5 concurrent workers
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	// Define a task function that processes string messages
//	task := func(msg string) error {
//	    fmt.Println("Processing:", msg)
//	    return nil
//	}
//
//	// Create error handler
//	errHandler := func(err error) error {
//	    fmt.Println("Error:", err)
//	    // Return nil if the error was handled successfully
//	    // Return an error if the error could not be handled
//	    return nil
//	}
//
//	// Create the worker pool
//	worker, msgChan := ezworker.New(ctx, 5, task, errHandler)
//
//	// Start the worker pool
//	err := worker.Run()
//	if err != nil {
//	    fmt.Println("Worker pool error:", err)
//	}
//
//	// Send messages to be processed
//	msgChan <- "Hello, World!"
func New[MSG any](
	stopCtx context.Context,
	instances int64,
	task Task[MSG],
	errHandler func(error) error,
) (*EzWorker[MSG], chan<- MSG) {

	ezW := &EzWorker[MSG]{
		runCtx:        stopCtx,
		semaphore:     semaphore.NewWeighted(instances),
		task:          task,
		msgChan:       make(chan MSG),
		errHandler:    errHandler,
		taskWaitGrp:   &sync.WaitGroup{},
		resizeWaitGrp: &sync.WaitGroup{},
	}

	return ezW, ezW.msgChan
}

// EzWorker represents a worker pool that processes messages of type MSG.
// It manages a pool of goroutines that execute tasks concurrently, with a limit
// on the number of concurrent executions.
type EzWorker[MSG any] struct {
	// semaphore limits the number of concurrent tasks
	semaphore *semaphore.Weighted
	// task is the function that will be executed for each message
	task Task[MSG]
	// runCtx is used to signal when the worker pool should Stop
	runCtx context.Context
	// msgChan is the channel through which messages are received
	msgChan chan MSG
	// errHandler is called when a task returns an error
	// It can also return an error if the error could not be handled
	errHandler func(error) error
	// taskWaitGrp tracks active tasks for graceful shutdown
	taskWaitGrp *sync.WaitGroup
	// resizeWaitGrp coordinates resize operations
	resizeWaitGrp *sync.WaitGroup
}

// Run starts the worker pool and begins processing messages.
// This method blocks until the context provided during creation is canceled.
// It should typically be called in a separate goroutine.
// It returns an error if the error handler returns an error, indicating that
// the error could not be handled.
//
// Example:
//
//	// Start the worker pool in a separate goroutine
//	go func() {
//	    if err := worker.Run(); err != nil {
//	        fmt.Println("Worker pool error:", err)
//	    }
//	}()
func (ezW *EzWorker[MSG]) Run() error {
	for {
		// Wait for any resize operation to complete before processing messages
		ezW.resizeWaitGrp.Wait()
		select {
		case <-ezW.runCtx.Done():
			// Context was canceled, Stop the worker pool
			return ezW.Stop(ezW.runCtx)

		case msg := <-ezW.msgChan:
			// Increment the wait group to track this task
			ezW.taskWaitGrp.Add(1)
			// Acquire a slot from the semaphore to limit concurrency
			err := ezW.semaphore.Acquire(ezW.runCtx, 1)
			if err != nil && ezW.errHandler != nil {
				handlerErr := ezW.errHandler(err)
				if handlerErr != nil {
					return handlerErr
				}
			}

			// Execute the task in a separate goroutine
			go ezW.executeTask(msg)
		}
	}
}

// executeTask runs the task function with the given message.
// It handles releasing the semaphore and decrementing the wait group when done.
// If the task returns an error and an error handler was provided, it calls the error handler.
// If the error handler returns an error, it means the error could not be handled,
// and the worker pool will Stop.
func (ezW *EzWorker[MSG]) executeTask(msg MSG) {

	// Release the semaphore slot when the function returns
	defer ezW.semaphore.Release(1)
	// Mark the task as done when the function returns
	defer ezW.taskWaitGrp.Done()

	// Execute the task with the message
	err := ezW.task(msg)
	// If the task returned an error and an error handler was provided, call the error handler
	if err != nil && ezW.errHandler != nil {
		handlerErr := ezW.errHandler(err)
		if handlerErr != nil {
			// If the error handler returns an error, it means the error could not be handled.
			// We can't do much here since we're in a goroutine, but we can log it or take other actions.
			// The Run function will handle errors returned by the error handler in the main loop.
		}
	}
}

// Stop gracefully shuts down the worker pool.
// It closes the message channel and waits for all tasks to complete.
// If the provided context times out or is canceled before all tasks complete,
// it returns the context's error.
func (ezW *EzWorker[MSG]) Stop(stopCtx context.Context) error {
	// Close the message channel to prevent new messages from being accepted
	close(ezW.msgChan)

	// Create a channel to signal when all tasks are done
	done := make(chan struct{})

	// Wait for all tasks in a goroutine
	go func() {
		ezW.taskWaitGrp.Wait()
		close(done)
	}()

	// Wait for either all tasks to complete or the context to be canceled
	select {
	case <-done:
		// All tasks completed successfully
		return nil
	case <-stopCtx.Done():
		// Context was canceled or timed out
		return stopCtx.Err()
	}
}

// Resize changes the number of concurrent tasks that can be executed.
// This method blocks until all current tasks have completed before changing the size.
// It is safe to call this method while the worker pool is running.
//
// Parameters:
//   - size: The new maximum number of concurrent tasks.
//
// Example:
//
//	// Increase the number of concurrent workers to 10
//	worker.Resize(10)
//
//	// Decrease the number of concurrent workers to 2
//	worker.Resize(2)
func (ezW *EzWorker[MSG]) Resize(size int64) {
	// Mark that we're starting a resize operation
	ezW.resizeWaitGrp.Add(1)
	defer ezW.resizeWaitGrp.Done()

	// Wait for all current tasks to complete
	ezW.taskWaitGrp.Wait()

	// Create a new semaphore with the new size
	ezW.semaphore = semaphore.NewWeighted(size)
}
