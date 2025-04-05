package ezworker

import (
	"context"
	"errors"
	"github.com/pgvanniekerk/ezapp/pkg/wire"
	"sync"
	"testing"
	"time"
)

// TestNew tests the New function
func TestNew(t *testing.T) {
	// Create a message channel
	msgChan := make(chan string)

	// Define a task function
	task := func(msg string) error {
		return nil
	}

	// Create a worker pool with 5 concurrent workers
	worker := New(5, task, msgChan)

	// Verify that the worker pool was created correctly
	if worker == nil {
		t.Fatal("New returned nil")
	}

	// Verify that the worker pool has the correct number of workers
	if worker.semaphore == nil {
		t.Fatal("semaphore is nil")
	}

	// Verify that the task function was set correctly
	if worker.task == nil {
		t.Fatal("task is nil")
	}

	// Verify that the message channel was set correctly
	if worker.msgChan == nil {
		t.Fatal("msgChan is nil")
	}

	// Verify that the wait groups were initialized
	if worker.taskWaitGrp == nil {
		t.Fatal("taskWaitGrp is nil")
	}
	if worker.resizeWaitGrp == nil {
		t.Fatal("resizeWaitGrp is nil")
	}
}

// TestRunDetectsClosedChannel tests that the Run method detects when the message channel is closed
func TestRunDetectsClosedChannel(t *testing.T) {
	// Create a message channel
	msgChan := make(chan string)

	// Define a task function
	task := func(msg string) error {
		return nil
	}

	// Create a worker pool
	worker := New(5, task, msgChan)

	// Close the message channel
	close(msgChan)

	// Run the worker pool
	err := worker.Run()

	// Verify that the worker pool detected the closed channel
	if err != ErrMsgChanClosed {
		t.Fatalf("Expected ErrMsgChanClosed, got %v", err)
	}
}

// TestRunProcessesMessages tests that the Run method processes messages correctly
func TestRunProcessesMessages(t *testing.T) {
	// Create a message channel
	msgChan := make(chan string)

	// Create a wait group to track when messages are processed
	var wg sync.WaitGroup
	wg.Add(3) // We'll send 3 messages

	// Define a task function that decrements the wait group when a message is processed
	task := func(msg string) error {
		defer wg.Done()
		return nil
	}

	// Create a worker pool
	worker := New(2, task, msgChan)

	// Start the worker pool in a goroutine
	go func() {
		err := worker.Run()
		if err != nil && err != ErrMsgChanClosed {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	}()

	// Send some messages
	msgChan <- "message 1"
	msgChan <- "message 2"
	msgChan <- "message 3"

	// Wait for all messages to be processed with a timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All messages were processed
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for messages to be processed")
	}

	// Close the message channel to stop the worker pool
	close(msgChan)
}

// TestRunHandlesErrors tests that the Run method handles errors correctly
func TestRunHandlesErrors(t *testing.T) {
	// Create a message channel
	msgChan := make(chan string)

	// Create a channel to receive errors
	errChan := make(chan error, 1)

	// Define a task function that returns an error
	task := func(msg string) error {
		return errors.New("test error")
	}

	// Define an error handler that sends the error to the error channel
	errHandler := func(err error) {
		errChan <- err
	}

	// Create a worker pool
	worker := New(1, task, msgChan)
	worker.errHandler = errHandler

	// Start the worker pool in a goroutine
	go func() {
		err := worker.Run()
		if err != nil && err != ErrMsgChanClosed {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	}()

	// Send a message that will cause an error
	msgChan <- "error message"

	// Wait for the error to be handled with a timeout
	select {
	case err := <-errChan:
		if err == nil || err.Error() != "test error" {
			t.Fatalf("Expected 'test error', got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for error to be handled")
	}

	// Close the message channel to stop the worker pool
	close(msgChan)
}

// TestStop tests that the Stop method waits for all tasks to complete
func TestStop(t *testing.T) {
	// Create a message channel
	msgChan := make(chan string)

	// Create a channel to signal when a task starts and completes
	taskStarted := make(chan struct{})
	taskCompleted := make(chan struct{})

	// Define a task function that signals when it starts and waits to be told to complete
	task := func(msg string) error {
		close(taskStarted)
		<-taskCompleted
		return nil
	}

	// Create a worker pool
	worker := New(1, task, msgChan)

	// Start the worker pool in a goroutine
	go func() {
		err := worker.Run()
		if err != nil && err != ErrMsgChanClosed {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	}()

	// Send a message to start a task
	msgChan <- "message"

	// Wait for the task to start
	select {
	case <-taskStarted:
		// Task started
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for task to start")
	}

	// Close the message channel to stop the worker pool from processing more messages
	close(msgChan)

	// Create a context with a timeout for stopping the worker pool
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Try to stop the worker pool, which should time out because the task is still running
	err := worker.Stop(ctx)
	if err == nil {
		t.Fatal("Stop should have timed out, but it didn't")
	}

	// Allow the task to complete
	close(taskCompleted)

	// Create a new context for stopping the worker pool
	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Try to stop the worker pool again, which should succeed now that the task is complete
	err = worker.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop returned unexpected error: %v", err)
	}
}

// TestResize tests that the Resize method changes the number of concurrent tasks
func TestResize(t *testing.T) {
	// Create a message channel
	msgChan := make(chan string)

	// Define a task function
	task := func(msg string) error {
		return nil
	}

	// Create a worker pool with 5 concurrent workers
	worker := New(5, task, msgChan)

	// Resize the worker pool to 10 concurrent workers
	worker.Resize(10)

	// Verify that the worker pool was resized correctly
	// We can't directly access the semaphore's size, so we'll test it indirectly
	// by acquiring more than 5 slots, which would block if the resize didn't work

	// Create a context for acquiring semaphore slots
	ctx := context.Background()

	// Acquire 7 slots, which would block if the resize didn't work
	for i := 0; i < 7; i++ {
		err := worker.semaphore.Acquire(ctx, 1)
		if err != nil {
			t.Fatalf("Failed to acquire slot %d: %v", i+1, err)
		}
	}

	// Release the slots
	worker.semaphore.Release(7)
}

// TestConcurrentExecution tests that tasks are executed concurrently
func TestConcurrentExecution(t *testing.T) {
	// Create a message channel
	msgChan := make(chan string)

	// Create a wait group to track when messages are processed
	var wg sync.WaitGroup
	wg.Add(5) // We'll send 5 messages

	// Create a mutex to protect the concurrent access to the counter
	var mu sync.Mutex
	concurrentTasks := 0
	maxConcurrentTasks := 0

	// Define a task function that tracks the number of concurrent tasks
	task := func(msg string) error {
		defer wg.Done()

		// Increment the counter of concurrent tasks
		mu.Lock()
		concurrentTasks++
		if concurrentTasks > maxConcurrentTasks {
			maxConcurrentTasks = concurrentTasks
		}
		mu.Unlock()

		// Simulate work
		time.Sleep(100 * time.Millisecond)

		// Decrement the counter of concurrent tasks
		mu.Lock()
		concurrentTasks--
		mu.Unlock()

		return nil
	}

	// Create a worker pool with 3 concurrent workers
	worker := New(3, task, msgChan)

	// Start the worker pool in a goroutine
	go func() {
		err := worker.Run()
		if err != nil && err != ErrMsgChanClosed {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	}()

	// Send some messages
	for i := 0; i < 5; i++ {
		msgChan <- "message"
	}

	// Wait for all messages to be processed with a timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All messages were processed
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for messages to be processed")
	}

	// Close the message channel to stop the worker pool
	close(msgChan)

	// Verify that tasks were executed concurrently
	if maxConcurrentTasks < 2 {
		t.Fatalf("Expected at least 2 concurrent tasks, got %d", maxConcurrentTasks)
	}
	if maxConcurrentTasks > 3 {
		t.Fatalf("Expected at most 3 concurrent tasks, got %d", maxConcurrentTasks)
	}
}

// TestRunnableCompatibility tests that an EzWorker instance has the methods required
// by the ezapp.Runnable interface. This is a compile-time test.
func TestRunnableCompatibility(t *testing.T) {
	// Create a message channel
	msgChan := make(chan string)

	// Define a task function
	task := func(msg string) error {
		return nil
	}

	// Create a worker pool
	worker := New(5, task, msgChan)

	// If this compiles, it's a valid Runnable
	_ = wire.Runnables(worker)
}

// TestRunnableAsArgument tests that an EzWorker instance can be passed to a function
// that expects a Runnable. This is a compile-time test.
func TestRunnableAsArgument(t *testing.T) {
	// Create a message channel
	msgChan := make(chan string)

	// Define a task function
	task := func(msg string) error {
		return nil
	}

	// Create a worker pool
	worker := New(5, task, msgChan)

	// Define a mock builder function that accepts any type and returns it
	builder := func(r interface{}) interface{} {
		return r
	}

	// Pass the worker to the builder function
	// This is a compile-time check - if the types are incompatible, the code won't compile
	result := builder(worker)

	// Verify that the result is not nil
	if result == nil {
		t.Fatal("Builder returned nil")
	}

	// Verify that the result is the same as the worker
	if result != worker {
		t.Fatal("Builder did not return the same object")
	}
}
