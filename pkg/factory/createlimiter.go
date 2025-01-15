package factory

import (
	concurrencyInternal "github.com/pgvanniekerk/ezworker/internal/concurrency"
	"github.com/pgvanniekerk/ezworker/pkg/concurrency"
	"runtime"
	"time"
)

// limiterOptions represents configuration options for a Limiter, including slot count, timeout, and error behavior.
type limiterOptions struct {
	slots                 uint32
	acquireTimeout        time.Duration
	errOnNoSlotsAvailable bool
}

// limiterOption defines a functional option for customizing the behavior of a Limiter by modifying limiterOptions.
type limiterOption func(*limiterOptions)

// WithSlots configures the number of slots available in a limiter by setting the slots field in limiterOptions.
func WithSlots(slots uint32) limiterOption {
	return func(options *limiterOptions) {
		options.slots = slots
	}
}

// WithAcquireTimeout sets the timeout duration for acquiring a slot in the limiter configuration.
func WithAcquireTimeout(acquireTimeout time.Duration) limiterOption {
	return func(options *limiterOptions) {
		options.acquireTimeout = acquireTimeout
	}
}

// WithErrOnNoSlotsAvailable configures whether an error should be returned when no slots are available in the Limiter.
func WithErrOnNoSlotsAvailable(errOnNoSlotsAvailable bool) limiterOption {
	return func(options *limiterOptions) {
		options.errOnNoSlotsAvailable = errOnNoSlotsAvailable
	}
}

// CreateLimiter initializes a concurrency limiter with customizable options and returns it or an error if creation fails.
// It defaults the number of slots to CPU Core Count and sets the limiter to block on Acquire() indefinitely until
// a slot becomes available.
func CreateLimiter(opts ...limiterOption) (concurrency.Limiter, error) {

	options := &limiterOptions{}

	// Default slots to CPU core count
	options.slots = uint32(runtime.NumCPU())

	for idx := range opts {
		opts[idx](options)
	}

	return concurrencyInternal.NewLimiter(options.slots, options.acquireTimeout, options.errOnNoSlotsAvailable)
}
