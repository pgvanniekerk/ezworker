// Package ezworker provides a simple, generic worker pool implementation for concurrent task processing.
//
// EzWorker allows for easy creation and management of worker pools that can process arbitrary tasks
// with type-safe message passing. It handles concurrency control, graceful shutdown, and error handling,
// making it straightforward to implement parallel processing in Go applications.
//
// # Dual Purpose Design
//
// The ezworker package is designed to serve two distinct purposes:
//
//  1. As a standalone worker pool library for concurrent task processing
//  2. As a Runnable component in the ezapp framework
//
// This dual-purpose design allows for maximum flexibility, enabling you to use ezworker
// in different contexts depending on your application's needs.
//
// # Overview
//
// The ezworker package is designed to simplify the creation and management of worker pools in Go.
// It provides a generic implementation that can work with any message type, allowing for type-safe
// concurrent processing of tasks.
//
// Key features:
//   - Generic implementation that works with any message type
//   - Controlled concurrency with a configurable number of workers
//   - Graceful shutdown that waits for in-progress tasks to complete
//   - Dynamic resizing of the worker pool at runtime
//   - Error handling with customizable error handlers
//   - Integration with the ezapp framework for lifecycle management
//
// # Usage as a Standalone Worker Pool
//
// When used as a standalone library, you create an EzWorker using the New function
// and start it by calling the Run method in a goroutine.
//
// Here's a simple example:
//
//	package main
//
//	import (
//		"context"
//		"fmt"
//		"log"
//		"time"
//
//		"github.com/pgvanniekerk/ezworker/pkg/ezworker"
//	)
//
//	func main() {
//		// Create a message channel
//		msgChan := make(chan string)
//
//		// Define a task function that processes string messages
//		task := func(msg string) error {
//			fmt.Println("Processing:", msg)
//			time.Sleep(100 * time.Millisecond) // Simulate work
//			return nil
//		}
//
//		// Define an error handler
//		errHandler := func(err error) {
//			log.Printf("Task error: %v", err)
//		}
//
//		// Create a worker pool with 5 concurrent workers
//		worker := ezworker.New(5, task, msgChan)
//		worker.errHandler = errHandler
//
//		// Start the worker pool in a separate goroutine
//		go func() {
//			if err := worker.Run(); err != nil {
//				log.Printf("Worker pool error: %v", err)
//			}
//		}()
//
//		// Send some messages to be processed
//		for i := 1; i <= 10; i++ {
//			msgChan <- fmt.Sprintf("Message %d", i)
//		}
//
//		// Wait a bit to allow processing
//		time.Sleep(1 * time.Second)
//
//		// Resize the worker pool to have 2 concurrent workers
//		worker.Resize(2)
//
//		// Send more messages
//		for i := 11; i <= 20; i++ {
//			msgChan <- fmt.Sprintf("Message %d", i)
//		}
//
//		// Wait a bit more
//		time.Sleep(1 * time.Second)
//
//		// Close the channel to stop the worker pool
//		close(msgChan)
//
//		// Create a context with timeout for graceful shutdown
//		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//		defer cancel()
//
//		// Wait for the worker pool to stop gracefully
//		if err := worker.Stop(ctx); err != nil {
//			log.Printf("Failed to stop worker pool gracefully: %v", err)
//		}
//	}
//
// # Usage with ezapp Framework
//
// When used with ezapp, the EzWorker can be registered as a Runnable component,
// allowing it to be managed by the ezapp framework's lifecycle and logging systems.
//
// Here's an example of how to use EzWorker with ezapp:
//
//	package main
//
//	import (
//		"github.com/pgvanniekerk/ezapp/pkg/ezapp"
//		"github.com/pgvanniekerk/ezworker/pkg/ezworker"
//	)
//
//	func main() {
//		// Create a new ezapp application
//		app := ezapp.New("MyApp")
//
//		// Create a message channel
//		msgChan := make(chan string)
//
//		// Define a task function
//		task := func(msg string) error {
//			app.Logger().Info("Processing message", "msg", msg)
//			return nil
//		}
//
//		// Create a worker pool
//		worker := ezworker.New(5, task, msgChan)
//
//		// Register the worker pool as a runnable component
//		app.RegisterRunnable("worker", worker)
//
//		// Start the application (this will call worker.Run())
//		app.Start()
//
//		// Send messages to be processed
//		msgChan <- "Hello, World!"
//
//		// The application will handle graceful shutdown of the worker pool
//	}
//
// # Working with Custom Message Types
//
// EzWorker can work with any message type, including custom structs:
//
//	// Define a custom message type
//	type JobRequest struct {
//		ID   int
//		Data string
//	}
//
//	// Create a task function for the custom type
//	jobTask := func(job JobRequest) error {
//		fmt.Printf("Processing job #%d with data: %s\n", job.ID, job.Data)
//
//		// Perform validation
//		if job.ID <= 0 {
//			return errors.New("invalid job ID")
//		}
//
//		// Do some work...
//		result, err := processData(job.Data)
//		if err != nil {
//			return fmt.Errorf("failed to process job #%d: %w", job.ID, err)
//		}
//
//		// Save the result
//		if err := saveResult(job.ID, result); err != nil {
//			return fmt.Errorf("failed to save result for job #%d: %w", job.ID, err)
//		}
//
//		return nil
//	}
//
//	// Create a message channel for the custom type
//	jobChan := make(chan JobRequest)
//
//	// Create a worker pool for the custom type
//	worker := ezworker.New(5, jobTask, jobChan)
//
//	// Start the worker pool
//	go worker.Run()
//
//	// Send a custom message
//	jobChan <- JobRequest{ID: 1, Data: "example"}
//
// # Best Practices
//
// 1. Always run the worker pool in a separate goroutine using `go worker.Run()` when used as a standalone library.
//
// 2. Implement proper error handling by providing an error handler function:
//
//	errHandler := func(err error) {
//		log.Printf("Task error: %v", err)
//		// You might want to log the error, increment metrics, etc.
//	}
//
//	worker := ezworker.New(5, task, msgChan)
//	worker.errHandler = errHandler
//
// 3. Size your worker pool appropriately based on the nature of your tasks:
//   - CPU-bound tasks: typically use runtime.NumCPU() workers
//   - I/O-bound tasks: may use more workers than CPU cores
//   - Consider the resource requirements of your tasks
//
// 4. For graceful shutdown, use the Stop method with a timeout context:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	if err := worker.Stop(ctx); err != nil {
//	    log.Printf("Failed to stop worker pool gracefully: %v", err)
//	}
//
// 5. Use the Resize method to dynamically adjust the number of workers based on load:
//
//	// Increase workers during high load
//	if queueSize > highLoadThreshold {
//	    worker.Resize(10)
//	}
//
//	// Decrease workers during low load
//	if queueSize < lowLoadThreshold {
//	    worker.Resize(2)
//	}
//
// 6. When using with ezapp, let the framework handle the lifecycle:
//
//	app.RegisterRunnable("worker", worker)
package ezworker
