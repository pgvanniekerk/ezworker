package pool

import (
	"github.com/pgvanniekerk/ezworker/internal/pool"
	"github.com/pgvanniekerk/ezworker/worker"
)

// NewPool creates a new worker pool with the specified worker function, input and error channels, and concurrency size.
func NewPool[INPUT any](workerFunc worker.Func[INPUT], inputChan <-chan INPUT, errChan chan<- error, size uint16) Pool {
	return pool.NewPool(workerFunc, inputChan, errChan, size)
}
