package workerpool

import (
	"errors"
	"github.com/pgvanniekerk/ezworker/concurrency"
	"sync"
	"testing"
	"time"
)

// TestPool_Start verifies the workerpool starts and handles tasks correctly.
func TestPool_Start(t *testing.T) {
	inputChan := make(chan int)
	result := make([]int, 0)
	mutex := &sync.Mutex{}
	inputWaitGroup := &sync.WaitGroup{}

	p := NewWorkerPool(
		func(input int) error {
			mutex.Lock()
			defer mutex.Unlock()
			result = append(result, input)
			return nil
		},
		inputChan,
		func(err error) {},
		concurrency.NewSlotGroup(3),
	)

	p.Start()

	// Send some tasks
	inputWaitGroup.Add(1)
	go func() {
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

// TestPool_ConcurrencyLimit ensures the workerpool honors the slot group limit.
func TestPool_ConcurrencyLimit(t *testing.T) {
	inputChan := make(chan int)
	processedTasks := []int{}

	// Create the workerpool with a conclimiter limit of 2
	p := NewWorkerPool(
		func(input int) error {
			time.Sleep(50 * time.Millisecond) // Simulate work
			processedTasks = append(processedTasks, input)
			return nil
		},
		inputChan,
		func(err error) {},
		concurrency.NewSlotGroup(2),
	)

	p.Start()

	go func() {
		defer close(inputChan)
		inputChan <- 1
		inputChan <- 2
		inputChan <- 3 // Task 3 will get blocked until a conclimiter slot is freed
	}()

	time.Sleep(30 * time.Millisecond)
	p.Stop()

	// Expect only 2 tasks to be completed/stored
	if len(processedTasks) != 2 {
		t.Fatalf("expected 2 processed inputs, got %d", len(processedTasks))
	}
}

// TestPool_Resize tests that the workerpool can dynamically resize its conclimiter limit.
func TestPool_Resize(t *testing.T) {
	inputChan := make(chan int)

	p := NewWorkerPool(
		func(input int) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
		inputChan,
		func(err error) {},
		concurrency.NewSlotGroup(1),
	)

	go p.Start()
	defer p.Stop()

	// Resize the workerpool
	p.Resize(3)

	// Validate that resizing works by sending a large batch of inputs
	go func() {
		for i := 0; i < 10; i++ {
			inputChan <- i
		}
	}()

	// Allow time for all tasks to process
	time.Sleep(500 * time.Millisecond)

}

// TestPool_Close verifies that the workerpool closes gracefully and no resources are leaked.
func TestPool_Close(t *testing.T) {
	inputChan := make(chan int)

	p := NewWorkerPool(
		func(input int) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
		inputChan,
		func(err error) {},
		concurrency.NewSlotGroup(2),
	)

	p.Start()

	// Send some tasks, then close the workerpool
	go func() {
		inputChan <- 1
		inputChan <- 2
		inputChan <- 3
	}()

	time.Sleep(50 * time.Millisecond)
	p.Close()

	// Validate that the workerpool no longer processes inputs after closure
	select {
	case inputChan <- 4:
		t.Fatalf("expected workerpool to stop processing after closure")
	default:
	}

	close(inputChan)
}

// TestPool_ErrorHandling ensures task errors are sent to the error channel.
func TestPool_ErrorHandling(t *testing.T) {
	inputChan := make(chan int)
	var err error

	p := NewWorkerPool(
		func(input int) error {
			if input == 1 {
				return errors.New("task failed")
			}
			return nil
		},
		inputChan,
		func(e error) {
			err = e
		},
		concurrency.NewSlotGroup(1),
	)

	p.Start()

	go func() {
		inputChan <- 1 // This will fail
		inputChan <- 2 // This will pass
	}()

	// Wait for tasks to complete
	time.Sleep(100 * time.Millisecond)
	p.Stop()

	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "task failed" {
		t.Fatalf("expected error 'task failed', got %v", err)
	}

}

// TestPool_StopWithoutStart ensures stopping an unstarted workerpool does not cause issues.
func TestPool_StopWithoutStart(t *testing.T) {
	inputChan := make(chan int)

	p := NewWorkerPool(
		func(input int) error { return nil },
		inputChan,
		func(e error) {},
		concurrency.NewSlotGroup(2),
	)

	// Stop before starting
	p.Stop()

	// If no panic, the test passes
}

// TestPool_ResizeClosedPool ensures resizing a closed workerpool returns an error.
func TestPool_ResizeClosedPool(t *testing.T) {
	inputChan := make(chan int)

	p := NewWorkerPool(
		func(input int) error { return nil },
		inputChan,
		func(e error) {},
		concurrency.NewSlotGroup(2),
	)

	p.Close()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected error")
		}
	}()
	p.Resize(4)

}
