package concurrency

import (
	"sync"
	"sync/atomic"
)

// SlotGroup is a control mechanism that enforces a maximum level of parallelism.
// It provides a fixed number of "slots" that can be acquired and released by workers.
// Thread-safe mechanisms are used to ensure safe access and prevent resource contention.
type SlotGroup struct {

	// slots is a buffered channel that represents the available worker slots.
	// Workers acquire slots from this channel to execute tasks and release them afterward.
	slots chan struct{}

	// closed is an atomic flag that indicates whether the limiter has been closed.
	// Once closed, the limiter cannot be reused, and no new slots can be acquired or released.
	closed *atomic.Bool

	// closeMutex ensures thread-safe closure of the limiter, preventing concurrent access to the Close method.
	closeMutex *sync.Mutex
}

//region Implementation

// Acquire returns the slots channel, allowing workers to acquire available slots for execution.
// Workers should read from this channel before starting work to ensure that the conclimiter
// limit is not exceeded.
// This method is non-blocking as it only returns the channel.
func (sg *SlotGroup) Acquire() <-chan struct{} {
	return sg.slots
}

// Release returns a slot back to the limiter, making it available for other workers.
// This method prevents blocking if the slots channel is already full or if the limiter has
// been closed (in which case the release is ignored entirely).
//
// Thread-safe and ensures no deadlocks occur when releasing slots.
func (sg *SlotGroup) Release() {

	// Do nothing if already closed.
	if sg.isClosed() {
		return
	}

	// Prevent blocking if the sg.slots channel's buffer is already full.
	if len(sg.slots) == cap(sg.slots) {
		return
	}

	sg.slots <- struct{}{}
}

// Close shuts down the limiter, preventing further acquisitions and releases.
// It ensures that the slots channel is safely closed and marks the limiter as closed
// using the atomic flag `closed`.
//
// This method is idempotent and prevents race conditions using a mutex.
// Once closed, the limiter cannot be reused.
func (sg *SlotGroup) Close() {
	sg.closeMutex.Lock()
	defer sg.closeMutex.Unlock()

	// Prevent close from being run twice
	if sg.isClosed() {
		return
	}

	// Drain the slots channel of any remaining objects
	for range len(sg.slots) {
		<-sg.slots
	}

	sg.closed.Store(true)
	close(sg.slots)
}

//endregion

//region Helpers

// isClosed is a helper method that checks whether the limiter has been closed.
// Returns true if the atomic `closed` flag is set.
func (sg *SlotGroup) isClosed() bool {
	return sg.closed.Load() == true
}

//endregion

//region Constructor

// NewSlotGroup initializes a new SlotGroup instance with a specified number of slots.
// The size parameter defines the maximum conclimiter level, controlling the number of
// slots that workers can acquire concurrently.
//
// Returns a fully initialized SlotGroup with the specified size.
func NewSlotGroup(size uint16) *SlotGroup {

	slotBuffer := make(chan struct{}, size)
	for i := uint16(0); i < size; i++ {
		slotBuffer <- struct{}{}
	}

	return &SlotGroup{
		slots:      slotBuffer,
		closed:     &atomic.Bool{},
		closeMutex: &sync.Mutex{},
	}
}

//endregion
