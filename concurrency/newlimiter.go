package concurrency

import "github.com/pgvanniekerk/ezworker/internal/concurrency"

// NewLimiter creates and returns a new Limiter instance with the specified maximum concurrency level (size).
func NewLimiter(size uint16) Limiter {
	return concurrency.NewLimiter(size)
}
