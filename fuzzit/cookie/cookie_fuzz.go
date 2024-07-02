//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"bytes"

	"github.com/pablolagos/fns"
)

func Fuzz(data []byte) int {
	c := fns.AcquireCookie()
	defer fns.ReleaseCookie(c)

	if err := c.ParseBytes(data); err != nil {
		return 0
	}

	w := bytes.Buffer{}
	if _, err := c.WriteTo(&w); err != nil {
		return 0
	}

	return 1
}
