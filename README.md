# protego

PLightweight, scalable cricuit breaker for Go.

## What is a circuit breaker?

A circuit breaker prevents your application from making calls to a service that is not working. When a service fails repeatedly, the circuit breaker *opens* and blocks new requests. After a short time, it allows a few requests through to test if the service has recovered. If the service is working again, the circuit breaker *closes* and normal operation continues.

For reference, please see <https://learn.microsoft.com/en-us/azure/architecture/patterns/circuit-breaker>

```mermaid
stateDiagram-v2
    [*] --> Closed
    Closed --> Open: failures exceed threshold
    Open --> HalfOpen: timeout elapsed
    HalfOpen --> Closed: request succeeds
    HalfOpen --> Open: request fails
    Open --> [*]
    Closed --> [*]
```

## Install

```bash
go get github.com/blairtcg/protego
```

## Quick start

```go
package main

import (
    "fmt"
    "github.com/blairtcg/protego"
)

func main() {
    cb := protego.New(protego.Config{
        Name:    "my-service",
        Timeout: 30 * time.Second,
    })

    err := cb.Execute(func() error {
        // call your service here
        return nil
    })
    if err != nil {
        fmt.Println("Request blocked:", err)
    }
}
```

## Config options

| Option | Description | Default |
|--------|-------------|---------|
| Name | Identifier for the breaker | "" |
| MaxRequests | Requests allowed in half-open state | 1 |
| Interval | Time before resetting counts in closed state | 0 |
| Timeout | Time before transitioning to half-open | 60s |
| ReadyToTrip | Function that returns true when breaker should open | 6 consecutive failures |
| IsSuccessful | Function that returns true for successful errors | nil error |
| OnStateChange | Function called when state changes | nil |
| HalfOpenMaxQueueSize | Max queue size in half-open state | 0 (no queue) |

## States

- **Closed**: Normal operation. Requests pass through.
- **Open**: Service is not working. Requests are rejected.
- **Half-open**: Testing if service has recovered. Limited requests allowed.

```mermaid
flowchart TD
    A[Request] --> B{Circuit State}
    B -->|Closed| C[Allow]
    B -->|Open| D[Reject]
    B -->|HalfOpen| E{Available?}
    C --> F[Execute]
    D --> G[Error]
    E -->|Yes| C
    E -->|No| D
    F --> H{Success}
    H -->|Yes| I[Success]
    H -->|No| J[Failure]
    I --> K[Check]
    J --> K
    K --> L{Trip}
    L -->|Yes| M[Open]
    L -->|No| N[Closed]
```

## Examples

See the `examples` folder for more use cases.

## Benchmark results

Tested on Intel Celeron N3060:

| Library | ns/op |
|---------|-------|
| Protego | 65 |
| Sony/Gobreaker | 388 |
| Rubyist/Circuitbreaker | 252 |

Tests run with `go test -bench=. -benchtime=1s -cpu=1`.

## Requirements

- Go 1.19 or later

## License

MIT
