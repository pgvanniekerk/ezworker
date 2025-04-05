# ezworker

A simple, generic worker pool implementation for concurrent task processing in Go. ezworker serves as both a standalone worker pool library and a Runnable component for the [ezapp framework](https://github.com/pgvanniekerk/ezapp).

## Features

- Generic implementation that works with any message type
- Controlled concurrency with a configurable number of workers
- Graceful shutdown that waits for in-progress tasks to complete
- Dynamic resizing of the worker pool at runtime
- Error handling with customizable error handlers
- Integration with the ezapp framework for lifecycle management

## Installation

```bash
go get github.com/pgvanniekerk/ezworker
```

## Dual Purpose Design

The ezworker package is designed to serve two distinct purposes:

1. **As a standalone worker pool library** for concurrent task processing
2. **As a Runnable component in the ezapp framework**

This dual-purpose design allows for maximum flexibility, enabling you to use ezworker in different contexts depending on your application's needs.

## Usage as a Standalone Worker Pool

When used as a standalone library, you create an EzWorker using the New function and start it by calling the Run method in a goroutine.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pgvanniekerk/ezworker/pkg/ezworker"
)

func main() {
	// Create a message channel
	msgChan := make(chan string)

	// Define a task function that processes string messages
	task := func(msg string) error {
		fmt.Println("Processing:", msg)
		time.Sleep(100 * time.Millisecond) // Simulate work
		return nil
	}

	// Define an error handler
	errHandler := func(err error) {
		log.Printf("Task error: %v", err)
	}

	// Create a worker pool with 5 concurrent workers
	worker := ezworker.New(5, task, msgChan)
	worker.errHandler = errHandler

	// Start the worker pool in a separate goroutine
	go func() {
		if err := worker.Run(); err != nil {
			log.Printf("Worker pool error: %v", err)
		}
	}()

	// Send some messages to be processed
	for i := 1; i <= 10; i++ {
		msgChan <- fmt.Sprintf("Message %d", i)
	}

	// Wait a bit to allow processing
	time.Sleep(1 * time.Second)

	// Resize the worker pool to have 2 concurrent workers
	worker.Resize(2)

	// Send more messages
	for i := 11; i <= 20; i++ {
		msgChan <- fmt.Sprintf("Message %d", i)
	}

	// Wait a bit more
	time.Sleep(1 * time.Second)

	// Close the channel to stop the worker pool
	close(msgChan)

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Wait for the worker pool to stop gracefully
	if err := worker.Stop(ctx); err != nil {
		log.Printf("Failed to stop worker pool gracefully: %v", err)
	}
}
```

## Usage with ezapp Framework

When used with ezapp, the EzWorker can be integrated as a Runnable component, allowing it to be managed by the ezapp framework's lifecycle and logging systems. The recommended approach is to use the wire package to create and configure your application.

### Step 1: Create a Configuration Structure

First, create a configuration structure that will hold settings for your worker pool:

```go
package config

// Config contains configuration for the application.
type Config struct {
	// WorkerCount is the number of concurrent workers in the pool
	// Environment variable: WORKER_COUNT
	// Default: 5
	WorkerCount int64 `envvar:"WORKER_COUNT" default:"5"`

	// LogLevel sets the application's logging level
	// Environment variable: LOG_LEVEL
	// Default: info
	LogLevel string `envvar:"LOG_LEVEL" default:"info"`

	// AppName is the name of the application
	// Environment variable: APP_NAME
	// Required: true
	AppName string `envvar:"APP_NAME" required:"true"`
}
```

### Step 2: Create a Wire Package

Create a wire package with a Build function that creates and configures your worker pool:

```go
package wire

import (
	"context"
	"log/slog"
	"time"

	"github.com/yourusername/myapp/internal/config"
	"github.com/pgvanniekerk/ezapp/internal/app"
	appwire "github.com/pgvanniekerk/ezapp/pkg/wire"
	"github.com/pgvanniekerk/ezworker/pkg/ezworker"
)

// Build creates an application with all dependencies wired up.
// This function matches the ezapp.Builder signature and is used
// with ezapp.Run to create and run the application.
func Build(startupCtx context.Context, cfg config.Config) (*app.App, error) {
	// Create a message channel
	msgChan := make(chan string)

	// Define a task function
	task := func(msg string) error {
		slog.Info("Processing message", "msg", msg)
		return nil
	}

	// Create a worker pool
	worker := ezworker.New(cfg.WorkerCount, task, msgChan)

	// Set up a goroutine to send messages to the worker pool
	go func() {
		// This is just an example - in a real application, you would
		// likely get messages from an external source
		for i := 1; i <= 10; i++ {
			msgChan <- "Message " + string(i)
			time.Sleep(1 * time.Second)
		}
	}()

	// Create and return the app using the wire package
	return appwire.App(
		// Provide the worker as a runnable component
		appwire.Runnables(worker),

		// Configure application timeouts
		appwire.WithAppStartupTimeout(15*time.Second),
		appwire.WithAppShutdownTimeout(15*time.Second),

		// Add custom log attributes
		appwire.WithLogAttrs(
			slog.String("app", cfg.AppName),
			slog.String("component", "worker"),
		),
	)
}
```

### Step 3: Create the Main Entry Point

Finally, create the main entry point for your application:

```go
package main

import (
	"github.com/yourusername/myapp/internal/wire"
	"github.com/pgvanniekerk/ezapp/pkg/ezapp"
)

func main() {
	// Run the application with the wire.Build function
	ezapp.Run(wire.Build)
}
```

This approach leverages ezapp's built-in features:

1. **Configuration Management**: ezapp automatically loads configuration from environment variables
2. **Lifecycle Management**: ezapp handles starting and stopping your worker pool
3. **Graceful Shutdown**: ezapp ensures your worker pool shuts down gracefully
4. **Structured Logging**: ezapp provides a structured logger that your worker pool can use

The `ezapp.Run` function:
1. Creates a background context
2. Loads configuration from environment variables
3. Calls the Build function to create the application
4. Runs the application and handles signals for graceful shutdown


## Working with Custom Message Types

EzWorker can work with any message type, including custom structs:

```go
// Define a custom message type
type JobRequest struct {
	ID   int
	Data string
}

// Create a task function for the custom type
jobTask := func(job JobRequest) error {
	fmt.Printf("Processing job #%d with data: %s\n", job.ID, job.Data)

	// Perform validation
	if job.ID <= 0 {
		return errors.New("invalid job ID")
	}

	// Do some work...
	result, err := processData(job.Data)
	if err != nil {
		return fmt.Errorf("failed to process job #%d: %w", job.ID, err)
	}

	// Save the result
	if err := saveResult(job.ID, result); err != nil {
		return fmt.Errorf("failed to save result for job #%d: %w", job.ID, err)
	}

	return nil
}

// Create a message channel for the custom type
jobChan := make(chan JobRequest)

// Create a worker pool for the custom type
worker := ezworker.New(5, jobTask, jobChan)

// Start the worker pool
go worker.Run()

// Send a custom message
jobChan <- JobRequest{ID: 1, Data: "example"}
```

## Best Practices

1. **Always run the worker pool in a separate goroutine** using `go worker.Run()` when used as a standalone library.

2. **Implement proper error handling** by providing an error handler function:

```go
errHandler := func(err error) {
	log.Printf("Task error: %v", err)
	// You might want to log the error, increment metrics, etc.
}

worker := ezworker.New(5, task, msgChan)
worker.errHandler = errHandler
```

3. **Size your worker pool appropriately** based on the nature of your tasks:
   - CPU-bound tasks: typically use `runtime.NumCPU()` workers
   - I/O-bound tasks: may use more workers than CPU cores
   - Consider the resource requirements of your tasks

4. **For graceful shutdown**, use the Stop method with a timeout context:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := worker.Stop(ctx); err != nil {
    log.Printf("Failed to stop worker pool gracefully: %v", err)
}
```

5. **Use the Resize method** to dynamically adjust the number of workers based on load:

```go
// Increase workers during high load
if queueSize > highLoadThreshold {
    worker.Resize(10)
}

// Decrease workers during low load
if queueSize < lowLoadThreshold {
    worker.Resize(2)
}
```

6. **When using with ezapp**, let the framework handle the lifecycle:

```go
app.RegisterRunnable("worker", worker)
```

## API Documentation

For detailed API documentation, see the [GoDoc](https://pkg.go.dev/github.com/pgvanniekerk/ezworker/pkg/ezworker).

## License

This project is licensed under the MIT License - see the LICENSE file for details.
