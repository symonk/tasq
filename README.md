![Tasq](.github/images/logo.png)

[![GoDoc](https://pkg.go.dev/badle/github.com/symonk/tasq)](https://pkg.go.dev/github.com/symonk/tasq)
[![Build Status](https://github.com/symonk/tasq/actions/workflows/go_test.yml/badge.svg)](https://github.com/symonk/tasq/actions/workflows/go_test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/symonk/tasq)](https://goreportcard.com/report/github.com/symonk/tasq)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://github.com/symonk/tasq/blob/master/LICENSE)


# tasq

`tasq` is a high performance worker pool for distributing tasks across a collection of worker
goroutines.

-----

### Quickstart:

```go

import (
    "github.com/symonk/tasq"
)

func main() {
    
    // Configure and launch a new pool
    pool := tasq.New(WithMaxWorkers(10), withIdleTimeout(2 * time.Seconds))
    defer pool.Shutdown()

    // Send tasks to the pool
    for i := 0; i < 10; i++ { 
        pool.Enqueue(func() {
            fmt.Println(i)
        })
    }
}

```
