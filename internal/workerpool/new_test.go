package workerpool

import (
	"github.com/stretchr/testify/suite"
	"testing"
)

// Test_New_TestSuite executes the test suite for the New function
// of this package
func Test_New_TestSuite(t *testing.T) {
	suite.Run(t, new(New_TestSuite))
}

// New_TestSuite tests the New function of this package.
type New_TestSuite struct {
	suite.Suite
}

// Test_New_PanicOn0Workers ensures that the function panics if the caller
// specifies the number 0 as the number of workers to create the WorkerPool
// with.
func (suite *New_TestSuite) Test_New_PanicOn0Workers() {
	defer func() {
		r := recover()
		suite.Require().NotNil(r)
		suite.Require().Equal("cannot create a WorkerPool with 0 workers", r.(error).Error())
	}()
	New(0)
}

// Test_New_InitializesWithProvidedSizeAndAvailWorkers ensures that the WorkerPool
// is initialized with the required number of workers in the pool and size
func (suite *New_TestSuite) Test_New_InitializesWithProvidedSizeAndAvailWorkers() {
	wp := New(10)
	suite.Require().Equal(wp.poolAvail, uint16(10))
	suite.Require().Equal(wp.poolSize, uint16(10))
}
