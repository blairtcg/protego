package protego

import (
	"time"
)

// nowNano returns monotonic nanoseconds since program start.
// Uses time.Now() which automatically handles monotonic clock.
func nowNano() int64 {
	return time.Now().UnixNano()
}
