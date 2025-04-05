package ezworker

// Task is a generic function type that processes messages of type MSG and returns an error.
// This is the core function that will be executed by the worker pool for each message
// received from the message channel.
//
// The Task function is the heart of the worker pool, containing the actual business logic
// for processing messages. It's defined as a generic type to allow the worker pool to
// process any type of message, providing maximum flexibility for different use cases.
//
// # Responsibilities of a Task function
//
// The Task function should:
//   - Process the message of type MSG according to your application's requirements
//   - Perform any necessary validation, transformation, or business logic
//   - Handle any recoverable errors internally when possible
//   - Return nil if processing was successful
//   - Return an error if processing failed and cannot be recovered
//
// # Error Handling
//
// When a Task function returns an error, it will be passed to the error handler function
// provided when creating the worker pool. The error handler can implement custom error
// handling logic such as logging, retrying, or alerting.
//
// # Thread Safety
//
// Task functions should be thread-safe, as they will be executed concurrently by multiple
// goroutines. If your task needs to access shared resources, make sure to use appropriate
// synchronization mechanisms like mutexes or channels.
//
// # Performance Considerations
//
// Since Task functions are executed concurrently, their performance characteristics
// will affect the overall throughput of the worker pool:
//   - CPU-bound tasks: Limit concurrency to around runtime.NumCPU()
//   - I/O-bound tasks: Higher concurrency may improve throughput
//   - Long-running tasks: Consider implementing timeouts or cancellation
//
// # Examples
//
// Simple string processing task:
//
//	// A task that processes string messages
//	stringTask := func(msg string) error {
//	    fmt.Println("Processing:", msg)
//	    return nil
//	}
//
// Processing custom message types:
//
//	// Define a custom message type
//	type JobRequest struct {
//	    ID   int
//	    Data string
//	}
//
//	// Create a task function for the custom type
//	jobTask := func(job JobRequest) error {
//	    fmt.Printf("Processing job #%d with data: %s\n", job.ID, job.Data)
//
//	    // Perform validation
//	    if job.ID <= 0 {
//	        return errors.New("invalid job ID")
//	    }
//
//	    // Do some work...
//	    result, err := processData(job.Data)
//	    if err != nil {
//	        return fmt.Errorf("failed to process job #%d: %w", job.ID, err)
//	    }
//
//	    // Save the result
//	    if err := saveResult(job.ID, result); err != nil {
//	        return fmt.Errorf("failed to save result for job #%d: %w", job.ID, err)
//	    }
//
//	    return nil
//	}
//
// Task with timeout:
//
//	// A task with a timeout
//	timeoutTask := func(msg string) error {
//	    // Create a context with a timeout
//	    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	    defer cancel()
//
//	    // Create a channel to receive the result
//	    resultCh := make(chan error, 1)
//
//	    // Execute the task in a separate goroutine
//	    go func() {
//	        // Do some work...
//	        result, err := longRunningOperation(msg)
//	        if err != nil {
//	            resultCh <- err
//	            return
//	        }
//
//	        // Process the result
//	        resultCh <- nil
//	    }()
//
//	    // Wait for either the task to complete or the timeout to expire
//	    select {
//	    case err := <-resultCh:
//	        return err
//	    case <-ctx.Done():
//	        return fmt.Errorf("task timed out: %w", ctx.Err())
//	    }
//	}
type Task[MSG any] func(MSG) error
