//go:build windows
// +build windows

package fns

import (
	"errors"
	"syscall"
)

func isConnectionReset(err error) bool {
	return errors.Is(err, syscall.WSAECONNRESET)
}
