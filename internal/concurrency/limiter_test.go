package concurrency

import (
	"sync"
	"testing"
	"time"
)

func TestLimiter_AcquireRelease(t *testing.T) {
	size := uint16(3)
	limiter := NewLimiter(size)

	// Validate that the slots channel is initialized with the correct capacity.
	if cap(limiter.Acquire()) != int(size) {
		t.Fatalf("expected limiter to have capacity %d, but got %d", size, cap(limiter.Acquire()))
	}

	// Acquire all slots and ensure that the channel behaves as expected.
	for i := 0; i < int(size); i++ {
		select {
		case <-limiter.Acquire():
			// Slot successfully acquired
		default:
			t.Fatalf("could not acquire slot %d", i+1)
		}
	}

	// Release slots and ensure they are reusable.
	for i := 0; i < int(size); i++ {
		limiter.Release()
		select {
		case <-limiter.Acquire():
			// Slot successfully reacquired
		default:
			t.Fatalf("could not reacquire slot %d", i+1)
		}
	}
}

func TestLimiter_ReleaseFullChannel(t *testing.T) {
	size := uint16(2)
	limiter := NewLimiter(size)

	// Fully acquire all slots.
	for i := 0; i < int(size); i++ {
		<-limiter.Acquire()
	}

	// Attempt to release more slots than the capacity and ensure no panic occurs.
	for i := 0; i < 5; i++ {
		limiter.Release()
	}
}

func TestLimiter_Close(t *testing.T) {
	size := uint16(2)
	limiter := NewLimiter(size)

	// Acquire a slot to simulate active usage.
	<-limiter.Acquire()

	// Close the limiter.
	limiter.Close()

	// Ensure the slots channel is properly closed.
	select {
	case _, ok := <-limiter.Acquire():
		if ok {
			t.Fatal("expected slots channel to be closed after Close()")
		}
	default:
		// No slots should be available after close
	}

	// Ensure no panic occurs when calling Release after closure.
	limiter.Release()

	// Ensure no duplicate closure occurs.
	limiter.Close()
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	size := uint16(5)
	limiter := NewLimiter(size)

	// Use a WaitGroup to coordinate concurrent goroutines.
	var wg sync.WaitGroup
	wg.Add(10)

	// Create 10 goroutines to concurrently acquire and release slots.
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			select {
			case <-limiter.Acquire():
				limiter.Release()
			default:
				// No slot available
			}
		}()
	}

	// Wait for all goroutines to complete.
	wg.Wait()

	// Ensure that all slots are reusable after concurrency.
	for i := 0; i < int(size); i++ {
		select {
		case <-limiter.Acquire():
			// Slot successfully acquired
		default:
			t.Fatalf("could not acquire slot %d after concurrent access", i+1)
		}
	}
}

func TestLimiter_CloseWhileInUse(t *testing.T) {
	size := uint16(2)
	limiter := NewLimiter(size)

	// Acquire one slot to simulate active usage.
	<-limiter.Acquire()

	// Close the limiter while slots are still being used.
	go func() {
		time.Sleep(10 * time.Millisecond)
		limiter.Close()
	}()

	// Simulate a short delay for testing cleanup behavior.
	time.Sleep(50 * time.Millisecond)

	// Ensure no panic occurs with closure during usage.
	limiter.Release()
}

func TestLimiter_ReleaseAfterClose(t *testing.T) {
	size := uint16(3)
	limiter := NewLimiter(size)

	// Close the limiter.
	limiter.Close()

	// Release after closure to ensure no panic occurs and no slot is reused.
	limiter.Release()

	select {
	case _, ok := <-limiter.Acquire():
		if ok {
			t.Fatal("expected slots channel to be closed, but it remains open")
		}
	default:
		// No slots should be available after close.
	}
}
