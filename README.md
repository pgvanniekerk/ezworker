# ezworker

A simple, generic worker pool implementation for concurrent task processing in Go.

## Features

- Generic implementation that works with any message type
- Controlled concurrency with a configurable number of workers
- Graceful shutdown that waits for in-progress tasks to complete
- Dynamic resizing of the worker pool
- Error handling with customizable error handlers

## Installation

```bash
go get github.com/pgvanniekerk/ezworker
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/pgvanniekerk/ezworker/pkg/ezworker"
)

func main() {
	// Create a context that can be used to stop the worker pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Define a task function that processes string messages
	task := func(msg string) error {
		fmt.Println("Processing:", msg)
		time.Sleep(100 * time.Millisecond) // Simulate work
		return nil
	}

	// Define an error handler
	errHandler := func(err error) error {
		fmt.Println("Error:", err)
		return err
	}

	// Create a worker pool with 5 concurrent workers
	worker, msgChan := ezworker.New(ctx, 5, task, errHandler)

	// Start the worker pool
	go worker.Run()

	// Send some messages to be processed
	for i := 1; i <= 10; i++ {
		msgChan <- fmt.Sprintf("Message %d", i)
	}

	// Wait a bit to allow processing
	time.Sleep(1 * time.Second)

	// The worker pool will be stopped when the context is canceled via the defer cancel()
}
```

## Documentation

For detailed documentation and examples, see the [GoDoc](https://pkg.go.dev/github.com/pgvanniekerk/ezworker/pkg/ezworker).

## License

This project is licensed under the MIT License - see the LICENSE file for details.
