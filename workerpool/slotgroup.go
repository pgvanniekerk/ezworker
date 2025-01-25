package workerpool

// A SlotGroup ensures that a limited number of concurrent operations (or "slots") can
// be performed simultaneously, enforcing a strict maximum concurrency level.
// Implementations of this interface must be thread-safe and ensure proper management of resources.
type SlotGroup interface {

	// Acquire provides access to the internal slots of the group.
	// Workers can read from the channel to acquire a slot for executing tasks.
	// This method ensures that at most a predefined number of slots are available
	// for workers, limiting the number of concurrent tasks.
	//
	// Returns:
	//   <-chan struct{}: The channel that workers can listen on to acquire a slot.
	Acquire() <-chan struct{}

	// Release returns a slot back to the group, making it available for other workers.
	// This method ensures that slots are not permanently consumed and can be reused
	// by other tasks. If the group is already closed or if the slots buffer is full,
	// the release is safely ignored.
	Release()

	// Close shuts down the group and releases all resources.
	// Once closed:
	//   - The slots channel is closed to signal that no more slots can be acquired.
	//   - Any further calls to Acquire or Release are ignored.
	//   - This method ensures proper synchronization to prevent race conditions
	//     when closing the group.
	//
	// Note: This method is idempotent, meaning calling it multiple times has no effect
	// beyond the first call.
	Close()
}
