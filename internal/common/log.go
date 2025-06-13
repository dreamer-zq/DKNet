package common

import (
	"fmt"
	"os"
)

// LogDo logs the error if it occurs
func LogDo(fn func() error) {
	if err := fn(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error in Do: %v\n", err)
	}
}

// LogMsgDo logs the error if it occurs
func LogMsgDo(msg string, fn func() error) {
	if err := fn(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error in %s: %v\n", msg, err)
	}
}
