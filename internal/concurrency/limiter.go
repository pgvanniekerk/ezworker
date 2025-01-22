package concurrency

import (
	"sync"
	"sync/atomic"
)

// Limiter is a concurrency control mechanism that enforces a maximum level of parallelism.
// It provides a fixed number of "slots" that can be acquired and released by workers.
// Thread-safe mechanisms are used to ensure safe access and prevent resource contention.
type Limiter struct {

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
// Workers should read from this channel before starting work to ensure that the concurrency
// limit is not exceeded.
// This method is non-blocking as it only returns the channel.
func (l *Limiter) Acquire() <-chan struct{} {
	return l.slots
}

// Release returns a slot back to the limiter, making it available for other workers.
// This method prevents blocking if the slots channel is already full or if the limiter has
// been closed (in which case the release is ignored entirely).
//
// Thread-safe and ensures no deadlocks occur when releasing slots.
func (l *Limiter) Release() {

	// Do nothing if already closed.
	if l.isClosed() {
		return
	}

	// Prevent blocking if the l.slots channel's buffer is already full.
	if len(l.slots) == cap(l.slots) {
		return
	}

	l.slots <- struct{}{}
}

// Close shuts down the limiter, preventing further acquisitions and releases.
// It ensures that the slots channel is safely closed and marks the limiter as closed
// using the atomic flag `closed`.
//
// This method is idempotent and prevents race conditions using a mutex.
// Once closed, the limiter cannot be reused.
func (l *Limiter) Close() {
	l.closeMutex.Lock()
	defer l.closeMutex.Unlock()

	// Prevent close from being run twice
	if l.isClosed() {
		return
	}

	// Drain the slots channel of any remaining objects
	for range len(l.slots) {
		<-l.slots
	}

	l.closed.Store(true)
	close(l.slots)
}

//endregion

//region Helpers

// isClosed is a helper method that checks whether the limiter has been closed.
// Returns true if the atomic `closed` flag is set.
func (l *Limiter) isClosed() bool {
	return l.closed.Load() == true
}

//endregion

//region Constructor

// NewLimiter initializes a new Limiter instance with a specified number of slots.
// The size parameter defines the maximum concurrency level, controlling the number of
// slots that workers can acquire concurrently.
//
// Returns a fully initialized Limiter with the specified size.
func NewLimiter(size uint16) *Limiter {

	slotBuffer := make(chan struct{}, size)
	for i := uint16(0); i < size; i++ {
		slotBuffer <- struct{}{}
	}

	return &Limiter{
		slots:      slotBuffer,
		closed:     &atomic.Bool{},
		closeMutex: &sync.Mutex{},
	}
}

//endregion
