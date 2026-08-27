package fns

import (
	"errors"
	"strings"
	"testing"
)

// Flushing an unbuffered response.
//
// These tests measure WHEN bytes reach the connection, not what they are, so
// they read the fake conn from inside the handler rather than after it returns.
//
// What was already true before Flush existed, and is worth knowing before
// reading further: the CHUNKED path already reaches the connection on every
// write, because writeChunk ends in w.Flush(). An HTTP/1.1 response of unknown
// length — which is what every stream is — therefore streamed all along. Flush
// is for the two cases that did not:
//
//   - a response whose length IS known, written in pieces;
//   - the layer BELOW the bufio.Writer. On HTTP/2 and HTTP/3 ctx.c is not a
//     socket but a wrapper over a response writer that holds bytes of its own,
//     and emptying our buffer into it moves the stall one layer down rather
//     than ending it. That is the case TestTheConnectionUnderneathIsFlushedToo
//     covers, and it is the one that matters in production.

// flushProbe runs a handler and hands it a way to look at what the connection
// has actually received so far.
func flushProbe(t *testing.T, handler func(ctx *RequestCtx, onWire func() string)) string {
	t.Helper()

	rw := &readWriter{}
	rw.r.WriteString("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")

	s := &Server{Handler: func(ctx *RequestCtx) {
		handler(ctx, func() string { return rw.w.String() })
	}}
	if err := s.ServeConn(rw); err != nil {
		t.Fatalf("unexpected error from ServeConn: %v", err)
	}
	return rw.w.String()
}

// TestChunkedWritesAlreadyReachTheConnection is here to keep a wrong belief
// from coming back.
//
// It looks like a test of nothing, and that is the point: an unbuffered
// chunked response has NEVER needed an explicit flush, because writeChunk ends
// in one. Anybody about to add flushing "so that server-sent events work over
// HTTP/1.1" should read this first and go one layer down instead.
func TestChunkedWritesAlreadyReachTheConnection(t *testing.T) {
	t.Parallel()

	var afterWrite string
	flushProbe(t, func(ctx *RequestCtx, onWire func() string) {
		ctx.DisableBuffering()
		defer ctx.CloseResponse() //nolint:errcheck
		ctx.SetStatusCode(StatusOK)
		ctx.SetContentType("text/event-stream")

		_, _ = ctx.WriteString("data: one\n\n")
		afterWrite = onWire()
	})

	if !strings.Contains(afterWrite, "data: one") {
		t.Fatalf("a chunked write did not reach the connection; writeChunk no longer flushes:\n%q", afterWrite)
	}
}

// TestAFlushedPieceReachesTheConnectionBeforeTheResponseEnds covers the case
// that genuinely stalls: a response whose length is known, written in pieces.
//
// There is no chunk framing to flush behind it, so the bytes sit in the
// bufio.Writer until Close — which for anything that keeps the response open is
// "never" as far as the client is concerned. The control is the same handler
// without the flush, and it is what makes the assertion mean anything.
func TestAFlushedPieceReachesTheConnectionBeforeTheResponseEnds(t *testing.T) {
	t.Parallel()

	const first = "first piece"
	const second = "second piece"

	var afterFlush string
	full := flushProbe(t, func(ctx *RequestCtx, onWire func() string) {
		ctx.DisableBuffering()
		defer ctx.CloseResponse() //nolint:errcheck
		ctx.SetStatusCode(StatusOK)
		ctx.SetContentType("text/plain")
		ctx.Response.Header.SetContentLength(len(first) + len(second))

		_, _ = ctx.WriteString(first)
		if err := ctx.Flush(); err != nil {
			t.Errorf("Flush: %v", err)
		}
		afterFlush = onWire()

		_, _ = ctx.WriteString(second)
	})

	if !strings.Contains(afterFlush, first) {
		t.Fatalf("the flushed piece had not reached the connection:\n%q", afterFlush)
	}
	if strings.Contains(afterFlush, second) {
		t.Fatalf("a piece written after the flush was already on the wire:\n%q", afterFlush)
	}
	if !strings.Contains(full, second) {
		t.Fatalf("the second piece never arrived:\n%q", full)
	}

	var afterWriteOnly string
	flushProbe(t, func(ctx *RequestCtx, onWire func() string) {
		ctx.DisableBuffering()
		defer ctx.CloseResponse() //nolint:errcheck
		ctx.SetStatusCode(StatusOK)
		ctx.SetContentType("text/plain")
		ctx.Response.Header.SetContentLength(len(first) + len(second))

		_, _ = ctx.WriteString(first)
		afterWriteOnly = onWire()
	})
	if strings.Contains(afterWriteOnly, first) {
		t.Fatal("an unflushed piece was already on the connection — this test cannot tell a flush from a write")
	}
}

// TestFlushingBeforeAnyBodySendsTheHeaders covers the handler that wants the
// client to know the stream has opened before it has anything to say. The
// framing decision the first Write would have made is made here instead.
func TestFlushingBeforeAnyBodySendsTheHeaders(t *testing.T) {
	t.Parallel()

	var afterFlush string
	flushProbe(t, func(ctx *RequestCtx, onWire func() string) {
		ctx.DisableBuffering()
		defer ctx.CloseResponse() //nolint:errcheck
		ctx.SetStatusCode(StatusOK)
		ctx.SetContentType("text/event-stream")

		if err := ctx.Flush(); err != nil {
			t.Errorf("Flush: %v", err)
		}
		afterFlush = onWire()
	})

	if !strings.Contains(afterFlush, "200 OK") {
		t.Fatalf("the status line had not been sent:\n%q", afterFlush)
	}
	if !strings.Contains(afterFlush, "Transfer-Encoding: chunked") {
		t.Fatalf("an unknown-length HTTP/1.1 response was not framed as chunked:\n%q", afterFlush)
	}
}

// TestFlushingABufferedResponseIsAnError keeps a bug visible. A handler
// streaming into a buffered response is not slow, it is wrong — the body is a
// buffer that goes out when the handler returns — and answering nil would hide
// that for as long as nobody looked at the timing.
func TestFlushingABufferedResponseIsAnError(t *testing.T) {
	t.Parallel()

	flushProbe(t, func(ctx *RequestCtx, _ func() string) {
		ctx.SetStatusCode(StatusOK)
		_, _ = ctx.WriteString("hello")
		if err := ctx.Flush(); !errors.Is(err, ErrNotUnbuffered) {
			t.Errorf("Flush on a buffered response answered %v, want ErrNotUnbuffered", err)
		}
	})
}

// flushingConn is a connection that records being flushed, the way a net.Conn
// standing in for an HTTP/2 stream does.
type flushingConn struct {
	readWriter
	flushes int
}

func (c *flushingConn) Flush() error {
	c.flushes++
	return nil
}

// TestTheConnectionUnderneathIsFlushedToo is the half that decides whether any
// of this works over HTTP/2 and HTTP/3, and it is the reason Flush exists.
//
// On those transports ctx.c is not a socket: it is a wrapper over a response
// writer that holds bytes of its own until it is told otherwise. writeChunk's
// flush empties OUR buffer into that wrapper and stops there, which moves the
// stall one layer down and changes nothing the client can see.
func TestTheConnectionUnderneathIsFlushedToo(t *testing.T) {
	t.Parallel()

	conn := &flushingConn{}
	conn.r.WriteString("GET /events HTTP/1.1\r\nHost: example.com\r\n\r\n")

	s := &Server{Handler: func(ctx *RequestCtx) {
		ctx.DisableBuffering()
		defer ctx.CloseResponse() //nolint:errcheck
		ctx.SetStatusCode(StatusOK)
		_, _ = ctx.WriteString("data: one\n\n")
		if err := ctx.Flush(); err != nil {
			t.Errorf("Flush: %v", err)
		}
	}}
	if err := s.ServeConn(conn); err != nil {
		t.Fatalf("unexpected error from ServeConn: %v", err)
	}

	if conn.flushes == 0 {
		t.Fatal("the connection under the unbuffered writer was never flushed")
	}
}
