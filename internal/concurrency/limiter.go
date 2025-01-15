package concurrency

import (
	"context"
	"errors"
	"github.com/pgvanniekerk/ezworker/pkg/concurrency"
	"sync"
	"sync/atomic"
	"time"
)

type Limiter struct {

	//
	// Core Pool
	//

	// concurrencyPool is the buffered channel used to hold the available slots.
	// Each slot is represented by an empty struct.
	// The channel enforces a limit on the number of concurrent slots in the pool.
	concurrencyPool chan struct{}

	// totalSlots specifies the total number of slots in the pool.
	totalSlots uint32

	// availableSlots is an atomic counter that tracks how many slots are currently available in the pool.
	availableSlots *atomic.Uint32

	// occupiedSlots is an atomic counter that tracks how many slots are currently in use.
	occupiedSlots *atomic.Uint32

	//
	// Resize Control
	//

	// resizeCtx is a context used to signal to blocking Acquire/Release that the pool is about to be resized.
	resizeCtx context.Context

	// resizeCancelFunc is used to signal resizeCtx that it's time to resize.
	resizeCancelFunc context.CancelFunc

	// resizeWaitGroup allows Acquire/Release to wait for any resize operations to complete
	// before proceeding.
	resizeWaitGroup *sync.WaitGroup

	//
	// Acquire Timeout Handling
	//

	// acquireTimeoutDuration specifies the duration that an Acquire operation is allowed to wait before timing out.
	// If set to 0, there is no timeout.
	acquireTimeoutDuration time.Duration

	// errOnNoSlotsAvailable indicates whether Acquire should immediately return an error when no slots
	// are available, rather than waiting for one to become available.
	errOnNoSlotsAvailable bool

	//
	// Closure and Release Management
	//

	// closeCtx is a context used to signal that the Limiter is closing. Ongoing operations will be canceled.
	closeCtx context.Context

	// closeFunc is the function used to cancel the closeCtx. It signals to all listeners that the pool is closing.
	closeFunc context.CancelFunc

	// closed is a flag that indicates whether the Limiter has been closed. It prevents any further usage
	// of the pool once it is closed.
	closed *atomic.Bool

	// closeWaitGroup tracks active Acquire and Release operations, allowing the Close method to wait until
	// all ongoing operations finish before closing the pool.
	closeWaitGroup *sync.WaitGroup
}

//region Implementation

// Acquire attempts to acquire a slot from the Limiter. If successful, it reserves a slot and updates
// the available and busy slot counters. This operation blocks until one of the following occurs:
//   - A slot becomes available and is acquired successfully.
//   - The acquire timeout duration (if set) is reached.
//   - The Limiter is closed.
//
// Behavior:
//   - If `errOnNoSlotsAvailable` is true and no slots are currently available, Acquire fails immediately with an error.
//   - If an `acquireTimeout` is specified, the operation will wait up to that duration for a slot to become available.
//
// Thread-safety:
//   - This method is thread-safe, but it respects ongoing resize operations and pauses execution until resizing is completed.
//
// Returns:
//   - `nil`: If a slot is successfully acquired.
//   - `ErrNoSlotsAvailable`: If no slots are available and `errOnNoSlotsAvailable` is true.
//   - `ErrAcquireTimeoutReached`: If the acquire timeout is reached without acquiring a slot.
//   - `ErrLimiterClosed`: If the Limiter has been closed.
//
// Example:
//
//	err := wg.Acquire()
//	if err != nil {
//	    // handle error
//	}
func (l *Limiter) Acquire() error {

	// Ensure the flow of execution does not continue if the Limiter has already
	// been closed.
	if l.closed.Load() {
		return concurrency.ErrLimiterClosed
	}

	// If there are no slots available and the Limiter is set to "error when none
	// are available", return concurrency.ErrNoSlotsAvailable as directed by the option.
	if l.availableSlots.Load() == 0 && l.errOnNoSlotsAvailable {
		return concurrency.ErrNoSlotsAvailable
	}

	// Retrieve the applicable timeout context based on the value of l.acquireTimeoutDuration.
	// This will determine whether to ignore the timeout or to apply one as directed
	// by acquireTimeoutDuration.
	timeoutCtx, timeoutCancelFunc := l.getTimeoutContext()

	// The cancel func should always be called.
	// We don't need to make use of it, so defer it to run after everything else.
	defer timeoutCancelFunc()

	// Add 1 to the closeWaitGroup to synchronize Close() to wait for all running threads to finish.
	// Defer Done() to run at the end of this method to allow the Close method to continue freeing resources.
	l.closeWaitGroup.Add(1)
	defer l.closeWaitGroup.Done()

	// Continuously loop to wait for either an available slot, the potential acquireTimeoutDuration timeout,
	// or the Limiter close signal.
	for {
		select {

		// If resize is called, wait for resizing to happen, then try to call Acquire again
		case <-l.resizeCtx.Done():
			l.resizeWaitGroup.Wait()
			return l.Acquire()

		// Retrieve an available slot from the pool and update the usage statistics atomically.
		case <-l.concurrencyPool:

			// Subtract 1 from the availableSlots counter as the slot is being acquired.
			// `^uint32(0)` is the bitwise NOT of `0`, which gives the maximum unsigned integer for
			// 32-bit: `0xFFFFFFFF`, which is equivalent to `-1` when used in the context of addition.
			l.availableSlots.Add(^uint32(0))

			// Add 1 to occupiedSlots as it is being acquired.
			l.occupiedSlots.Add(1)

			// Return nil, indicating that a slot has been successfully acquired.
			return nil

		// Return the applicable error if l.acquireTimeoutDuration was none zero and the time has been reached.
		case <-timeoutCtx.Done():
			return concurrency.ErrAcquireTimeoutReached

		// Return the applicable error if the close signal was retrieved.
		case <-l.closeCtx.Done():
			return concurrency.ErrLimiterClosed
		}
	}
}

// Release releases a slot back to the Limiter, making it available for others to acquire. It updates
// the available and busy slot counters and unblocks any operations waiting in Acquire.
//
// Behavior:
// - If the Limiter has already been closed, Release fails with an error.
// - If there are no occupied slots, indicating an excess release attempt, Release fails with an error.
//
// Thread-safety:
// - This method is thread-safe, but it respects ongoing resize operations and pauses execution until resizing is completed.
//
// Returns:
// - `nil`: If the slot is successfully released.
// - `ErrReleaseExceedsMaxLimit`: If the number of occupied slots is 0 at the time of release.
// - `ErrLimiterClosed`: If the Limiter has been closed.
//
// Example:
//
//	err := wg.Release()
//	if err != nil {
//	    // handle error
//	}
func (l *Limiter) Release() error {

	// Ensure the flow of execution does not continue if the Limiter has already been closed.
	if l.closed.Load() {
		return concurrency.ErrLimiterClosed
	}

	// If the pool is full, it will block if we try to add to it.
	// Return an error notifying the caller of the fact.
	if l.occupiedSlots.Load() == 0 {
		return concurrency.ErrReleaseExceedsMaxLimit
	}

	// Add 1 to the closeWaitGroup to synchronize channel closure or l.concurrencyPool.
	// Defer Done() to run at the end of this method to allow the Close method
	// to continue freeing resources.
	l.closeWaitGroup.Add(1)
	defer l.closeWaitGroup.Done()

	// Continuously loop to wait for an opening in the buffer to write the slot to, or for the resize
	// instruction.
	for {
		select {

		// If resize is called, wait for resizing to happen, then try to call Acquire again.
		case <-l.resizeCtx.Done():
			l.resizeWaitGroup.Wait()
			return l.Release()

		// Try to send the slot into the channel.
		case l.concurrencyPool <- struct{}{}:

			// Subtract 1 from the occupiedSlots counter as the slot is being released.
			// `^uint32(0)` is the bitwise NOT of `0`, which gives the maximum unsigned integer for
			// 32-bit: `0xFFFFFFFF`, which is equivalent to `-1` when used in the context of addition.
			l.occupiedSlots.Add(^uint32(0))

			// Add 1 to the available slots counter as the slot is being released.
			l.availableSlots.Add(1)

			return nil
		}
	}
}

// Close gracefully shuts down the Limiter, ensuring no further operations can be performed on it.
// It cancels any queued or ongoing Acquire calls, waits for active operations to complete, and then
// releases all resources.
//
// Behavior:
// - After Close is called, further calls to Acquire, Release, or Resize will return an error.
// - Ongoing Acquire operations blocked on slot availability or timeout are canceled.
// - Waits for all active slots to complete before cleaning up resources, ensuring a safe shutdown.
//
// Thread-safety:
// - This method is thread-safe and can be called concurrently.
//
// Returns:
// - `nil`: If the Limiter is successfully closed.
// - `ErrLimiterClosed`: If the Limiter has already been closed.
//
// Example:
//
//	err := wg.Close()
//	if err != nil {
//	    // handle error
//	}
func (l *Limiter) Close() error {

	// Ensure the flow of execution does not continue if the Limiter has already been closed.
	if l.closed.Load() {
		return concurrency.ErrLimiterClosed
	}
	l.closed.Store(true) // Set l.closed to true to prevent further calls to Acquire and Release.

	// Signal the close context to safely terminate any running Acquire method invocations.
	l.closeFunc()

	// Wait for any resizing to finishing
	l.resizeWaitGroup.Wait()

	// Wait for all running Acquire and Release method calls to finish before starting to release resources.
	l.closeWaitGroup.Wait()

	// Release/Close references
	l.closeCtx = nil
	l.closeFunc = nil
	l.closeWaitGroup = nil
	l.resizeCancelFunc()
	l.resizeCtx = nil
	close(l.concurrencyPool)
	l.concurrencyPool = nil
	return nil
}

// Resize dynamically adjusts the total number of slots in the Limiter. It modifies the total capacity
// (`concurrencyPool` and `totalSlots`) to the specified new size.
//
// Behavior:
//   - If the new size is larger than the current capacity, additional slots are added to the pool.
//   - If the new size is smaller than the current capacity, the number of available slots is reduced. However, the method ensures
//     that the number of active (busy) slots does not exceed the new capacity. If there are more active slots than the
//     new pool size, they are capped to the new size.
//
// Thread-safety:
// - This method is thread-safe. It temporarily pauses Acquire and Release operations during execution to avoid conflicts.
//
// Parameters:
// - `size` (uint32): The new desired capacity of the group.
//
// Returns:
// - `nil`: If resizing is successful.
// - `ErrGroupClosed`: If the Limiter has already been closed.
//
// Example:
//
//	err := wg.Resize(15)
//	if err != nil {
//	    // handle error
//	}
func (l *Limiter) Resize(size uint32) error {

	// Ensure the flow of execution does not continue if the Limiter has already been closed.
	if l.closed.Load() {
		return concurrency.ErrLimiterClosed
	}

	// Wait for any potentially running resizes to complete before continuing by waiting on resizeWaitGroup.
	// If no Resize calls are in progress, Wait() will block, otherwise continue.
	l.resizeWaitGroup.Wait()

	// Add a waiter to resizeWaitGroup to pause any Acquire/Release operations until resizing has been done.
	l.resizeWaitGroup.Add(1)
	defer l.resizeWaitGroup.Done()

	// Signal to running go-routines that a resize is taking place.
	l.resizeCancelFunc()

	// Update the totalSlots value to the provided new size.
	l.totalSlots = size

	// Store the updated available slots.
	if int(size-l.occupiedSlots.Load()) <= 0 {
		l.availableSlots.Store(0)
	} else {
		l.availableSlots.Store(size - l.occupiedSlots.Load())
	}

	// Store the updated busy slots.
	if l.occupiedSlots.Load() > size {
		l.occupiedSlots.Store(size)
	}

	// Populate the new concurrencyPool with the right number of available slots.
	// This will allow Acquire to unblock with available slots
	availSlots := l.availableSlots.Load()
	l.concurrencyPool = make(chan struct{}, size)
	for i := uint32(0); i < availSlots; i++ {
		l.concurrencyPool <- struct{}{}
	}

	// Create a new resize context for the potential next call. Note: This is done last to ensure we don't
	// update l.resizeCtx before channels waiting on the Cancel signal have been triggered.
	l.resizeCtx, l.resizeCancelFunc = context.WithCancel(context.Background())

	return nil
}

// TotalSlots returns the total number of slots in the Limiter.
// This represents the maximum allowed concurrency level.
//
// Thread-safety:
// This method is thread-safe and can be called concurrently with other operations.
//
// Returns:
// - `uint32`: The total number of slots in the group.
//
// Example:
//
//	total := wg.TotalSlots()
//	fmt.Println("Total Slots:", total)
func (l *Limiter) TotalSlots() uint32 {
	return l.totalSlots
}

// AvailableSlots returns the number of currently available slots in the Limiter.
// This reflects how many slots can be acquired at the current moment.
//
// Thread-safety:
// This method is thread-safe and can be called concurrently with other operations.
//
// Returns:
// - `uint32`: The number of available slots.
//
// Example:
//
//	available := wg.AvailableSlots()
//	fmt.Println("Available Slots:", available)
func (l *Limiter) AvailableSlots() uint32 {
	return l.availableSlots.Load()
}

// OccupiedSlots returns the number of currently busy slots in the Limiter.
// This reflects how many slots are currently acquired and in use.
//
// Thread-safety:
// This method is thread-safe and can be called concurrently with other operations.
//
// Returns:
// - `uint32`: The number of occupied slots.
//
// Example:
//
//	occupied := wg.OccupiedSlots()
//	fmt.Println("Occupied Slots:", occupied)
func (l *Limiter) OccupiedSlots() uint32 {
	return l.occupiedSlots.Load()
}

//endregion

//region Helpers

// getTimeoutContext creates and returns a context with or without a timeout based on the
// acquire timeout duration (`acquireTimeoutDuration`) defined in the Limiter.
//
// Behavior:
// - If `acquireTimeoutDuration` is greater than 0, a context with the specified timeout is returned.
// - If `acquireTimeoutDuration` is 0, a cancellable context with no timeout is returned.
//
// Thread-safety:
// This private method is thread-safe and ensures proper context creation for Acquire operations.
//
// Returns:
// - `context.Context`: The created context (with or without a timeout).
// - `context.CancelFunc`: A function to cancel the context.
//
// Example (used internally by Acquire):
//
//	ctx, cancel := wg.getTimeoutContext()
//	defer cancel()
//	<-ctx.Done() // handle timeout or cancellation
func (l *Limiter) getTimeoutContext() (context.Context, context.CancelFunc) {

	if l.acquireTimeoutDuration > time.Duration(0) {
		return context.WithTimeout(context.Background(), l.acquireTimeoutDuration)
	}
	return context.WithCancel(context.Background())
}

//endregion

//region Constructor

// NewLimiter creates and initializes a new Limiter with the specified number of slots (total slots),
// acquire timeout duration, and an option to immediately return an error if no slots are available.
//
// Parameters:
//   - `slots` (uint32): The total number of slots in the group. Must be greater than 0.
//   - `acquireTimeout` (time.Duration): The maximum time allowed to wait for a slot to become available in Acquire.
//     If set to 0, there is no timeout.
//   - `errOnNoSlotsAvailable` (bool): If true, an Acquire call will immediately fail with an error when no slots
//     are available, instead of waiting.
//
// Returns:
// - `*Limiter`: The initialized Limiter structure.
// - `error`: An error if the initialization fails (e.g., `slots` is 0).
//
// Behavior:
// - Initializes a slot pool to manage concurrency, with a specified number of slots.
// - Tracks available and busy slots using atomic counters for thread safety.
// - Creates synchronization mechanisms to gracefully handle group closure or resizing.
//
// Example:
//
//	wg, err := NewLimiter(10, time.Second*5, true)
//	if err != nil {
//	    // handle error
//	}
func NewLimiter(slots uint32, acquireTimeout time.Duration, errOnNoSlotsAvailable bool) (*Limiter, error) {

	if slots == 0 {
		return nil, errors.New("cannot create a Limiter with 0 slots")
	}

	// Close and Resize Contexts for synchronization.
	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	// Atomic uint32 to store the current number of available slots.
	availableSlots := &atomic.Uint32{}
	availableSlots.Store(slots)

	// Atomic uint32 to store the current number of busy slots.
	busySlots := &atomic.Uint32{}
	busySlots.Store(0)

	// Atomic Boolean flag to prevent calling of a closed Limiter
	closed := &atomic.Bool{}
	closed.Store(false)

	// Main channel that will be used to limit concurrent slots
	concurrencyPool := make(chan struct{}, slots)
	for i := uint32(0); i < slots; i++ {
		concurrencyPool <- struct{}{}
	}

	limiter := &Limiter{
		concurrencyPool:        concurrencyPool,
		totalSlots:             slots,
		availableSlots:         availableSlots,
		occupiedSlots:          busySlots,
		acquireTimeoutDuration: acquireTimeout,
		errOnNoSlotsAvailable:  errOnNoSlotsAvailable,
		closeCtx:               closeCtx,
		closeFunc:              closeFunc,
		closed:                 closed,
		closeWaitGroup:         &sync.WaitGroup{},
		resizeCtx:              resizeCtx,
		resizeCancelFunc:       resizeFunc,
		resizeWaitGroup:        &sync.WaitGroup{},
	}

	return limiter, nil
}

//endregion
