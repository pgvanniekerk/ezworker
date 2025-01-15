package concurrency

import (
	"context"
	"errors"
	"github.com/pgvanniekerk/ezworker/pkg/concurrency"
	"github.com/stretchr/testify/suite"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWorkerPool_Acquire_TestSuite executes the test suite for the Acquire function
// of the Limiter type.
func TestWorkerPoolTestSuite(t *testing.T) {
	suite.Run(t, new(WorkerPoolTestSuite))
}

// WorkerPoolTestSuite tests the Limiter type and it's methods.
// It aims to cover all possible scenarios, with specific attention being paid to concurrent scenarios.
type WorkerPoolTestSuite struct {
	suite.Suite
}

// Test_Acquire_ErrorWhenClosed ensures that the concurrency.ErrLimiterClosed error
// is returned if Acquire is called on a closed limiter.
func (s *WorkerPoolTestSuite) Test_Acquire_ErrorWhenClosed() {

	closed := &atomic.Bool{}
	closed.Store(true)

	limiter := &Limiter{
		closed: closed,
	}

	err := limiter.Acquire()
	s.Require().Error(err)
	s.Require().True(errors.Is(err, concurrency.ErrLimiterClosed))
}

// Test_Acquire_ErrOnNoAvailableWorkers tests that the concurrency.ErrNoSlotsAvailable error
// is returned if the Limiter is configured to error when no slots are available and the
// availableSlots variable == 0
func (s *WorkerPoolTestSuite) Test_Acquire_ErrOnNoAvailableWorkers() {

	closed := &atomic.Bool{}
	closed.Store(false)

	slots := &atomic.Uint32{}
	slots.Store(0)

	limiter := &Limiter{
		closed:                closed,
		availableSlots:        slots,
		errOnNoSlotsAvailable: true,
	}

	err := limiter.Acquire()
	s.Require().Error(err)
	s.Require().True(errors.Is(err, concurrency.ErrNoSlotsAvailable))
}

// Test_Acquire_ReturnsTimeoutErrorWhenRequired tests that the Acquire function returns
// concurrency.ErrAcquireTimeoutReached error should the timeout be reached.
func (s *WorkerPoolTestSuite) Test_Acquire_ReturnsTimeoutErrorWhenRequired() {

	closed := &atomic.Bool{}
	closed.Store(false)

	availSlots := &atomic.Uint32{}
	availSlots.Store(0)

	busySlots := &atomic.Uint32{}
	busySlots.Store(1)

	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	limiter := &Limiter{
		closed:                 closed,
		availableSlots:         availSlots,
		errOnNoSlotsAvailable:  false,
		acquireTimeoutDuration: 2 * time.Second,
		concurrencyPool:        make(chan struct{}),
		closeWaitGroup:         &sync.WaitGroup{},
		closeCtx:               closeCtx,
		closeFunc:              closeFunc,
		totalSlots:             1,
		occupiedSlots:          busySlots,
		resizeCtx:              resizeCtx,
		resizeCancelFunc:       resizeFunc,
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	var err error
	go func(wg *sync.WaitGroup, l *Limiter) {
		defer wg.Done()
		err = l.Acquire()
	}(wg, limiter)

	wg.Wait()
	s.Require().Error(err)
	s.Require().True(
		errors.Is(err, concurrency.ErrAcquireTimeoutReached),
		"expected error to be concurrency.ErrAcquireTimeoutReached, got %v",
		err,
	)

}

// Test_Acquire_WaitsTillCancelIfNoAcquireTimeoutIsConfigured tests that the Acquire method
// will not throw a timeout error and wait until there is a slot available or the Limiter
// is closed.
func (s *WorkerPoolTestSuite) Test_Acquire_WaitsTillCancelIfNoAcquireTimeoutIsConfigured() {

	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(0)

	availableSlots := &atomic.Uint32{}
	availableSlots.Store(1)

	closeCtx, closeFunc := context.WithTimeout(context.Background(), time.Second)
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	limiter := &Limiter{
		closed:                closed,
		availableSlots:        availableSlots,
		errOnNoSlotsAvailable: false,
		concurrencyPool:       make(chan struct{}),
		closeCtx:              closeCtx,
		closeFunc:             closeFunc,
		closeWaitGroup:        &sync.WaitGroup{},
		occupiedSlots:         busySlots,
		resizeCtx:             resizeCtx,
		resizeCancelFunc:      resizeFunc,
	}

	// Invoke limiter.Close after 2 seconds asynchronously. This will allow testing that acquire
	// returns the correct error indicating closure.
	go func() {
		time.Sleep(2 * time.Second)
	}()

	// Try to acquire a slot, this should block until closeFunc is called
	err := limiter.Acquire()
	s.Require().Error(err)
	s.Require().True(errors.Is(err, concurrency.ErrLimiterClosed))
}

// Test_Acquire_WaitsTillWorkerAvailableBeforeReturningAndSetsStatistics tests that Acquire waits for a slot
// to become available on the channel before unblocking. This test assumes no timeout
// has been configured for Acquire nor the flag to raise an error when no slots are available.
func (s *WorkerPoolTestSuite) Test_Acquire_WaitsTillWorkerAvailableBeforeReturningAndSetsStatistics() {

	//
	//	Set up the Limiter state so that it seems like it has a slot available. The actual
	//  slot will be inserted later on in the test to ensure that the running thread
	//  blocks on the call to Acquire until a message is written into the channel.
	//

	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(0)

	availableSlots := &atomic.Uint32{}
	availableSlots.Store(1)

	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	limiter := &Limiter{
		closed:                closed,
		availableSlots:        availableSlots,
		errOnNoSlotsAvailable: false,
		concurrencyPool:       make(chan struct{}),
		closeCtx:              closeCtx,
		closeFunc:             closeFunc,
		closeWaitGroup:        &sync.WaitGroup{},
		occupiedSlots:         busySlots,
		resizeCtx:             resizeCtx,
		resizeCancelFunc:      resizeFunc,
	}

	//
	// Asynchronously sleep and add a message into the channel.
	// This allows the current thread to block on Acquire until the message has been inserted
	// and the time acquired logged.
	//
	beforeTime := time.Now()
	go func() {
		time.Sleep(1 * time.Second)
		limiter.concurrencyPool <- struct{}{}
	}()

	// Track the "before/After" timestamps and ensure they are greater than 1 second apart.
	err := limiter.Acquire()
	afterTime := time.Now()

	// Expecting no error as it was a normal wait scenario.
	s.Require().NoError(err)

	// Ensure the wait time showcases the time waited accounting for the time that the go-routine spent sleeping.
	s.Require().True(afterTime.UnixMilli()-beforeTime.UnixMilli() >= time.Second.Milliseconds())

	// Ensure that statistics are updated to reflect the acquired resource.
	s.Require().Equal(uint32(1), limiter.occupiedSlots.Load())
	s.Require().Equal(uint32(0), limiter.availableSlots.Load())

}

// Test_Release_ReturnsErrorWhenClosed ensures that the concurrency.ErrLimiterClosed error
// is returned should the caller try to call Release after the Limiter has been closed.
func (s *WorkerPoolTestSuite) Test_Release_ReturnsErrorWhenClosed() {

	//
	// Set up the limiter to allow the Release function to result
	// in the required error when invoked.
	//

	closed := &atomic.Bool{}
	closed.Store(true)

	limiter := &Limiter{
		closed: closed,
	}

	// Execute the tests
	err := limiter.Release()
	s.Require().Error(err)
	s.Require().True(errors.Is(err, concurrency.ErrLimiterClosed))

}

// Test_Release_ReturnsErrorWhenMaxWorkersReached ensures that the concurrency.ErrReleaseExceedsMaxLimit
// error is returned should Release be called more times than the slot size of the Limiter.
func (s *WorkerPoolTestSuite) Test_Release_ReturnsErrorWhenMaxWorkersReached() {

	//
	//	Set up the Limiter state so that it seems like it has a slot available. The actual
	//  slot will be inserted later on in the test to ensure that the running thread
	//  blocks on the call to Acquire until a message is written into the channel.
	//

	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(0)

	limiter := &Limiter{
		closed:        closed,
		occupiedSlots: busySlots,
	}

	// Execute the tests
	err := limiter.Release()
	s.Require().Error(err)
	s.Require().True(errors.Is(err, concurrency.ErrReleaseExceedsMaxLimit))

}

// Test_Release_AddsWorkerBackIntoLimiterAndAllowsAcquireToUnblock tests that calling
// Release unblocks a blocking call the Acquire.
func (s *WorkerPoolTestSuite) Test_Release_AddsWorkerBackIntoLimiterAndAllowsAcquireToUnblock() {

	//
	//	Set up the Limiter state so that it seems like it has a slot available. The actual
	//  slot will be inserted later on in the test to ensure that the running thread
	//  blocks on the call to Acquire until a message is written into the channel.
	//

	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(1)

	availableSlots := &atomic.Uint32{}
	availableSlots.Store(0)

	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	limiter := &Limiter{
		closed:                closed,
		availableSlots:        availableSlots,
		errOnNoSlotsAvailable: false,
		concurrencyPool:       make(chan struct{}, 1),
		closeCtx:              closeCtx,
		closeFunc:             closeFunc,
		closeWaitGroup:        &sync.WaitGroup{},
		occupiedSlots:         busySlots,
		totalSlots:            1,
		resizeCtx:             resizeCtx,
		resizeCancelFunc:      resizeFunc,
	}

	//
	// Asynchronously sleep and add a message into the channel.
	//
	beforeTime := time.Now()
	go func() {
		time.Sleep(1 * time.Second)
		err := limiter.Release()
		s.Require().NoError(err)
	}()

	<-limiter.concurrencyPool
	afterTime := time.Now()

	// Ensure the wait time showcases the time waited accounting for the time that the go-routine spent sleeping.
	s.Require().True(afterTime.UnixMilli()-beforeTime.UnixMilli() >= time.Second.Milliseconds(), "expected wait time to be greater than 1 second, got %d", afterTime.UnixMilli()-beforeTime.UnixMilli())

	// Ensure that statistics are updated to reflect the acquired resource.
	s.Require().Equal(uint32(0), limiter.occupiedSlots.Load())
	s.Require().Equal(uint32(1), limiter.availableSlots.Load())

}

// Test_Close_ErrorWhenAlreadyClosed tests that the concurrency.ErrLimiterClosed error is thrown when the Limiter
// has already been closed.
func (s *WorkerPoolTestSuite) Test_Close_ErrorWhenAlreadyClosed() {

	closed := &atomic.Bool{}
	closed.Store(true)

	wrkrGrp := &Limiter{
		closed: closed,
	}

	err := wrkrGrp.Close()
	s.Require().Error(err)
	s.Require().True(errors.Is(err, concurrency.ErrLimiterClosed))

}

// Test_Close_TriggersErrWorkerGroupClosedWhileBlockingWithAcquire tests that the concurrency.ErrLimiterClosed
// error is returned if a thread is currently waiting for a worker and Close is called.
func (s *WorkerPoolTestSuite) Test_Close_TriggersErrWorkerGroupClosedWhileBlockingWithAcquire() {

	//
	//	Set up the Limiter state so that it seems like it has a slot available. The actual
	//  slot will be inserted later on in the test to ensure that the running thread
	//  blocks on the call to Acquire until a message is written into the channel.
	//

	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(1)

	availableSlots := &atomic.Uint32{}
	availableSlots.Store(0)

	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	l := &Limiter{
		closed:                closed,
		availableSlots:        availableSlots,
		errOnNoSlotsAvailable: false,
		concurrencyPool:       make(chan struct{}, 1),
		closeCtx:              closeCtx,
		closeFunc:             closeFunc,
		closeWaitGroup:        &sync.WaitGroup{},
		occupiedSlots:         busySlots,
		totalSlots:            1,
		resizeWaitGroup:       &sync.WaitGroup{},
		resizeCtx:             resizeCtx,
		resizeCancelFunc:      resizeFunc,
	}

	wg := &sync.WaitGroup{}

	// Asynchronously Close the Limiter after a set amount of time.
	startTime := time.Now()
	closeTime := time.Now()
	wg.Add(1)
	go func(closeTime *time.Time, wg *sync.WaitGroup) {
		time.Sleep(2 * time.Second)
		err := l.Close()
		*closeTime = time.Now()
		s.Require().NoError(err)
		wg.Done()
	}(&closeTime, wg)

	// Try to Acquire a worker. Since there are none available, it should
	// block until l.Close is called.
	err := l.Acquire()
	endTime := time.Now()
	wg.Wait()
	s.Require().Error(err)
	s.Require().True(errors.Is(err, concurrency.ErrLimiterClosed))
	s.Require().True(endTime.UnixMilli()-startTime.UnixMilli() >= 2*time.Second.Milliseconds())
	s.Require().True(closeTime.UnixMilli() == endTime.UnixMilli(), "expected closeTime to be equal to endTime, got closeTime[%d] and endTime[%d]", closeTime.UnixMilli(), endTime.UnixMilli())
}

// Test_Resize_ErrOnClosedLimiter tests that the call returns the concurrency.ErrLimiterClosed
// error when called after Close has been called on the limiter.
func (s *WorkerPoolTestSuite) Test_Resize_ErrOnClosedLimiter() {

	closed := &atomic.Bool{}
	closed.Store(true)

	limiter := &Limiter{
		closed: closed,
	}

	err := limiter.Resize(10)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, concurrency.ErrLimiterClosed))
}

// Test_Resize_WaitForRunningResizeToFinaliseBeforeContinuing tests that multiple asynchronous
// calls to Resize will block until a running instance has been executed.
func (s *WorkerPoolTestSuite) Test_Resize_WaitForRunningResizeToFinaliseBeforeContinuing() {

	//
	// Arrange
	//

	// Set up the Limiter state.
	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(1)

	availableSlots := &atomic.Uint32{}
	availableSlots.Store(0)

	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	l := &Limiter{
		closed:                closed,
		availableSlots:        availableSlots,
		errOnNoSlotsAvailable: false,
		concurrencyPool:       make(chan struct{}, 1),
		closeCtx:              closeCtx,
		closeFunc:             closeFunc,
		closeWaitGroup:        &sync.WaitGroup{},
		occupiedSlots:         busySlots,
		totalSlots:            1,
		resizeCtx:             resizeCtx,
		resizeCancelFunc:      resizeFunc,
		resizeWaitGroup:       &sync.WaitGroup{},
	}

	// Set up a wait group to wait for the go-routine to complete before testing/comparing times
	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Add a waiter to simulate a currently in-progress Resize operation
	l.resizeWaitGroup.Add(1)
	startTime := time.Now()
	resizeTime := time.Now()
	go func(wg *sync.WaitGroup, rt *time.Time) {
		err := l.Resize(10)
		s.Require().NoError(err)
		resizeTime = time.Now()
		wg.Done()
	}(wg, &resizeTime)

	// Wait for processing to complete.
	time.Sleep(2 * time.Second)
	l.resizeWaitGroup.Done()
	wg.Wait()
	endTime := time.Now()

	// Assert that (resizeTime - startTime >= (2 * time.Second)), indicating that the thread blocked on the call
	// to resize will until 'l.resizeWaitGroup.Done()' was called.
	s.Require().True(resizeTime.UnixMilli()-startTime.UnixMilli() >= 2*time.Second.Milliseconds())
	s.Require().True(resizeTime.UnixMilli() == endTime.UnixMilli(), "expected resizeTime to be equal to endTime, got resizeTime[%d] and endTime[%d]", resizeTime.UnixMilli(), endTime.UnixMilli())

}

// Test_Resize_callsResizeCancelFunc tests that threads listening for Limiter.resizeCtx.Done() will unblock
// when Resize is called.
func (s *WorkerPoolTestSuite) Test_Resize_callsResizeCancelFunc() {

	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(1)

	availableSlots := &atomic.Uint32{}
	availableSlots.Store(0)

	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	l := &Limiter{
		closed:                closed,
		availableSlots:        availableSlots,
		errOnNoSlotsAvailable: false,
		concurrencyPool:       make(chan struct{}, 1),
		closeCtx:              closeCtx,
		closeFunc:             closeFunc,
		closeWaitGroup:        &sync.WaitGroup{},
		occupiedSlots:         busySlots,
		totalSlots:            1,
		resizeCtx:             resizeCtx,
		resizeCancelFunc:      resizeFunc,
		resizeWaitGroup:       &sync.WaitGroup{},
	}

	// Asynchronously send the call to Resize after 2 seconds
	startTime := time.Now()
	go func() {
		time.Sleep(2 * time.Second)
		err := l.Resize(10)
		s.Require().NoError(err)
	}()

	// Wait for the Done signal triggered by the call to Resize
	<-resizeCtx.Done()
	endTime := time.Now()

	// The error should be Canceled per what l.Resize invokes
	s.Require().Equal(resizeCtx.Err(), context.Canceled)

	// Time greater than 2 seconds as the async routine sleeps for 2 seconds before calling resize
	s.Require().True(endTime.UnixMilli()-startTime.UnixMilli() >= 2*time.Second.Milliseconds())

}

// Test_Resize_setsTotalWorkersToSize tests that the internal variable tracking totalWorkers is correctly
// set after a successful call to Resize.
func (s *WorkerPoolTestSuite) Test_Resize_setsTotalWorkersToSize() {

	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(1)

	availableSlots := &atomic.Uint32{}
	availableSlots.Store(0)

	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	l := &Limiter{
		closed:                closed,
		availableSlots:        availableSlots,
		errOnNoSlotsAvailable: false,
		concurrencyPool:       make(chan struct{}, 1),
		closeCtx:              closeCtx,
		closeFunc:             closeFunc,
		closeWaitGroup:        &sync.WaitGroup{},
		occupiedSlots:         busySlots,
		totalSlots:            1,
		resizeCtx:             resizeCtx,
		resizeCancelFunc:      resizeFunc,
		resizeWaitGroup:       &sync.WaitGroup{},
	}

	err := l.Resize(10)
	s.Require().NoError(err)
	s.Require().Equal(uint32(10), l.TotalSlots())

}

// Test_Resize_recalculatesAvailableSlots tests that the internal variable for tracking available slots
// is updated accordingly and takes into account occupied slots vs. current slot availability vs. new size.
func (s *WorkerPoolTestSuite) Test_Resize_recalculatesAvailableSlots() {

	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(1)

	availableSlots := &atomic.Uint32{}
	availableSlots.Store(0)

	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	l := &Limiter{
		closed:                closed,
		availableSlots:        availableSlots,
		errOnNoSlotsAvailable: false,
		concurrencyPool:       make(chan struct{}, 1),
		closeCtx:              closeCtx,
		closeFunc:             closeFunc,
		closeWaitGroup:        &sync.WaitGroup{},
		occupiedSlots:         busySlots,
		totalSlots:            1,
		resizeCtx:             resizeCtx,
		resizeCancelFunc:      resizeFunc,
		resizeWaitGroup:       &sync.WaitGroup{},
	}

	err := l.Resize(10)
	s.Require().NoError(err)
	s.Require().Equal(uint32(9), l.AvailableSlots())

}

// Test_Resize_LimiterStatisticsAndAvailableSlotsUpdateWhenGrowingAndShrinking tests that the internal variable for
// tracking available slots is set to 0 if (new size number - the occupied number) <= 0, defaulting to 0 available
// workers, as we've reduced enough size to Acquire all slots. There should be no objects in the channel either.
func (s *WorkerPoolTestSuite) Test_Resize_LimiterStatisticsAndAvailableSlotsUpdateWhenGrowingAndShrinking() {

	closed := &atomic.Bool{}
	closed.Store(false)

	busySlots := &atomic.Uint32{}
	busySlots.Store(5)

	availableSlots := &atomic.Uint32{}
	availableSlots.Store(3)

	closeCtx, closeFunc := context.WithCancel(context.Background())
	resizeCtx, resizeFunc := context.WithCancel(context.Background())

	l := &Limiter{
		closed:                closed,
		availableSlots:        availableSlots,
		errOnNoSlotsAvailable: false,
		concurrencyPool:       make(chan struct{}, 8),
		closeCtx:              closeCtx,
		closeFunc:             closeFunc,
		closeWaitGroup:        &sync.WaitGroup{},
		occupiedSlots:         busySlots,
		totalSlots:            8,
		resizeCtx:             resizeCtx,
		resizeCancelFunc:      resizeFunc,
		resizeWaitGroup:       &sync.WaitGroup{},
	}

	for i := uint32(0); i < availableSlots.Load(); i++ {
		l.concurrencyPool <- struct{}{}
	}

	//
	// Shrink the total, expecting 0 avail and 5 occupied, 5 total
	//

	// Call Resize and assert update slot statistics.
	err := l.Resize(5)
	s.Require().NoError(err)
	s.Require().Equal(uint32(0), l.AvailableSlots())
	s.Require().Equal(uint32(5), l.OccupiedSlots())
	s.Require().Equal(uint32(5), l.TotalSlots())

	// Ensure there are as many available slots on the concurrencyPool channel.
	foundSlot := 0
	wg := &sync.WaitGroup{}
	wg.Add(1)

SlotFindingFor1:
	for {
		select {
		case <-l.concurrencyPool:
			foundSlot++
			continue
		case <-time.After(1 * time.Second):
			wg.Done()
			break SlotFindingFor1
		}
	}
	wg.Wait()
	s.Require().Equal(0, foundSlot)

	//
	// Increase the total to 10, expecting 5 occupied and 5 avail
	//

	// Call Resize and assert update slot statistics.
	err = l.Resize(10)
	s.Require().NoError(err)
	s.Require().Equal(uint32(5), l.AvailableSlots())
	s.Require().Equal(uint32(5), l.OccupiedSlots())
	s.Require().Equal(uint32(10), l.TotalSlots())

	// Ensure there are as many available slots on the concurrencyPool channel.
	foundSlot = 0
	wg = &sync.WaitGroup{}
	wg.Add(1)

SlotFindingFor2:
	for {
		select {
		case <-l.concurrencyPool:
			foundSlot++
			continue
		case <-time.After(1 * time.Second):
			wg.Done()
			break SlotFindingFor2
		}
	}
	s.Require().Equal(5, foundSlot)

	//
	//	Decrease the total to 8, expecting 5 occupied, 3 avail
	//

	// Call Resize and assert update slot statistics.
	err = l.Resize(8)
	s.Require().NoError(err)
	s.Require().Equal(uint32(3), l.AvailableSlots())
	s.Require().Equal(uint32(5), l.OccupiedSlots())
	s.Require().Equal(uint32(8), l.TotalSlots())

	// Ensure there are as many available slots on the concurrencyPool channel.
	foundSlot = 0
	wg = &sync.WaitGroup{}
	wg.Add(1)

SlotFindingFor3:
	for {
		select {
		case <-l.concurrencyPool:
			foundSlot++
			continue
		case <-time.After(1 * time.Second):
			wg.Done()
			break SlotFindingFor3
		}
	}
	s.Require().Equal(3, foundSlot)
}

// Test_TotalWorkers_returnsValidNumber tests that the method returns the current value
// persisted in the WorkGroup object.
func (s *WorkerPoolTestSuite) Test_TotalWorkers_returnsValidNumber() {

	l := &Limiter{
		totalSlots: 10,
	}
	s.Require().Equal(uint32(10), l.TotalSlots())
}

// Test_AvailableWorkers_returnsValidNumber tests that the method returns the current
// value persisted in the WorkGroup object.
func (s *WorkerPoolTestSuite) Test_AvailableWorkers_returnsValidNumber() {

	availableWorkers := &atomic.Uint32{}
	availableWorkers.Store(10)

	l := &Limiter{
		availableSlots: availableWorkers,
	}
	s.Require().Equal(uint32(10), l.AvailableSlots())
}

// Test_OccupiedWorkers_returnsValidNumber tests that the method returns the current
// // value persisted in the WorkGroup object.
func (s *WorkerPoolTestSuite) Test_OccupiedWorkers_returnsValidNumber() {

	busyWorkers := &atomic.Uint32{}
	busyWorkers.Store(10)

	l := &Limiter{
		occupiedSlots: busyWorkers,
	}
	s.Require().Equal(uint32(10), l.OccupiedSlots())
}

// TestNewTestSuite executes the test suite for the NewLimiter function
// of this package
func TestNewLimiterTestSuite(t *testing.T) {
	suite.Run(t, new(NewLimiterTestSuite))
}

// New_TestSuite tests the NewLimiter function of this package.
type NewLimiterTestSuite struct {
	suite.Suite
}

// Test cases for the NewLimiter function in the NewLimiterTestSuite
func (suite *NewLimiterTestSuite) TestNewLimiter_WithValidInputs() {
	// Arrange
	slots := uint32(5)
	acquireTimeout := 2 * time.Second
	errOnNoSlotsAvailable := false

	// Act
	limiter, err := NewLimiter(slots, acquireTimeout, errOnNoSlotsAvailable)

	// Assert
	suite.Require().NoError(err)
	suite.NotNil(limiter)
	suite.Equal(slots, limiter.totalSlots)
	suite.Equal(slots, limiter.availableSlots.Load())
	suite.Equal(uint32(0), limiter.occupiedSlots.Load())
	suite.False(limiter.closed.Load())
	suite.Equal(acquireTimeout, limiter.acquireTimeoutDuration)
	suite.False(limiter.errOnNoSlotsAvailable)
	suite.NotNil(limiter.concurrencyPool)
	suite.Equal(int(slots), len(limiter.concurrencyPool))
	suite.NotNil(limiter.closeCtx)
	suite.NotNil(limiter.closeFunc)
	suite.NotNil(limiter.closeWaitGroup)
}

func (suite *NewLimiterTestSuite) TestNewLimiter_WithZeroSlots_ShouldError() {
	// Arrange
	slots := uint32(0)
	acquireTimeout := 2 * time.Second
	errOnNoSlotsAvailable := true

	// Act
	limiter, err := NewLimiter(slots, acquireTimeout, errOnNoSlotsAvailable)

	// Assert
	suite.Require().Error(err)
	suite.Nil(limiter)
	suite.EqualError(err, "cannot create a Limiter with 0 slots")
}

func (suite *NewLimiterTestSuite) TestNewLimiter_WithNilTimeout_ShouldSetZeroTimeout() {
	// Arrange
	slots := uint32(3)
	acquireTimeout := time.Duration(0)
	errOnNoSlotsAvailable := true

	// Act
	limiter, err := NewLimiter(slots, acquireTimeout, errOnNoSlotsAvailable)

	// Assert
	suite.Require().NoError(err)
	suite.NotNil(limiter)
	suite.Equal(time.Duration(0), limiter.acquireTimeoutDuration)
	suite.True(limiter.errOnNoSlotsAvailable)
}

func (suite *NewLimiterTestSuite) TestNewLimiter_WithContextInitialization() {
	// Arrange
	slots := uint32(4)
	acquireTimeout := 1 * time.Second
	errOnNoSlotsAvailable := false

	// Act
	limiter, err := NewLimiter(slots, acquireTimeout, errOnNoSlotsAvailable)

	// Assert
	suite.Require().NoError(err)
	suite.NotNil(limiter)
	select {
	case <-limiter.closeCtx.Done():
		suite.Fail("closeCtx should not be done")
	default:
		// Valid case; closeCtx should not yet be canceled
	}
}

// Test cases for concurrencyPool initialization
func (suite *NewLimiterTestSuite) TestNewLimiter_ConcurrencyPoolInitialization() {
	// Arrange
	slots := uint32(6)
	acquireTimeout := 3 * time.Second
	errOnNoSlotsAvailable := false

	// Act
	limiter, err := NewLimiter(slots, acquireTimeout, errOnNoSlotsAvailable)

	// Assert
	suite.Require().NoError(err)
	suite.NotNil(limiter)
	suite.Equal(int(slots), len(limiter.concurrencyPool))
	for i := 0; i < int(slots); i++ {
		select {
		case <-limiter.concurrencyPool:
			// Resource successfully retrieved
		default:
			suite.Fail("concurrencyPool should be fully initialized on creation")
		}
	}
}
