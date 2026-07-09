package fns

import (
	"bufio"
	"bytes"
	"testing"
)

// collectVisitAll returns the (key,value) pairs VisitAll emits, as strings.
func collectVisitAll(h *RequestHeader) []string {
	var out []string
	h.VisitAll(func(key, value []byte) {
		out = append(out, string(key)+": "+string(value))
	})
	return out
}

// collectKV returns the (key,value) pairs the indexed view (KVLen/KVAt) emits.
func collectKV(h *RequestHeader) []string {
	out := make([]string, 0, h.KVLen())
	for i, n := 0, h.KVLen(); i < n; i++ {
		key, value := h.KVAt(i)
		out = append(out, string(key)+": "+string(value))
	}
	return out
}

// buildHeader applies a scenario's mutations onto a fresh RequestHeader.
type headerScenario struct {
	name  string
	build func(h *RequestHeader)
}

func kvIndexScenarios() []headerScenario {
	return []headerScenario{
		{"empty", func(h *RequestHeader) {}},
		{"only-regular", func(h *RequestHeader) {
			h.Set("X-Foo", "bar")
			h.Set("X-Baz", "qux")
		}},
		{"host-only", func(h *RequestHeader) { h.SetHost("example.com") }},
		{"content-type-length", func(h *RequestHeader) {
			h.SetContentType("application/json")
			h.SetContentLength(42)
		}},
		{"user-agent", func(h *RequestHeader) { h.SetUserAgent("Mozilla/5.0") }},
		{"single-cookie", func(h *RequestHeader) { h.SetCookie("sid", "deadbeef") }},
		{"multi-cookie", func(h *RequestHeader) {
			h.SetCookie("a", "1")
			h.SetCookie("b", "2")
		}},
		{"connection-close", func(h *RequestHeader) {
			h.Set("X-Foo", "bar")
			h.SetConnectionClose()
		}},
		{"trailer", func(h *RequestHeader) {
			_ = h.SetTrailer("X-Checksum")
		}},
		{"full-mix", func(h *RequestHeader) {
			h.SetHost("example.com")
			h.SetContentType("application/json")
			h.SetContentLength(17)
			h.SetUserAgent("curl/8.0")
			h.Set("X-Forwarded-For", "203.0.113.9")
			h.Set("Accept", "*/*")
			h.SetCookie("sid", "deadbeefdeadbeef")
			h.SetConnectionClose()
		}},
	}
}

// TestRequestHeaderKVIndexMatchesVisitAll asserts the indexed header view is a
// byte-for-byte, same-order replica of VisitAll across representative header
// combinations (specials, regular headers, cookies, trailer, Connection:close).
func TestRequestHeaderKVIndexMatchesVisitAll(t *testing.T) {
	for _, sc := range kvIndexScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			var h RequestHeader
			sc.build(&h)

			want := collectVisitAll(&h)
			got := collectKV(&h)

			if len(want) != len(got) {
				t.Fatalf("length mismatch: VisitAll=%d (%v) KV=%d (%v)", len(want), want, len(got), got)
			}
			for i := range want {
				if want[i] != got[i] {
					t.Fatalf("entry %d mismatch:\n  VisitAll: %q\n  KV:       %q\n  full VisitAll=%v\n  full KV=%v",
						i, want[i], got[i], want, got)
				}
			}
		})
	}
}

// TestRequestHeaderKVIndexMatchesVisitAllWireParsed asserts parity on the
// production path: headers parsed from the wire (populating h.h and rawHeaders,
// including duplicate header names and specials that the parser routes into
// dedicated fields), not synthesized via setters.
func TestRequestHeaderKVIndexMatchesVisitAllWireParsed(t *testing.T) {
	raw := "GET /path HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"User-Agent: curl/8.0\r\n" +
		"Accept: */*\r\n" +
		"X-Forwarded-For: 203.0.113.9\r\n" +
		"X-Forwarded-For: 198.51.100.7\r\n" + // duplicate name
		"Cookie: a=1; b=2\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 5\r\n" +
		"Connection: close\r\n" +
		"\r\n"

	var h RequestHeader
	if err := h.Read(bufio.NewReader(bytes.NewReader([]byte(raw)))); err != nil {
		t.Fatalf("Read: %v", err)
	}

	want := collectVisitAll(&h)
	got := collectKV(&h)
	if len(want) != len(got) {
		t.Fatalf("length mismatch: VisitAll=%d (%v) KV=%d (%v)", len(want), want, len(got), got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("entry %d mismatch:\n  VisitAll: %q\n  KV:       %q\n  full VisitAll=%v\n  full KV=%v",
				i, want[i], got[i], want, got)
		}
	}
	// Sanity: the duplicate header must appear twice in both views.
	dup := 0
	for _, e := range got {
		if e == "X-Forwarded-For: 203.0.113.9" || e == "X-Forwarded-For: 198.51.100.7" {
			dup++
		}
	}
	if dup != 2 {
		t.Fatalf("expected both duplicate X-Forwarded-For values, got %d (%v)", dup, got)
	}
}

// TestRequestHeaderKVIndexResetInvalidates asserts the cached view is rebuilt
// after a Reset, so a pooled/reused RequestHeader never serves a stale layout.
func TestRequestHeaderKVIndexResetInvalidates(t *testing.T) {
	var h RequestHeader
	h.SetHost("first.example.com")
	h.Set("X-First", "1")

	first := collectKV(&h)
	if len(first) != 2 {
		t.Fatalf("expected 2 headers before reset, got %d (%v)", len(first), first)
	}

	h.Reset()
	h.SetHost("second.example.com")
	h.Set("X-Second", "2")
	h.Set("X-Third", "3")

	second := collectKV(&h)
	want := collectVisitAll(&h)
	if len(second) != len(want) {
		t.Fatalf("after reset: KV=%d (%v) VisitAll=%d (%v)", len(second), second, len(want), want)
	}
	for i := range want {
		if second[i] != want[i] {
			t.Fatalf("after reset entry %d: KV=%q VisitAll=%q", i, second[i], want[i])
		}
	}
	// Ensure no stale first-request data leaked through.
	for _, e := range second {
		if e == "Host: first.example.com" || e == "X-First: 1" {
			t.Fatalf("stale entry from before reset leaked: %q", e)
		}
	}
}

// TestRequestHeaderKVIndexZeroAlloc asserts that iterating the indexed view is
// allocation-free once built (the whole point: no VisitAll closure). The build
// itself is amortized by AllocsPerRun's warm-up run, matching the real hot path
// where the view is built once per request and iterated many times.
func TestRequestHeaderKVIndexZeroAlloc(t *testing.T) {
	var h RequestHeader
	h.SetHost("example.com")
	h.SetContentType("application/json")
	h.SetUserAgent("curl/8.0")
	h.Set("X-Forwarded-For", "203.0.113.9")
	h.Set("Accept", "*/*")
	h.SetCookie("sid", "deadbeefdeadbeef")
	h.SetConnectionClose()

	var sink int
	allocs := testing.AllocsPerRun(200, func() {
		for i, n := 0, h.KVLen(); i < n; i++ {
			key, value := h.KVAt(i)
			sink += len(key) + len(value)
		}
	})
	_ = sink
	if allocs != 0 {
		t.Fatalf("indexed header iteration must be 0-alloc, got %v", allocs)
	}
}

// TestRequestHeaderKVIndexRebuildZeroAlloc asserts that rebuilding the view
// after invalidation reuses the special-header buffers (Cookie/Trailer), so a
// pooled header does not allocate on subsequent requests. It toggles the
// internal kvIndexReady flag directly to isolate buildKVIndex's cost from the
// unrelated allocations of Reset/Set.
func TestRequestHeaderKVIndexRebuildZeroAlloc(t *testing.T) {
	var h RequestHeader
	h.SetHost("example.com")
	h.SetCookie("a", "1")
	h.SetCookie("b", "2")
	if err := h.SetTrailer("X-Checksum"); err != nil {
		t.Fatalf("SetTrailer: %v", err)
	}
	h.buildKVIndex() // prime kvCookieBuf / kvTrailerBuf

	allocs := testing.AllocsPerRun(200, func() {
		h.kvIndexReady = false // force a full rebuild
		h.buildKVIndex()
	})
	if allocs != 0 {
		t.Fatalf("index rebuild must reuse special-header buffers (0-alloc), got %v", allocs)
	}
}
