package concurrency

import "errors"

var ErrAcquireTimeoutReached = errors.New("acquire timeout reached")
var ErrLimiterClosed = errors.New("Limiter has been closed")
var ErrNoSlotsAvailable = errors.New("no slots available")
var ErrReleaseExceedsMaxLimit = errors.New("release exceeds limit size")
