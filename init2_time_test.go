package fns

import (
	"testing"
	"time"
)

func TestInit2SetsRequestTime(t *testing.T) {
	var ctx RequestCtx
	before := time.Now()
	ctx.Init2(nil, nil, true)
	if ctx.Time().IsZero() {
		t.Fatal("Init2 left ctx.Time() at zero")
	}
	if ctx.Time().Before(before) {
		t.Fatalf("ctx.Time() = %v, want >= %v", ctx.Time(), before)
	}
}
