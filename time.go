//go:build gc

package protego

// Required import for go:linkname to access runtime.nanotime.
// This allows us to use runtime.nanotime for monotonic time.
//nolint:revive // Required for go:linkname.
import _ "unsafe"

// we will mark this to be removed soon as its not beneficial.
// it is documented more clearly here 
// https://golang.bg/src/runtime/time_nofake.go#:~:text=15%20var%20faketime%20int64%2016,for%20a%20fast%20monotonic%20time. 
// nowNano returns monotonic nanoseconds since program start.
//
//go:linkname nowNano runtime.nanotime
func nowNano() int64
