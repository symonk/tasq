<img src="https://github.com/symonk/tasq/blob/main/.github/images/logo.png" border="1" width="275" height="275"/>

[![GoDoc](https://pkg.go.dev/badge/github.com/symonk/tasq)](https://pkg.go.dev/github.com/symonk/tasq)
[![Build Status](https://github.com/symonk/tasq/actions/workflows/go_test.yml/badge.svg)](https://github.com/symonk/tasq/actions/workflows/go_test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/symonk/tasq)](https://goreportcard.com/report/github.com/symonk/tasq)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://github.com/symonk/tasq/blob/master/LICENSE)


# Tasq (Task Queue)

`tasq` is a high performance worker pool for distributing tasks across a collection of worker
goroutines.  `tasq` is dynamic in nature and auto scales depending on the number of work available
at any point in time.

-----

### Quickstart:

```go

package main

import (
    "github.com/symonk/tasq"
)

func main() {
	// Instantiate a pool with whatever options fit your needs.
	pool := New(
		WithMaxWorkers(10),
		WithIdleTimeout(time.Second),
		WithWaitingQueueBuffer(30),
	)
	// Defer the pool toshutdown, this is blocking until tasks have finished.
	defer pool.Shutdown()

	// Enqueue some tasks, a Task is a simple func()
	for i := 0; i < 10; i++ {
		pool.Enqueue(func() {
			i := i
			time.Sleep(time.Duration(i) * time.Microsecond)
		})
	}
}

```

-----

