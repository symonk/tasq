<img src="https://github.com/symonk/tasq/blob/main/.github/images/logo.png" border="1" width="275" height="275"/>

[![GoDoc](https://pkg.go.dev/badge/github.com/symonk/tasq)](https://pkg.go.dev/github.com/symonk/tasq)
[![Build Status](https://github.com/symonk/tasq/actions/workflows/go_test.yml/badge.svg)](https://github.com/symonk/tasq/actions/workflows/go_test.yml)
[![codecov](https://codecov.io/gh/symonk/tasq/branch/main/graph/badge.svg)](https://codecov.io/gh/symonk/tasq)
[![Go Report Card](https://goreportcard.com/badge/github.com/symonk/tasq)](https://goreportcard.com/report/github.com/symonk/tasq)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://github.com/symonk/tasq/blob/master/LICENSE)


> [!CAUTION]
> tasq is currently in alpha and not fit for production level use.

# Tasq (Task Queue)

`tasq` is a high performance worker pool for distributing tasks across a collection of worker
goroutines.  `tasq` is dynamic in nature and auto scales depending on the number of work available
at any point in time.

If you have a bunch of tasks to do and want an easy way to distribute them in a parallel manner without
the hassle of managing worker dispatching yourself `tasq` is for you. 

> [!NOTE]
> By design `tasq` does not propagate errors or return values, have your tasks listen on a channel for
> return values etc.

-----

### Quickstart:

```go

package main

import (
    "github.com/symonk/tasq"
)

func main() {
	// Instantiate a pool with whatever options fit your needs.
	pool := NewWorkerPool(
		WithMaxWorkers(10),
		WithIdleTimeout(time.Second),
		WithWaitingQueueBuffer(30),
	)

	// Enqueue some tasks, a Task is a simple func()
	// This is non blocking and the tasks will eventually be 
	// processed, if you need to wait for a task, see `EnqueueWait` below.
	for i := 0; i < 10; i++ {
		err := pool.Enqueue(func() {
			i := i
			time.Sleep(time.Duration(i) * time.Microsecond)
		})
	}

	// Wait until a task has been processed by the pool
	err := pool.EnqueueWait(context.Background(), func() {
		fmt.Println("I want to block until this has been processed by the pool")
	})

	// If for whatever reason you need to throttle the workers because perhaps your
	// database has gone offline etc.  This will cause a throttling task to be
	// sent to workers until the context is cancelled and they will then continue
	// processing tasks.  Note this is NOT immediate, there may be tasks in the 
	// worker queue that will be seen before the throttling task.
	// in future tasq will implement a seperate higher prio channel for throttling
	// tasks only to overcome this.
	ctx := context.WithTimeout(context.Background(), 1 * time.Minute)
	pool.Stall(ctx)

	// Block until the pool has cleared down queues and gracefully finalised
	// could also defer pool.Shutdown() earlier.
	pool.Shutdown()
}

```

-----

The worker pool assumes the following configurations by default, all of which are overwritable
using functional options:

 - `1024` interim queue buffer size (The queue between task submission and worker queues).
 - `5` maximum workers.
 - `5 seconds` idle scaling timeout (The time where workers are scaled down without work).

The workerpool implements the exported `Scheduler` interface.

----- 

