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
