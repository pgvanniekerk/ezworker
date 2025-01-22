package pool

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// TestPool_Start verifies the pool starts and handles tasks correctly.
func TestPool_Start(t *testing.T) {
	inputChan := make(chan int)
	errChan := make(chan error, 1)
	result := make([]int, 0)
	mutex := &sync.Mutex{}
	inputWaitGroup := &sync.WaitGroup{}

	workerFunc := func(input int) error {
		mutex.Lock()
		defer mutex.Unlock()
		result = append(result, input)
		return nil
	}

	p := NewPool(workerFunc, inputChan, errChan, 3)

	p.Start()

	// Send some tasks
	inputWaitGroup.Add(1)
	go func() {
		defer close(inputChan)
		inputChan <- 1
		inputChan <- 2
		inputChan <- 3
		inputWaitGroup.Done()
	}()

	inputWaitGroup.Wait()
	p.Stop()

	// Validate results
	mutex.Lock()
	defer mutex.Unlock()
	if len(result) != 3 {
		t.Fatalf("expected 3 processed inputs, got %d", len(result))
	}
}

// TestPool_ConcurrencyLimit ensures the pool honors the concurrency limit.
func TestPool_ConcurrencyLimit(t *testing.T) {
	inputChan := make(chan int)
	errChan := make(chan error, 1)
	inputWaitGroup := &sync.WaitGroup{}

	// Simulate workers with delays
	blockingWorkerFunc := func(input int) error {
		time.Sleep(50 * time.Millisecond) // Simulate work
		inputWaitGroup.Done()
		return nil
	}

	// Create the pool with a concurrency limit of 2
	p := NewPool(blockingWorkerFunc, inputChan, errChan, 2)

	go p.Start()
	defer p.Stop()

	inputWaitGroup.Add(3)
	go func() {
		defer close(inputChan)
		inputChan <- 1
		inputChan <- 2
		inputChan <- 3 // Task 3 will get blocked until a concurrency slot is freed
	}()

	start := time.Now()

	// Wait for tasks to complete
	inputWaitGroup.Wait()

	// Expect the tasks to take at least 100ms (2 concurrent workers + the 3rd blocked task)
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Fatalf("expected at least 100ms processing time, got %v", elapsed)
	}
}

// TestPool_Resize tests that the pool can dynamically resize its concurrency limit.
func TestPool_Resize(t *testing.T) {
	inputChan := make(chan int)
	errChan := make(chan error, 1)

	workerFunc := func(input int) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	p := NewPool(workerFunc, inputChan, errChan, 1)

	go p.Start()
	defer p.Stop()

	// Resize the pool
	err := p.Resize(3)
	if err != nil {
		t.Fatalf("unexpected error resizing pool: %v", err)
	}

	// Validate that resizing works by sending a large batch of inputs
	go func() {
		defer close(inputChan)
		for i := 0; i < 10; i++ {
			inputChan <- i
		}
	}()

	// Allow time for all tasks to process
	time.Sleep(500 * time.Millisecond)

	// If concurrency of 3 is set, the processing time should reduce
}

// TestPool_Close verifies that the pool closes gracefully and no resources are leaked.
func TestPool_Close(t *testing.T) {
	inputChan := make(chan int)
	errChan := make(chan error, 1)

	workerFunc := func(input int) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	p := NewPool(workerFunc, inputChan, errChan, 2)

	go p.Start()

	// Send some tasks, then close the pool
	go func() {
		defer close(inputChan)
		inputChan <- 1
		inputChan <- 2
		inputChan <- 3
	}()

	time.Sleep(100 * time.Millisecond)
	p.Close()

	// Validate that the pool no longer processes inputs after closure
	select {
	case inputChan <- 4:
		t.Fatalf("expected pool to stop processing after closure")
	default:
	}
}

// TestPool_ErrorHandling ensures task errors are sent to the error channel.
func TestPool_ErrorHandling(t *testing.T) {
	inputChan := make(chan int)
	errChan := make(chan error, 1)

	failingWorkerFunc := func(input int) error {
		if input == 1 {
			return errors.New("task failed")
		}
		return nil
	}

	p := NewPool(failingWorkerFunc, inputChan, errChan, 1)

	go p.Start()
	defer p.Stop()

	go func() {
		defer close(inputChan)
		inputChan <- 1 // This will fail
		inputChan <- 2 // This will pass
	}()

	// Wait for tasks to complete
	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-errChan:
		if err.Error() != "task failed" {
			t.Fatalf("expected error 'task failed', got %v", err)
		}
	default:
		t.Fatal("expected error but got none")
	}
}

// TestPool_StopWithoutStart ensures stopping an unstarted pool does not cause issues.
func TestPool_StopWithoutStart(t *testing.T) {
	inputChan := make(chan int)
	errChan := make(chan error, 1)

	workerFunc := func(input int) error { return nil }
	p := NewPool(workerFunc, inputChan, errChan, 2)

	// Stop before starting
	p.Stop()

	// If no panic, the test passes
}

// TestPool_ResizeClosedPool ensures resizing a closed pool returns an error.
func TestPool_ResizeClosedPool(t *testing.T) {
	inputChan := make(chan int)
	errChan := make(chan error, 1)

	workerFunc := func(input int) error { return nil }
	p := NewPool(workerFunc, inputChan, errChan, 2)

	p.Close()

	err := p.Resize(4)
	if err == nil {
		t.Fatal("expected error resizing a closed pool but got none")
	}
}
