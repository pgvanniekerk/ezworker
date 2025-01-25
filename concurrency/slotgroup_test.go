package concurrency

import (
	"sync"
	"testing"
	"time"
)

func TestSlotGroup_AcquireRelease(t *testing.T) {
	size := uint16(3)
	slotGroup := NewSlotGroup(size)

	// Validate that the slots channel is initialized with the correct capacity.
	if cap(slotGroup.Acquire()) != int(size) {
		t.Fatalf("expected SlotGroup to have capacity %d, but got %d", size, cap(slotGroup.Acquire()))
	}

	// Acquire all slots and ensure that the channel behaves as expected.
	for i := 0; i < int(size); i++ {
		select {
		case <-slotGroup.Acquire():
			// Slot successfully acquired
		default:
			t.Fatalf("could not acquire slot %d", i+1)
		}
	}

	// Release slots and ensure they are reusable.
	for i := 0; i < int(size); i++ {
		slotGroup.Release()
		select {
		case <-slotGroup.Acquire():
			// Slot successfully reacquired
		default:
			t.Fatalf("could not reacquire slot %d", i+1)
		}
	}
}

func TestSlotGroup_ReleaseFullChannel(t *testing.T) {
	size := uint16(2)
	slotGroup := NewSlotGroup(size)

	// Fully acquire all slots.
	for i := 0; i < int(size); i++ {
		<-slotGroup.Acquire()
	}

	// Attempt to release more slots than the capacity and ensure no panic occurs.
	for i := 0; i < 5; i++ {
		slotGroup.Release()
	}
}

func TestSlotGroup_Close(t *testing.T) {
	size := uint16(2)
	slotGroup := NewSlotGroup(size)

	// Acquire a slot to simulate active usage.
	<-slotGroup.Acquire()

	// Close the SlotGroup.
	slotGroup.Close()

	// Ensure the slots channel is properly closed.
	select {
	case _, ok := <-slotGroup.Acquire():
		if ok {
			t.Fatal("expected slots channel to be closed after Close()")
		}
	default:
		// No slots should be available after close
	}

	// Ensure no panic occurs when calling Release after closure.
	slotGroup.Release()

	// Ensure no duplicate closure occurs.
	slotGroup.Close()
}

func TestSlotGroup_ConcurrentAccess(t *testing.T) {
	size := uint16(5)
	slotGroup := NewSlotGroup(size)

	// Use a WaitGroup to coordinate concurrent goroutines.
	var wg sync.WaitGroup
	wg.Add(10)

	// Create 10 goroutines to concurrently acquire and release slots.
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			select {
			case <-slotGroup.Acquire():
				slotGroup.Release()
			default:
				// No slot available
			}
		}()
	}

	// Wait for all goroutines to complete.
	wg.Wait()

	// Ensure that all slots are reusable after SlotGroup use.
	for i := 0; i < int(size); i++ {
		select {
		case <-slotGroup.Acquire():
			// Slot successfully acquired
		default:
			t.Fatalf("could not acquire slot %d after concurrent access", i+1)
		}
	}
}

func TestSlotGroup_CloseWhileInUse(t *testing.T) {
	size := uint16(2)
	slotGroup := NewSlotGroup(size)

	// Acquire one slot to simulate active usage.
	<-slotGroup.Acquire()

	// Close the SlotGroup while slots are still being used.
	go func() {
		time.Sleep(10 * time.Millisecond)
		slotGroup.Close()
	}()

	// Simulate a short delay for testing cleanup behavior.
	time.Sleep(50 * time.Millisecond)

	// Ensure no panic occurs with closure during usage.
	slotGroup.Release()
}

func TestSlotGroup_ReleaseAfterClose(t *testing.T) {
	size := uint16(3)
	slotGroup := NewSlotGroup(size)

	// Close the SlotGroup.
	slotGroup.Close()

	// Release after closure to ensure no panic occurs and no slot is reused.
	slotGroup.Release()

	select {
	case _, ok := <-slotGroup.Acquire():
		if ok {
			t.Fatal("expected slots channel to be closed, but it remains open")
		}
	default:
		// No slots should be available after close.
	}
}
