package concurrency

// Limiter represents a concurrency control mechanism that manages access to a limited pool of resources.
// It allows operations to acquire a "slot" for use, release the slot when done,
// and safely close the group to prevent further usage.
//
// Designed for high-concurrency scenarios, the Limiter implementation supports:
//   - Managing a fixed number of total slots (capacity).
//   - Efficient tracking of available and occupied slots.
//   - Graceful closing of the group with proper cleanup of resources.
//   - Timeout or error handling when slots are not available for acquisition.
//
// Methods:
//   - Acquire(): Acquire a slot for use, blocking if necessary, or returning an error if no slots are available.
//   - Release(): Release an acquired slot back to the group.
//   - Close(): Gracefully close the Limiter, ensuring no further usage and cleaning up resources.
//   - Resize(): Dynamically updates the capacity of the group (i.e., the number of workers).
//   - TotalSlots(): Return the total capacity of the group (number of slots).
//   - AvailableSlots(): Return the current count of available slots.
//   - OccupiedSlots(): Return the current count of occupied/used slots.
type Limiter interface {

	// Acquire attempts to acquire a slot from the group. If successful, it returns nil.
	// If no slots are available, it will either:
	//   - Block until a slot becomes available,
	//   - Return an error immediately if configured to do so, or
	//   - Timeout if a timeout period has been specified.
	//
	// Acquire is thread-safe and can be called concurrently.
	//
	// Returns:
	// - `nil`: If a slot was successfully acquired.
	// - `ErrNoSlotsAvailable`: If no slots are available (and immediate-error mode is enabled).
	// - `ErrAcquireTimeoutReached`: If acquisition timed out.
	// - `ErrGroupClosed`: If the group has been closed and no new slots can be acquired.
	Acquire() error

	// Release releases an acquired slot back to the group, making it available for others to acquire.
	// If the group is full (i.e., all slots are unoccupied), it returns an error.
	//
	// Release is thread-safe and can be called concurrently.
	//
	// Returns:
	// - `nil`: If the slot was successfully released.
	// - `ErrReleaseExceedsMaxLimit`: If a release is attempted when no slots are in use.
	// - `ErrGroupClosed`: If the group has been closed, and slots can no longer be released.
	Release() error

	// Close gracefully closes the group, ensuring no new slots can be acquired or released.
	// It cancels any pending Acquire calls, waits for all in-progress operations to complete,
	// and releases any resources held by the group.
	//
	// Close is thread-safe and can be called concurrently. Later calls to Close will return
	// an error after the group has already been closed.
	//
	// Returns:
	// - `nil`: If the group was successfully closed.
	// - `ErrGroupClosed`: If the group has already been closed.
	Close() error

	// Resize dynamically updates the capacity of the group (i.e., the number of slots).
	// This function allows increasing or decreasing the total number of slots available
	// for concurrent operations.
	//
	// When increasing the capacity, additional slots will become available for acquisition.
	// When decreasing the capacity, the new size must not be smaller than the number of
	// currently occupied slots. If the requested capacity is smaller than the occupied slots,
	// an error is returned.
	//
	// This method is thread-safe and can be called concurrently with other methods.
	// However, proper checks should ensure that resizing does not disrupt ongoing operations.
	//
	// Parameters:
	// - `size` (uint32): The new desired capacity of the group.
	//
	// Returns:
	// - `nil`: If resizing is successful.
	// - `ErrResizeInvalidCapacity`: If the requested size is zero, negative, or less than the number
	//   of currently occupied slots.
	// - `ErrGroupClosed`: If the group is closed and cannot accept a resize operation.
	Resize(size uint32) error

	// TotalSlots returns the total number of slots configured for the group.
	// This value represents the maximum concurrency allowed by the group.
	TotalSlots() uint32

	// AvailableSlots returns the current number of slots that are available for acquisition.
	// This value decreases as slots are acquired and increases as slots are released.
	AvailableSlots() uint32

	// OccupiedSlots returns the current number of slots that are in use.
	// This is equivalent to `TotalSlots - AvailableSlots`.
	OccupiedSlots() uint32
}
