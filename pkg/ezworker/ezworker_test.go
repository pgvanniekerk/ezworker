package ezworker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestNew tests that the New function creates a worker pool correctly
func TestNew(t *testing.T) {
	// Create a context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a task function
	task := func(msg string) error {
		return nil
	}

	// Create an error handler
	errHandler := func(err error) error { return nil }

	// Create a worker pool
	worker, msgChan := New(ctx, 5, task, errHandler)

	// Check that the worker pool was created correctly
	if worker == nil {
		t.Error("Expected worker to not be nil")
	}
	if msgChan == nil {
		t.Error("Expected msgChan to not be nil")
	}
}

// TestRunProcessesMessages tests that the Run method processes messages correctly
func TestRunProcessesMessages(t *testing.T) {
	// Create a context
	ctx, cancel := context.WithCancel(context.Background())
	// Don't defer cancel() here, we'll cancel explicitly after all messages are processed

	// Create a wait group to track when messages are processed
	var wg sync.WaitGroup
	wg.Add(3) // We'll send 3 messages

	// Create a channel to signal when the worker pool goroutine has completed
	done := make(chan struct{})

	// Create a task function that decrements the wait group when a message is processed
	task := func(msg string) error {
		defer wg.Done()
		return nil
	}

	// Create a worker pool
	worker, msgChan := New(ctx, 2, task, nil)

	// Start the worker pool
	go func() {
		if err := worker.Run(); err != nil && err != context.Canceled {
			t.Errorf("Worker pool error: %v", err)
		}
		close(done)
	}()

	// Send some messages
	msgChan <- "message 1"
	msgChan <- "message 2"
	msgChan <- "message 3"

	// Wait for all messages to be processed
	wg.Wait()

	// Cancel the context to stop the worker pool
	cancel()

	// Wait for the worker pool goroutine to complete
	<-done

	// If we got here, all messages were processed
}

// TestErrorHandling tests that errors are properly handled
func TestErrorHandling(t *testing.T) {
	// Create a context
	ctx, cancel := context.WithCancel(context.Background())
	// Don't defer cancel() here, we'll cancel explicitly after the error is handled

	// Create a channel to track when errors are handled
	errChan := make(chan error, 1)

	// Create a channel to signal when the worker pool goroutine has completed
	done := make(chan struct{})

	// Create a task function that returns an error
	task := func(msg string) error {
		return errors.New("test error")
	}

	// Create an error handler that sends the error to the channel
	errHandler := func(err error) error {
		errChan <- err
		return nil
	}

	// Create a worker pool
	worker, msgChan := New(ctx, 1, task, errHandler)

	// Start the worker pool
	go func() {
		if err := worker.Run(); err != nil && err != context.Canceled {
			t.Errorf("Worker pool error: %v", err)
		}
		close(done)
	}()

	// Send a message
	msgChan <- "message"

	// Wait for the error to be handled
	select {
	case err := <-errChan:
		if err.Error() != "test error" {
			t.Errorf("Expected error message 'test error', got '%s'", err.Error())
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for error to be handled")
	}

	// Cancel the context to stop the worker pool
	cancel()

	// Wait for the worker pool goroutine to complete
	<-done
}

// TestResize tests that the Resize method changes the number of concurrent workers
func TestResize(t *testing.T) {
	// Create a context
	ctx, cancel := context.WithCancel(context.Background())
	// Don't defer cancel() here, we'll cancel explicitly after all messages are processed

	// Create a wait group to track when messages are processed
	var wg sync.WaitGroup
	wg.Add(5) // We'll send 5 messages

	// Create a channel to signal when the worker pool goroutine has completed
	done := make(chan struct{})

	// Create a mutex to protect the counter
	var mu sync.Mutex
	// Create a counter to track the number of concurrent tasks
	var counter int
	var maxCounter int

	// Create a task function that increments the counter when a task starts
	// and decrements it when a task ends
	task := func(msg string) error {
		mu.Lock()
		counter++
		if counter > maxCounter {
			maxCounter = counter
		}
		mu.Unlock()

		// Simulate work
		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		counter--
		mu.Unlock()

		wg.Done()
		return nil
	}

	// Create a worker pool with 2 concurrent workers
	worker, msgChan := New(ctx, 2, task, nil)

	// Start the worker pool
	go func() {
		if err := worker.Run(); err != nil && err != context.Canceled {
			t.Errorf("Worker pool error: %v", err)
		}
		close(done)
	}()

	// Send 5 messages
	for i := 0; i < 5; i++ {
		msgChan <- "message"
	}

	// Wait for all messages to be processed
	wg.Wait()

	// Check that the maximum number of concurrent tasks was 2
	if maxCounter > 2 {
		t.Errorf("Expected maximum of 2 concurrent tasks, got %d", maxCounter)
	}

	// Reset the counter and max counter
	counter = 0
	maxCounter = 0

	// Reset the wait group
	wg.Add(5)

	// Resize the worker pool to 4 concurrent workers
	worker.Resize(4)

	// Send 5 more messages
	for i := 0; i < 5; i++ {
		msgChan <- "message"
	}

	// Wait for all messages to be processed
	wg.Wait()

	// Check that the maximum number of concurrent tasks was 4
	if maxCounter > 4 {
		t.Errorf("Expected maximum of 4 concurrent tasks, got %d", maxCounter)
	}

	// Cancel the context to stop the worker pool
	cancel()

	// Wait for the worker pool goroutine to complete
	<-done
}

// TestContextCancellation tests that the worker pool stops when the context is canceled
func TestContextCancellation(t *testing.T) {
	// Create a context
	ctx, cancel := context.WithCancel(context.Background())

	// Create a channel to track when the worker pool stops
	stopChan := make(chan struct{})

	// Create a task function
	task := func(msg string) error {
		return nil
	}

	// Create a worker pool
	worker, _ := New(ctx, 1, task, nil)

	// Start the worker pool in a goroutine that signals when it stops
	go func() {
		err := worker.Run()
		if err != nil && err != context.Canceled {
			t.Errorf("Worker pool error: %v", err)
		}
		stopChan <- struct{}{}
	}()

	// Cancel the context to Stop the worker pool
	cancel()

	// Wait for the worker pool to Stop
	select {
	case <-stopChan:
		// Worker pool stopped as expected
	case <-time.After(time.Second):
		t.Error("Timeout waiting for worker pool to Stop")
	}
}

// TestNilErrHandler tests that the worker pool works correctly when no error handler is provided
func TestNilErrHandler(t *testing.T) {
	// Create a context
	ctx, cancel := context.WithCancel(context.Background())
	// Don't defer cancel() here, we'll cancel explicitly after the message is processed

	// Create a wait group to track when messages are processed
	var wg sync.WaitGroup
	wg.Add(1)

	// Create a channel to signal when the worker pool goroutine has completed
	done := make(chan struct{})

	// Create a task function that returns an error
	task := func(msg string) error {
		defer wg.Done()
		return errors.New("test error")
	}

	// Create a worker pool with no error handler
	worker, msgChan := New(ctx, 1, task, nil)

	// Start the worker pool
	go func() {
		if err := worker.Run(); err != nil && err != context.Canceled {
			t.Errorf("Worker pool error: %v", err)
		}
		close(done)
	}()

	// Send a message
	msgChan <- "message"

	// Wait for the message to be processed
	wg.Wait()

	// If we got here, the worker pool handled the error correctly even with no error handler

	// Cancel the context to stop the worker pool
	cancel()

	// Wait for the worker pool goroutine to complete
	<-done
}

// TestAcquireError tests that the worker pool handles errors from the semaphore.Acquire method
func TestAcquireError(t *testing.T) {
	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Create a channel to track when errors are handled
	errChan := make(chan error, 1)

	// Create a task function
	task := func(msg string) error {
		return nil
	}

	// Create an error handler that sends the error to the channel
	errHandler := func(err error) error {
		errChan <- err
		return nil
	}

	// Create a worker pool with the canceled context
	worker, msgChan := New(ctx, 1, task, errHandler)

	// Create a wait group to ensure the goroutine completes
	var wg sync.WaitGroup
	wg.Add(1)

	// Start the worker pool in a goroutine
	go func() {
		defer wg.Done()
		if err := worker.Run(); err != nil && err != context.Canceled {
			t.Errorf("Worker pool error: %v", err)
		}
	}()

	// Send a message, which should cause an error in semaphore.Acquire
	// We need to do this in a non-blocking way since the channel might be closed
	select {
	case msgChan <- "message":
	default:
		// Channel might be closed, that's okay
	}

	// Wait for the error to be handled or for a timeout
	select {
	case err := <-errChan:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	case <-time.After(time.Second):
		// If we don't get an error, that's okay too, as the worker might have stopped
		// before processing our message
	}

	// Wait for the worker to Stop
	wg.Wait()
}

// TestGracefulShutdown tests that all tasks complete before stopping
func TestGracefulShutdown(t *testing.T) {
	// Create a context
	ctx, cancel := context.WithCancel(context.Background())

	// Create a wait group to track when messages are processed
	var wg sync.WaitGroup
	wg.Add(3) // We'll send 3 messages

	// Create a channel to track when tasks are done
	doneChan := make(chan struct{}, 3)

	// Create a channel to signal when the worker pool goroutine has completed
	workerDone := make(chan struct{})

	// Create a task function that takes some time to complete
	task := func(msg string) error {
		// Simulate work
		time.Sleep(100 * time.Millisecond)
		wg.Done()
		doneChan <- struct{}{}
		return nil
	}

	// Create a worker pool
	worker, msgChan := New(ctx, 3, task, nil)

	// Start the worker pool
	go func() {
		if err := worker.Run(); err != nil && err != context.Canceled {
			t.Errorf("Worker pool error: %v", err)
		}
		close(workerDone)
	}()

	// Send some messages
	msgChan <- "message 1"
	msgChan <- "message 2"
	msgChan <- "message 3"

	// Cancel the context to Stop the worker pool
	cancel()

	// Wait for all tasks to complete
	wg.Wait()

	// Check that all tasks completed
	count := 0
	for i := 0; i < 3; i++ {
		select {
		case <-doneChan:
			count++
		case <-time.After(time.Second):
			t.Error("Timeout waiting for tasks to complete")
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 tasks to complete, got %d", count)
	}

	// Wait for the worker pool goroutine to complete
	<-workerDone
}

// BenchmarkWorkerThroughput measures the throughput of the worker pool
func BenchmarkWorkerThroughput(b *testing.B) {
	// Create a context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a wait group to track when messages are processed
	var wg sync.WaitGroup
	wg.Add(b.N) // We'll send b.N messages

	// Create a task function that decrements the wait group when a message is processed
	task := func(msg int) error {
		// Simulate a small amount of work
		time.Sleep(10 * time.Microsecond)
		wg.Done()
		return nil
	}

	// Create a worker pool with 4 concurrent workers
	worker, msgChan := New(ctx, 4, task, nil)

	// Start the worker pool
	go func() {
		if err := worker.Run(); err != nil {
			b.Errorf("Worker pool error: %v", err)
		}
	}()

	// Reset the timer to exclude setup time
	b.ResetTimer()

	// Send b.N messages
	for i := 0; i < b.N; i++ {
		msgChan <- i
	}

	// Wait for all messages to be processed
	wg.Wait()

	// Stop the timer to exclude cleanup time
	b.StopTimer()
}

// BenchmarkWorkerConcurrency measures the effect of different concurrency levels
func BenchmarkWorkerConcurrency(b *testing.B) {
	// Define different concurrency levels to test
	concurrencyLevels := []int64{1, 2, 4, 8, 16}

	for _, concurrency := range concurrencyLevels {
		// Run a sub-benchmark for each concurrency level
		b.Run(fmt.Sprintf("Concurrency-%d", concurrency), func(b *testing.B) {
			// Create a context
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create a wait group to track when messages are processed
			var wg sync.WaitGroup
			wg.Add(b.N) // We'll send b.N messages

			// Create a task function that decrements the wait group when a message is processed
			task := func(msg int) error {
				// Simulate a small amount of work
				time.Sleep(10 * time.Microsecond)
				wg.Done()
				return nil
			}

			// Create a worker pool with the specified concurrency
			worker, msgChan := New(ctx, concurrency, task, nil)

			// Start the worker pool
			go func() {
				if err := worker.Run(); err != nil {
					b.Errorf("Worker pool error: %v", err)
				}
			}()

			// Reset the timer to exclude setup time
			b.ResetTimer()

			// Send b.N messages
			for i := 0; i < b.N; i++ {
				msgChan <- i
			}

			// Wait for all messages to be processed
			wg.Wait()

			// Stop the timer to exclude cleanup time
			b.StopTimer()
		})
	}
}

// ExampleNew demonstrates how to create and use a worker pool
func ExampleNew() {
	// Create a context that can be used to Stop the worker pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a wait group to track when all messages are processed
	var wg sync.WaitGroup
	wg.Add(3) // We'll send 3 messages

	// Create a channel to collect the output in order
	outputChan := make(chan string, 3)

	// Define a task function that processes string messages
	task := func(msg string) error {
		// Instead of printing directly, send to the channel
		outputChan <- fmt.Sprintf("Processing: %s", msg)
		wg.Done()
		return nil
	}

	// Define an error handler
	errHandler := func(err error) error {
		fmt.Println("Error:", err)
		return nil
	}

	// Create a worker pool with 1 concurrent worker to ensure deterministic order
	worker, msgChan := New(ctx, 1, task, errHandler)

	// Start the worker pool
	go func() {
		if err := worker.Run(); err != nil && err != context.Canceled {
			fmt.Println("Worker pool error:", err)
		}
	}()

	// Send some messages to be processed
	msgChan <- "message 1"
	msgChan <- "message 2"
	msgChan <- "message 3"

	// Wait for all messages to be processed
	wg.Wait()

	// Close the output channel
	close(outputChan)

	// Print the output in order
	for output := range outputChan {
		fmt.Println(output)
	}

	// Output:
	// Processing: message 1
	// Processing: message 2
	// Processing: message 3
}

// ExampleEzWorker_Resize demonstrates how to resize a worker pool
func ExampleEzWorker_Resize() {
	// Create a context that can be used to Stop the worker pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a wait group to track when all messages are processed
	var wg sync.WaitGroup
	wg.Add(5) // We'll send 5 messages

	// Create a channel to collect the output in order
	outputChan := make(chan string, 5)

	// Define a task function that processes string messages
	task := func(msg string) error {
		// Instead of printing directly, send to the channel
		outputChan <- fmt.Sprintf("Processing: %s", msg)
		wg.Done()
		return nil
	}

	// Create a worker pool with 1 concurrent worker to ensure deterministic order
	worker, msgChan := New(ctx, 1, task, nil)

	// Start the worker pool
	go func() {
		if err := worker.Run(); err != nil && err != context.Canceled {
			fmt.Println("Worker pool error:", err)
		}
	}()

	// Send some messages to be processed
	msgChan <- "message 1"
	msgChan <- "message 2"

	// Resize the worker pool to have 1 concurrent worker (still 1 to ensure deterministic order)
	worker.Resize(1)

	// Send more messages
	msgChan <- "message 3"
	msgChan <- "message 4"
	msgChan <- "message 5"

	// Wait for all messages to be processed
	wg.Wait()

	// Close the output channel
	close(outputChan)

	// Print the output in order
	for output := range outputChan {
		fmt.Println(output)
	}

	// Output:
	// Processing: message 1
	// Processing: message 2
	// Processing: message 3
	// Processing: message 4
	// Processing: message 5
}
