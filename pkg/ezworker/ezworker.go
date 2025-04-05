package ezworker

import (
	"context"
	"github.com/pgvanniekerk/ezapp/pkg/ezapp"
	"golang.org/x/sync/semaphore"
	"sync"
)

// New creates a new worker pool that processes messages of type MSG.
//
// This function is the entry point for using ezworker as a standalone worker pool library.
// It creates and configures an EzWorker instance with the specified parameters.
//
// Parameters:
//   - instances: The maximum number of concurrent tasks that can be executed.
//   - task: The function that will be executed for each message.
//   - msgChan: The channel through which messages are received.
//
// Returns:
//   - A pointer to the created EzWorker instance.
//
// Example:
//
//	// Create a message channel
//	msgChan := make(chan string)
//
//	// Define a task function
//	task := func(msg string) error {
//	    fmt.Println("Processing:", msg)
//	    return nil
//	}
//
//	// Create a worker pool with 5 concurrent workers
//	worker := ezworker.New(5, task, msgChan)
//
//	// Start the worker pool
//	go func() {
//	    if err := worker.Run(); err != nil {
//	        fmt.Println("Worker pool error:", err)
//	    }
//	}()
//
//	// Send messages to be processed
//	msgChan <- "Hello, World!"
func New[MSG any](
	instances int64,
	task Task[MSG],
	msgChan <-chan MSG,
) *EzWorker[MSG] {
	return &EzWorker[MSG]{
		semaphore:     semaphore.NewWeighted(instances),
		task:          task,
		msgChan:       msgChan,
		taskWaitGrp:   &sync.WaitGroup{},
		resizeWaitGrp: &sync.WaitGroup{},
	}
}

// EzWorker represents a worker pool that processes messages of type MSG.
// It manages a pool of goroutines that execute tasks concurrently, with a limit
// on the number of concurrent executions.
//
// EzWorker serves two purposes:
//  1. As a standalone worker pool library for concurrent task processing
//  2. As a Runnable component in the ezapp framework
//
// When used as a standalone library, you create an EzWorker using the New function
// and start it by calling the Run method in a goroutine.
//
// When used with ezapp, the EzWorker can be registered as a Runnable component,
// allowing it to be managed by the ezapp framework's lifecycle and logging systems.
// The `toggle:"useEzAppLogger"` tag enables integration with ezapp's logging system.
type EzWorker[MSG any] struct {
	ezapp.Runnable `toggle:"useEzAppLogger"`

	// semaphore limits the number of concurrent tasks that can be executed simultaneously.
	// It's implemented using golang.org/x/sync/semaphore.Weighted to provide efficient
	// concurrency control.
	semaphore *semaphore.Weighted

	// task is the function that will be executed for each message received.
	// It takes a message of type MSG and returns an error if processing fails.
	task Task[MSG]

	// msgChan is the channel through which messages are received for processing.
	// The worker pool reads from this channel and executes the task function
	// for each message.
	msgChan <-chan MSG

	// taskWaitGrp tracks active tasks for graceful shutdown.
	// It's incremented before starting a task and decremented when the task completes.
	// This allows the Stop method to wait for all in-progress tasks to complete.
	taskWaitGrp *sync.WaitGroup

	// resizeWaitGrp coordinates resize operations to ensure they don't interfere
	// with message processing. It's used to block the Run method while a resize
	// operation is in progress.
	resizeWaitGrp *sync.WaitGroup

	// errHandler is called when an error occurs during task execution.
	// It receives the error returned by the task function and can implement
	// custom error handling logic such as logging, retrying, or alerting.
	errHandler func(error)
}

// Run starts the worker pool and begins processing messages from the message channel.
// This method implements the ezapp.Runnable interface, allowing EzWorker to be used
// as a component in the ezapp framework.
//
// When used as a standalone worker pool, Run should be called in a separate goroutine:
//
//	go func() {
//	    if err := worker.Run(); err != nil {
//	        // Handle error
//	    }
//	}()
//
// When used with ezapp, the framework will call this method automatically when the
// application starts.
//
// The Run method will continue processing messages until one of the following occurs:
//   - The message channel is closed, in which case it returns ErrMsgChanClosed
//   - An unrecoverable error occurs during processing
//
// Returns:
//   - nil if the worker pool stops normally
//   - ErrMsgChanClosed if the message channel is closed
//   - Other errors that might occur during processing
func (ezW *EzWorker[MSG]) Run() error {
	// The main processing loop continues until the message channel is closed
	// or an error occurs
	for {
		// Wait for any resize operation to complete before processing messages.
		// This ensures that we don't process messages while the worker pool is
		// being resized, which could lead to exceeding the concurrency limit.
		ezW.resizeWaitGrp.Wait()

		// Wait for a message on the msgChan and check if the channel is closed.
		// The comma-ok idiom is used to detect if the channel is closed.
		msg, ok := <-ezW.msgChan
		if !ok {
			// If the channel is closed, return an error to signal that the
			// worker pool should stop processing.
			return ErrMsgChanClosed
		}

		// Increment the wait group to track this task. This allows the Stop
		// method to wait for all in-progress tasks to complete before shutting down.
		ezW.taskWaitGrp.Add(1)

		// Create a context for acquiring a semaphore slot. This context can be
		// canceled if the acquisition fails or when it's no longer needed.
		acquireCtx, cancel := context.WithCancel(context.Background())

		// Acquire a slot from the semaphore to limit concurrency. This ensures
		// that we don't exceed the maximum number of concurrent tasks.
		err := ezW.semaphore.Acquire(acquireCtx, 1)
		if err != nil {
			// If we can't acquire a slot, cancel the context and handle the error.
			// This could happen if the context is canceled or if there's an internal error.
			cancel()
			ezW.errHandler(err)
		}
		// Cancel the context regardless of whether the acquisition succeeded or failed,
		// as it's no longer needed after the Acquire call completes.
		cancel()

		// Execute the task in a separate goroutine to allow the main loop to
		// continue processing messages. The executeTask method handles releasing
		// the semaphore and decrementing the wait group when the task completes.
		go ezW.executeTask(msg)
	}
}

// executeTask runs the task function with the given message in a separate goroutine.
// This method is responsible for the actual execution of the task and proper cleanup
// of resources when the task completes.
//
// The method performs the following operations:
//  1. Executes the task function with the provided message
//  2. Handles any errors returned by the task function
//  3. Releases the semaphore slot to allow other tasks to execute
//  4. Decrements the wait group to signal that the task has completed
//
// This method is called by the Run method for each message received from the channel.
// It's designed to be run in a separate goroutine to allow the main loop to continue
// processing messages without waiting for each task to complete.
//
// Parameters:
//   - msg: The message to be processed by the task function
func (ezW *EzWorker[MSG]) executeTask(msg MSG) {
	// Release the semaphore slot when the function returns.
	// This ensures that the slot is always released, even if the task
	// function panics or returns an error. Using defer guarantees that
	// this cleanup code will execute regardless of how the function exits.
	defer ezW.semaphore.Release(1)

	// Mark the task as done when the function returns.
	// This decrements the taskWaitGrp counter, allowing the Stop method
	// to know when all tasks have completed. Like the semaphore release,
	// using defer ensures this happens even if the task function fails.
	defer ezW.taskWaitGrp.Done()

	// Execute the task function with the message.
	// The task function is provided by the user when creating the worker pool
	// and contains the actual business logic for processing messages.
	err := ezW.task(msg)

	// If the task function returns an error, pass it to the error handler.
	// The error handler is responsible for logging, retrying, or otherwise
	// dealing with errors that occur during task execution.
	if err != nil {
		ezW.errHandler(err)
	}
}

// Stop gracefully shuts down the worker pool.
// This method implements the ezapp.Runnable interface's Stop method, allowing EzWorker
// to be properly managed by the ezapp framework.
//
// When used as a standalone worker pool, Stop should be called to gracefully shut down
// the worker pool when it's no longer needed:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	if err := worker.Stop(ctx); err != nil {
//	    log.Printf("Failed to stop worker pool gracefully: %v", err)
//	}
//
// When used with ezapp, the framework will call this method automatically when the
// application is shutting down.
//
// The Stop method waits for all in-progress tasks to complete before returning.
// If the provided context times out or is canceled before all tasks complete,
// it returns the context's error, allowing the caller to handle timeout situations.
//
// Parameters:
//   - stopCtx: A context that can be used to set a timeout or cancel the shutdown process.
//
// Returns:
//   - nil if all tasks completed successfully before the context was canceled
//   - The context's error if the context was canceled or timed out before all tasks completed
//
// Example with timeout:
//
//	// Allow 5 seconds for graceful shutdown
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	// Try to stop gracefully, but don't wait forever
//	if err := worker.Stop(ctx); err != nil {
//	    log.Printf("Some tasks didn't complete: %v", err)
//	}
func (ezW *EzWorker[MSG]) Stop(stopCtx context.Context) error {
	// Create a channel to signal when all tasks are done.
	// This channel will be closed when all tasks have completed,
	// which will unblock the select statement below.
	done := make(chan struct{})

	// Wait for all tasks in a separate goroutine.
	// This allows us to wait for task completion without blocking
	// the current goroutine, so we can still respond to context cancellation.
	go func() {
		// Wait for all tasks to complete. This blocks until the taskWaitGrp
		// counter reaches zero, which happens when all tasks have called Done().
		ezW.taskWaitGrp.Wait()

		// Signal that all tasks are done by closing the channel.
		// This will cause the first case in the select statement to be selected.
		close(done)
	}()

	// Wait for either all tasks to complete or the context to be canceled.
	// This select statement blocks until one of the two channels receives a value.
	var err error
	select {
	case <-done:
		// All tasks completed successfully.
		// When a closed channel is read from, it returns the zero value
		// of the channel's type, which for struct{} is an empty struct.
		// We explicitly set err to nil to indicate success.
		err = nil
	case <-stopCtx.Done():
		// Context was canceled or timed out.
		// We return the context's error, which will be context.Canceled
		// or context.DeadlineExceeded, to indicate why the shutdown didn't
		// complete gracefully.
		err = stopCtx.Err()
	}

	return err
}

// Resize changes the number of concurrent tasks that can be executed by the worker pool.
// This method provides dynamic scaling capabilities, allowing you to adjust the
// concurrency level based on workload or resource availability.
//
// The method blocks until all current tasks have completed before changing the size,
// ensuring that the worker pool maintains its concurrency guarantees during the resize
// operation. It is safe to call this method while the worker pool is running.
//
// Use cases for resizing:
//   - Increase concurrency during high load periods to improve throughput
//   - Decrease concurrency during low load periods to conserve resources
//   - Adjust concurrency based on available system resources (CPU, memory)
//   - Implement adaptive scaling based on performance metrics
//
// Parameters:
//   - size: The new maximum number of concurrent tasks that can be executed.
//     This value should be greater than 0.
//
// Example - Scaling based on load:
//
//	// Increase workers during high load
//	if queueSize > highLoadThreshold {
//	    worker.Resize(20) // Scale up to 20 concurrent tasks
//	}
//
//	// Decrease workers during low load
//	if queueSize < lowLoadThreshold {
//	    worker.Resize(5) // Scale down to 5 concurrent tasks
//	}
//
// Example - Scaling based on time of day:
//
//	// During business hours, use more workers
//	if isBusinessHours() {
//	    worker.Resize(15)
//	} else {
//	    // During off-hours, use fewer workers
//	    worker.Resize(3)
//	}
func (ezW *EzWorker[MSG]) Resize(size int64) {
	// Mark that we're starting a resize operation by incrementing the resizeWaitGrp.
	// This signals to the Run method that it should wait before processing new messages,
	// preventing race conditions during the resize operation.
	ezW.resizeWaitGrp.Add(1)

	// Ensure that we always decrement the resizeWaitGrp when this function returns,
	// regardless of how it exits. This allows the Run method to continue processing
	// messages after the resize operation is complete.
	defer ezW.resizeWaitGrp.Done()

	// Wait for all current tasks to complete before changing the semaphore.
	// This ensures that we don't violate the concurrency guarantees during the
	// resize operation. All existing tasks will complete with the old concurrency
	// limit before the new limit takes effect.
	ezW.taskWaitGrp.Wait()

	// Create a new semaphore with the new size. This effectively changes the
	// maximum number of concurrent tasks that can be executed by the worker pool.
	// The old semaphore will be garbage collected.
	ezW.semaphore = semaphore.NewWeighted(size)
}
