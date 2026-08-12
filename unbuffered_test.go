package fns

import (
	"strings"
	"testing"
)

// The unbuffered path writes the response headers itself, from inside the
// handler, so every framing decision serveConn normally makes after the handler
// returns (chunked vs identity, Connection, HEAD's empty body) has to be made
// there instead. These tests pin the wire bytes for each combination.

// unbufferedHandler streams a body the way the static-file, FastCGI and proxy
// handlers do: DisableBuffering, then write. A negative contentLength means the
// handler does not know the size up front.
func unbufferedHandler(body string, contentLength int) RequestHandler {
	return func(ctx *RequestCtx) {
		ctx.DisableBuffering()
		defer ctx.CloseResponse() //nolint:errcheck
		ctx.SetStatusCode(StatusOK)
		ctx.SetContentType("text/plain")
		if contentLength >= 0 {
			ctx.Response.Header.SetContentLength(contentLength)
		}
		_, _ = ctx.WriteString(body)
	}
}

// serveRaw feeds requests to a server over a fake connection and returns
// everything written back, unparsed.
func serveRaw(t *testing.T, h RequestHandler, requests ...string) string {
	t.Helper()

	s := &Server{Handler: h}
	rw := &readWriter{}
	for _, req := range requests {
		rw.r.WriteString(req)
	}
	if err := s.ServeConn(rw); err != nil {
		t.Fatalf("unexpected error from ServeConn: %v", err)
	}
	return rw.w.String()
}

func TestUnbufferedHTTP11UnknownLengthIsChunked(t *testing.T) {
	t.Parallel()

	resp := serveRaw(t, unbufferedHandler("hello world", -1),
		"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")

	if !strings.Contains(resp, "Transfer-Encoding: chunked") {
		t.Fatalf("expecting chunked framing for an HTTP/1.1 client, got:\n%s", resp)
	}
	if !strings.HasSuffix(resp, "b\r\nhello world\r\n0\r\n\r\n") {
		t.Fatalf("expecting a chunked body terminated by the last chunk, got:\n%s", resp)
	}
}

func TestUnbufferedHTTP10UnknownLengthClosesInsteadOfChunking(t *testing.T) {
	t.Parallel()

	// Both with and without keep-alive: chunked is not available to an HTTP/1.0
	// client either way, so the body is delimited by the close.
	for _, req := range []string{
		"GET / HTTP/1.0\r\nHost: example.com\r\nConnection: Keep-Alive\r\n\r\n",
		"GET / HTTP/1.0\r\nHost: example.com\r\n\r\n",
	} {
		resp := serveRaw(t, unbufferedHandler("hello world", -1), req)

		if strings.Contains(resp, "chunked") {
			t.Fatalf("HTTP/1.0 must never be sent chunked (RFC 9112 §7.1), got:\n%s", resp)
		}
		if !strings.Contains(resp, "Connection: close") {
			t.Fatalf("a body of unknown length needs Connection: close to be delimited, got:\n%s", resp)
		}
		if !strings.HasSuffix(resp, "\r\n\r\nhello world") {
			t.Fatalf("expecting the raw body after the headers, got:\n%s", resp)
		}
	}
}

func TestUnbufferedHTTP10KnownLengthKeepsAlive(t *testing.T) {
	t.Parallel()

	body := "hello world"
	resp := serveRaw(t, unbufferedHandler(body, len(body)),
		"GET /first HTTP/1.0\r\nHost: example.com\r\nConnection: Keep-Alive\r\n\r\n",
		"GET /second HTTP/1.0\r\nHost: example.com\r\nConnection: Keep-Alive\r\n\r\n")

	if !strings.Contains(resp, "Connection: keep-alive") {
		t.Fatalf("an HTTP/1.0 client is only told the connection survives by the header, got:\n%s", resp)
	}
	if strings.Contains(resp, "Connection: close") {
		t.Fatalf("a delimited response to a keep-alive request must not close, got:\n%s", resp)
	}
	if n := strings.Count(resp, "HTTP/1.1 200"); n != 2 {
		t.Fatalf("expecting both requests answered on one connection, got %d:\n%s", n, resp)
	}
}

func TestUnbufferedHeadHasNoBody(t *testing.T) {
	t.Parallel()

	for _, req := range []string{
		"HEAD / HTTP/1.1\r\nHost: example.com\r\n\r\n",
		"HEAD / HTTP/1.0\r\nHost: example.com\r\n\r\n",
	} {
		resp := serveRaw(t, unbufferedHandler("hello world", -1), req)

		if strings.Contains(resp, "hello world") {
			t.Fatalf("a HEAD response must not carry a body (RFC 9110 §9.3.2), got:\n%s", resp)
		}
		if strings.Contains(resp, "chunked") {
			t.Fatalf("there is no body to frame in a HEAD response, got:\n%s", resp)
		}
		// Content-Length: 0 would misreport what the GET returns, so with an
		// unknown size the header is simply absent.
		if strings.Contains(resp, "Content-Length: 0") {
			t.Fatalf("expecting no Content-Length rather than a false zero, got:\n%s", resp)
		}
	}
}

func TestUnbufferedHeadKeepsKnownContentLength(t *testing.T) {
	t.Parallel()

	body := "hello world"
	resp := serveRaw(t, unbufferedHandler(body, len(body)),
		"HEAD / HTTP/1.1\r\nHost: example.com\r\n\r\n")

	if !strings.Contains(resp, "Content-Length: 11") {
		t.Fatalf("a HEAD response carries the headers its GET would have, got:\n%s", resp)
	}
	if strings.Contains(resp, body) {
		t.Fatalf("a HEAD response must not carry a body, got:\n%s", resp)
	}
}

func TestUnbufferedEmptyResponseIsZeroLength(t *testing.T) {
	t.Parallel()

	// A handler that writes no body at all (a redirect, a bare status) still
	// gets Content-Length: 0 — that one is true for the GET as well.
	h := func(ctx *RequestCtx) {
		ctx.DisableBuffering()
		defer ctx.CloseResponse() //nolint:errcheck
		ctx.SetStatusCode(StatusMovedPermanently)
		ctx.Response.Header.Set("Location", "https://example.com/")
	}

	for _, req := range []string{
		"GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
		"GET / HTTP/1.0\r\nHost: example.com\r\nConnection: Keep-Alive\r\n\r\n",
	} {
		resp := serveRaw(t, h, req)
		if !strings.Contains(resp, "Content-Length: 0") {
			t.Fatalf("expecting an explicit zero length, got:\n%s", resp)
		}
		if strings.Contains(resp, "chunked") {
			t.Fatalf("an empty body needs no framing, got:\n%s", resp)
		}
	}
}
