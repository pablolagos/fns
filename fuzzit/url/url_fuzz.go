//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"bytes"

	"github.com/pablolagos/fns"
)

func Fuzz(data []byte) int {
	u := fns.AcquireURI()
	defer fns.ReleaseURI(u)

	u.UpdateBytes(data)

	w := bytes.Buffer{}
	if _, err := u.WriteTo(&w); err != nil {
		return 0
	}

	return 1
}
