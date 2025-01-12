package workerpool

import (
	"github.com/stretchr/testify/suite"
	"sync"
	"testing"
	"time"
)

// TestWorkerPool_Acquire_TestSuite executes the test suite for the Acquire function
// of the WorkerPool type.
func TestWorkerPool_Acquire_TestSuite(t *testing.T) {
	suite.Run(t, new(WorkerPool_Acquire_TestSuite))
}

// WorkerPool_Acquire_TestSuite tests the Acquire function of the WorkerPool type.
type WorkerPool_Acquire_TestSuite struct {
	suite.Suite

	wp *WorkerPool
}

// SetupTest initializes a new instance of the WorkerPool type with initialized pool
func (w *WorkerPool_Acquire_TestSuite) SetupTest() {

	mu := &sync.Mutex{}
	w.wp = &WorkerPool{
		mu:        mu,
		poolSize:  1,
		poolAvail: 0,
		cond:      sync.NewCond(mu),
	}
}

// TearDownTest closes the buffered channel(pool) and sets the wp
// pointer to nil to be garbage collected.
func (w *WorkerPool_Acquire_TestSuite) TearDownTest() {
	w.wp = nil
}

// TestWorkerPool_Acquire_PanicOnClosedWorkGroup tests that the Acquire function causes
// a panic if it is invoked while the "closed" variable is equal to true.
func (w *WorkerPool_Acquire_TestSuite) TestWorkerPool_Acquire_PanicOnClosedWorkGroup() {
	w.wp.closed = true
	defer func() {
		r := recover()
		w.Require().NotNil(r)
		w.Require().Equal("cannot call Acquire on a closed WorkerPool", r.(error).Error())
	}()
	w.wp.Acquire()
}

// TestWorkerPool_Acquire_BlocksUntilWorkerAvailBeforeUnblocking tests that the Acquire function
// blocks until a worker(struct{}) is put into the channel before unblocking
func (w *WorkerPool_Acquire_TestSuite) TestWorkerPool_Acquire_BlocksUntilWorkerAvailBeforeUnblocking() {

	var availTime time.Time
	var unblockTime time.Time

	go func() {
		time.Sleep(1 * time.Second)
		availTime = time.Now()
		w.wp.poolAvail++
		w.wp.cond.Signal()
	}()

	w.wp.Acquire()
	unblockTime = time.Now()

	w.Require().True(availTime.Before(unblockTime))
}

// TestWorkerPool_Release_TestSuite executes the test suite for the Release function
// of the WorkerPool type.
func TestWorkerPool_Release_TestSuite(t *testing.T) {
	suite.Run(t, new(WorkerPool_Release_TestSuite))
}

// WorkerPool_Release_TestSuite tests the Acquire function of the WorkerPool type.
type WorkerPool_Release_TestSuite struct {
	suite.Suite

	wp *WorkerPool
}

// SetupTest initializes a new instance of the WorkerPool type with a buffered
// channel(pool) with a buffer size of 1.
func (w *WorkerPool_Release_TestSuite) SetupTest() {

	mu := &sync.Mutex{}
	w.wp = &WorkerPool{
		mu:        mu,
		poolSize:  1,
		poolAvail: 0,
		cond:      sync.NewCond(mu),
	}
}

// TearDownTest closes the buffered channel(pool) and sets the wp
// pointer to nil to be garbage collected.
func (w *WorkerPool_Release_TestSuite) TearDownTest() {
	w.wp = nil
}

// TestWorkerPool_Release_PanicOnClosedWorkGroup tests that the Release function causes
// a panic if it is invoked while the "closed" variable is equal to true.
func (w *WorkerPool_Release_TestSuite) TestWorkerPool_Release_PanicOnClosedWorkGroup() {
	w.wp.closed = true
	defer func() {
		r := recover()
		w.Require().NotNil(r)
		w.Require().Equal("cannot call Release on a closed WorkerPool", r.(error).Error())
	}()
	w.wp.Release()
}

// TestWorkerPool_Release_UnblocksReadingFromChannel tests that the Release function
// unblocks a reading from the WorkerPool.pool channel.
func (w *WorkerPool_Release_TestSuite) TestWorkerPool_Release_UnblocksReadingFromChannel() {

	var releaseTime time.Time
	var unblockTime time.Time

	go func() {
		time.Sleep(1 * time.Second)
		releaseTime = time.Now()
		w.wp.Release()
	}()

	w.wp.mu.Lock()
	w.wp.cond.Wait()
	unblockTime = time.Now()

	w.Require().True(releaseTime.Before(unblockTime))
}

// TestWorkerPool_Release_PanicsOnTooManyReleaseCalls tests that the Release function
// panics if Release is called more times than the size of the pool allows.
func (w *WorkerPool_Release_TestSuite) TestWorkerPool_Release_PanicsOnTooManyReleaseCalls() {
	w.wp.poolAvail = 1
	defer func() {
		r := recover()
		w.Require().NotNil(r)
		w.Require().Equal("cannot release more workers than are available", r.(error).Error())
	}()
	w.wp.Release()
}

// TestWorkerPool_Close_TestSuite executes the test suite for the expected usage
// of the WorkerPool type.
func TestWorkerPool_Close_TestSuite(t *testing.T) {
	suite.Run(t, new(WorkerPool_Close_TestSuite))
}

// WorkerPool_Close_TestSuite tests the expected usage of the WorkerPool type.
type WorkerPool_Close_TestSuite struct {
	suite.Suite

	wp *WorkerPool
}

// SetupTest instantiates a new WorkerPool
func (w *WorkerPool_Close_TestSuite) SetupTest() {

	mu := &sync.Mutex{}
	w.wp = &WorkerPool{
		mu:        mu,
		poolSize:  3,
		poolAvail: 0,
		cond:      sync.NewCond(mu),
	}
}

// TearDownTest closes the WorkerPool to release resources
// and sets it to nil to be garbage collected.
func (w *WorkerPool_Close_TestSuite) TearDownTest() {
	w.wp = nil
}

// TestWorkerPool_Close_PanicsWhenCalledTwice panics when already closed
func (w *WorkerPool_Close_TestSuite) TestWorkerPool_Close_PanicsWhenCalledTwice() {
	w.wp.closed = true
	defer func() {
		r := recover()
		w.Require().NotNil(r)
		w.Require().Equal("cannot call Close on a closed WorkerPool", r.(error).Error())
	}()
	w.wp.Close()
}

// TestWorkerPool_Close_SetsAvailableWorkersTo0 tests that the available workers are set to 0
// when closed is called.
func (w *WorkerPool_Close_TestSuite) TestWorkerPool_Close_SetsAvailableWorkersTo0() {
	w.wp.Close()
	w.Require().Equal(uint16(0), w.wp.poolAvail)
}

// TestWorkerPool_Close_SignalsWaitingThreadsToContinue tests that any currently waiting thread,
// as is normally held by calls to Acquire currently blocking due to having no workers available,
// releases the wait.
func (w *WorkerPool_Close_TestSuite) TestWorkerPool_Close_SignalsWaitingThreadsToContinue() {

	releaseTimeSlice := make([]time.Time, 2)
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func(ts []time.Time) {
		w.wp.mu.Lock()
		defer w.wp.mu.Unlock()
		defer wg.Done()

		w.wp.cond.Wait()
		ts[0] = time.Now()
	}(releaseTimeSlice)

	wg.Add(1)
	go func(ts []time.Time) {
		w.wp.mu.Lock()
		defer w.wp.mu.Unlock()
		defer wg.Done()

		w.wp.cond.Wait()
		ts[1] = time.Now()
	}(releaseTimeSlice)

	time.Sleep(1 * time.Second)
	broadCastTime := time.Now()
	w.wp.Close()
	wg.Wait()

	w.Require().True(releaseTimeSlice[0].After(broadCastTime), "expected releaseTime_1[%s] to be after broadcastTime[%s]", releaseTimeSlice[0], broadCastTime)
	w.Require().True(releaseTimeSlice[1].After(broadCastTime), "expected releaseTime_2[%s] to be after broadcastTime[%s]", releaseTimeSlice[1], broadCastTime)
}
