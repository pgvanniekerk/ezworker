// Package ezworker provides a simple, generic worker pool implementation for concurrent task processing.
package ezworker

import (
	"errors"
)

// ErrMsgChanClosed is returned by the Run method when it detects that the message channel
// has been closed. This error indicates a normal shutdown condition when the channel is
// intentionally closed to signal that no more messages will be sent.
//
// This error can be used to distinguish between different shutdown scenarios:
//   - When the channel is closed intentionally (ErrMsgChanClosed)
//   - When the context is canceled (context.Canceled)
//   - When the context deadline is exceeded (context.DeadlineExceeded)
//
// Example handling:
//
//	go func() {
//	    err := worker.Run()
//	    if err == ezworker.ErrMsgChanClosed {
//	        log.Println("Worker pool stopped: message channel was closed")
//	    } else if err != nil {
//	        log.Printf("Worker pool error: %v", err)
//	    }
//	}()
//
//	// Later, to stop the worker pool by closing the channel:
//	close(msgChan)
var ErrMsgChanClosed = errors.New("message channel is closed")
