<img src="https://github.com/symonk/tasq/blob/main/.github/images/logo.png" border="1" width="275" height="275"/>

[![GoDoc](https://pkg.go.dev/badge/github.com/symonk/tasq)](https://pkg.go.dev/github.com/symonk/tasq)
[![Build Status](https://github.com/symonk/tasq/actions/workflows/go_test.yml/badge.svg)](https://github.com/symonk/tasq/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/symonk/tasq/branch/main/graph/badge.svg)](https://codecov.io/gh/symonk/tasq)
[![Go Report Card](https://goreportcard.com/badge/github.com/symonk/tasq)](https://goreportcard.com/report/github.com/symonk/tasq)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://github.com/symonk/tasq/blob/master/LICENSE)


> [!CAUTION]
> tasq is currently in alpha and not fit for production level use.

# Tasq (_Task Queue_)

`tasq` is a high performance worker pool for distributing tasks across a collection of worker
goroutines.  `tasq` is dynamic in nature and auto scales depending on the number of work available
at any point in time.

If you have a bunch of tasks to do and want an easy way to distribute them in a parallel manner without
the hassle of managing worker dispatching yourself `tasq` is for you. 

`tasq` has built in support for pausing the pool for a given time (configurable via a `context`) for improved
error handling scenarios where upstream dependencies may be non functional.

> [!NOTE]
> By design `tasq` does not propagate errors or return values, tasks should handle their own persistence
> by (for example) shovelling their return values into a channel etc.

`tasq` follows semantic versioning.

-----

### Quickstart:

```go
package main

import (
    "github.com/symonk/tasq

    "time"
)

func main() {
    pool := tasq.New(tasq.WithMaxWorkers(10))
    results := make(chan string)
    for i := range 100 {
        pool.Enqueue(func() { 
            time.Sleep(time.Second)
            results <- fmt.Sprintf("%d", i)
        })
    }
    close(results)
    go func() {
        for r := range results {
            fmt.Println(r)
        }
    }()
    pool.Stop()
}
```
