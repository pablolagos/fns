//go:build windows
// +build windows

package fns

import "testing"

func TestURIPathNormalizeIssue86(t *testing.T) {
	t.Parallel()

	// see https://github.com/powerwaf-cdn/fasthttp/issues/86
	var u URI

	testURIPathNormalize(t, &u, `C:\a\b\c\fs.go`, `C:\a\b\c\fs.go`)

	testURIPathNormalize(t, &u, `a`, `/a`)

	testURIPathNormalize(t, &u, "/../../../../../foo", "/foo")

	testURIPathNormalize(t, &u, "/..\\..\\..\\..\\..\\", "/")

	testURIPathNormalize(t, &u, "/..%5c..%5cfoo", "/foo")
}
