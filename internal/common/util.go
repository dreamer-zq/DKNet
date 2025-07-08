package common

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"
)

// Map applies a function to each element in the input slice and returns a new slice.
func Map[T any, R any](in []T, f func(T) R) []R {
	out := make([]R, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

// Distinct returns a distinct slice of the input slice
func Distinct[T comparable](input []T) []T {
	seen := make(map[T]struct{})
	var result []T
	for _, v := range input {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// Retry retries a function until it returns a non-zero value
func Retry[T comparable](fun func() T, delaySecond, tryCnt int) T {
	var zero T
	for i := 0; i < tryCnt; i++ {
		result := fun()
		if result != zero {
			return result
		}
		if i < tryCnt-1 {
			time.Sleep(time.Duration(delaySecond) * time.Second)
		}
	}
	return zero
}

// Now returns the current time in UTC
func Now() *time.Time {
	now := time.Now().UTC()
	return &now
}

// SafeGo runs a function in a goroutine and sends the result to the channel
// If the function panics, the panic is sent to the channel
func SafeGo(ch chan<- any, f func() any) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Recovered from panic,Stack trace:\n %s", string(debug.Stack()))
				ch <- r
			}
		}()

		if r := f(); r != nil {
			ch <- r
		}
	}()
}

// ConvertToAnyCh converts a typed channel to a generic channel
func ConvertToAnyCh[T any](ch chan T) chan any {
	out := make(chan any)
	go func() {
		for v := range ch {
			out <- v
		}
		close(out)
	}()
	return out
}
