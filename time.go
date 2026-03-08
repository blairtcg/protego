//go:build gc
// +build gc

package protego

// Required import for go:linkname to access runtime.nanotime.
// This allows us to use runtime.nanotime for monotonic time.
//nolint:revive // Required for go:linkname.
import _ "unsafe"

// nowNano returns monotonic nanoseconds since program start.
//
//go:linkname nowNano runtime.nanotime
func nowNano() int64
