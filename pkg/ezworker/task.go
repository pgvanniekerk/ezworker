package ezworker

// Task is a generic function type that processes messages of type MSG and returns an error.
// This is the core function that will be executed by the worker pool for each message.
//
// The Task function should:
// - Process the message of type MSG
// - Return nil if processing was successful
// - Return an error if processing failed
//
// Example:
//
//	// A task that processes string messages
//	stringTask := func(msg string) error {
//	    fmt.Println("Processing:", msg)
//	    return nil
//	}
//
//	// A task that processes custom message types
//	type JobRequest struct {
//	    ID   int
//	    Data string
//	}
//
//	jobTask := func(job JobRequest) error {
//	    fmt.Printf("Processing job #%d with data: %s\n", job.ID, job.Data)
//	    // Do some work...
//	    if /* some error condition */ {
//	        return errors.New("failed to process job")
//	    }
//	    return nil
//	}
type Task[MSG any] func(MSG) error
