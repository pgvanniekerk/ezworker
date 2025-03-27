package ezworker

// Package ezworker provides a simple, generic worker pool implementation for concurrent task processing.
//
// EzWorker allows for easy creation and management of worker pools that can process arbitrary tasks
// with type-safe message passing. It handles concurrency control, graceful shutdown, and error handling,
// making it straightforward to implement parallel processing in Go applications.
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
//   - Dynamic resizing of the worker pool
//   - Error handling with customizable error handlers
//
// # Basic Usage
//
// Here's a simple example of how to use the ezworker package:
//
//	package main
//
//	import (
//		"context"
//		"fmt"
//		"time"
//
//		"github.com/yourusername/ezworker/pkg/ezworker"
//	)
//
//	func main() {
//		// Create a context that can be used to Stop the worker pool
//		ctx, cancel := context.WithCancel(context.Background())
//		defer cancel()
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
//			fmt.Println("Error:", err)
//		}
//
//		// Create a worker pool with 5 concurrent workers
//		worker, msgChan := ezworker.New(ctx, 5, task, errHandler)
//
//		// Start the worker pool
//		go worker.Run()
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
//		// Cancel the context to Stop the worker pool
//		cancel()
//
//		// Wait for the worker pool to Stop
//		time.Sleep(100 * time.Millisecond)
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
//		// Do some work...
//		return nil
//	}
//
//	// Create a worker pool for the custom type
//	worker, jobChan := ezworker.New(ctx, 5, jobTask, errHandler)
//	go worker.Run()
//
//	// Send a custom message
//	jobChan <- JobRequest{ID: 1, Data: "example"}
//
// # Best Practices
//
// 1. Always run the worker pool in a separate goroutine using `go worker.Run()`.
//
// 2. Use a context with cancel to control the lifecycle of the worker pool:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel() // Ensure the worker pool is stopped when the function returns
//
// 3. Handle errors appropriately by providing an error handler function:
//
//	errHandler := func(err error) {
//		log.Printf("Task error: %v", err)
//		// You might want to log the error, increment metrics, etc.
//	}
//
// 4. Size your worker pool appropriately based on the nature of your tasks:
//    - CPU-bound tasks: typically use runtime.NumCPU() workers
//    - I/O-bound tasks: may use more workers than CPU cores
//
// 5. For graceful shutdown, cancel the context and allow time for tasks to complete:
//
//	cancel() // Signal the worker pool to Stop
//	// Optionally wait a bit to allow in-progress tasks to complete
//	time.Sleep(100 * time.Millisecond)
//
// 6. Use the Resize method to adjust the number of workers based on load:
//
//	// Increase workers during high load
//	worker.Resize(10)
//
//	// Decrease workers during low load
//	worker.Resize(2)
