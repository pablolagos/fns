package fns

import (
	"bufio"
	"errors"
	"fmt"
)

type UnbufferedWriter interface {
	Write(p []byte) (int, error)
	WriteHeaders() (int, error)
	Close() error
}

type unbufferedWriter struct {
	writer            *bufio.Writer
	ctx               *RequestCtx
	bodyChunkStarted  bool
	bodyLastChunkSent bool
	headersWritten    bool
}

var (
	ErrNotUnbuffered          = errors.New("not unbuffered")
	ErrClosedUnbufferedWriter = errors.New("use of closed unbuffered writer")
)

// Ensure unbufferedWriter implements UnbufferedWriter.
var _ UnbufferedWriter = &unbufferedWriter{}

// newUnbufferedWriter
//
// Object must be discarded when request is finished
func newUnbufferedWriter(ctx *RequestCtx) *unbufferedWriter {
	writer := acquireWriter(ctx)
	return &unbufferedWriter{ctx: ctx, writer: writer}
}

// skipBody reports whether this response must go out without a body. A HEAD
// request is the case the handler cannot be expected to know about: it writes
// the body it would have written for a GET, and the framing layer drops it.
func (uw *unbufferedWriter) skipBody() bool {
	return uw.ctx.Response.SkipBody || uw.ctx.IsHead()
}

func (uw *unbufferedWriter) Write(p []byte) (int, error) {
	if uw.writer == nil || uw.ctx == nil {
		return 0, ErrClosedUnbufferedWriter
	}

	// Write headers if not already sent
	if !uw.headersWritten {
		_, err := uw.WriteHeaders()
		if err != nil {
			return 0, fmt.Errorf("error writing headers: %w", err)
		}
	}

	// A response that must not carry a body (HEAD, and anything the handler
	// marked as body-less) still runs through the handler's normal write path,
	// so the bytes are accounted for and discarded here. The buffered path gets
	// this for free from Response.SkipBody, which serveConn applies after the
	// handler returns — too late for a response already on the wire.
	if uw.skipBody() {
		return len(p), nil
	}

	// Write body. In chunks if that is how the headers said it would be framed:
	// contentLength is the decision WriteHeaders already made and wrote out, so
	// it is the only thing that may be consulted here. Reading the protocol
	// version again could contradict the headers already on the wire.
	if uw.ctx.Response.Header.contentLength == -1 {
		uw.bodyChunkStarted = true
		err := writeChunk(uw.writer, p)
		if err != nil {
			return 0, err
		}
		uw.ctx.bytesSent += len(p) + 4 + countHexDigits(len(p))
		return len(p), nil
	}

	n, err := uw.writer.Write(p)
	uw.ctx.bytesSent += n

	return n, err
}

func (uw *unbufferedWriter) WriteHeaders() (int, error) {
	if uw.writer == nil || uw.ctx == nil {
		return 0, ErrClosedUnbufferedWriter
	}

	if !uw.headersWritten {
		// How a body of unknown length may be delimited is decided by the
		// version of the client asking, which lives in the request header. It
		// is deliberately not read off the response header: a proxied response
		// carries whatever version the upstream spoke (CopyTo brings it along),
		// and an origin answering HTTP/1.0 must not strip the framing owed to
		// the HTTP/1.1 client in front of us.
		if uw.ctx.Response.Header.contentLength == 0 {
			switch {
			case uw.ctx.IsHead():
				// A HEAD response carries the headers its GET would have, so
				// leave Content-Length as the handler left it: either the real
				// size it knew, or absent. Never chunked — there is no body to
				// frame, and never zero, which would be a lie about the GET.
			case uw.ctx.Response.SkipBody:
				uw.ctx.Response.Header.SetContentLength(0)
			case uw.ctx.Request.Header.IsHTTP11():
				uw.ctx.Response.Header.SetContentLength(-1) // means Transfer-Encoding = chunked
			default:
				// HTTP/1.0 has no chunked encoding (RFC 9112 §7.1: a server must
				// not send Transfer-Encoding unless the request is 1.1 or
				// later), so the only way left to delimit the body is to close
				// the connection. -2 sets Transfer-Encoding: identity and
				// Connection: close, and serveConn honours the latter.
				uw.ctx.Response.Header.SetContentLength(-2)
			}
		}

		// serveConn advertises keep-alive to HTTP/1.0 clients after the handler
		// returns, which never reaches an unbuffered response: its headers are
		// already on the wire by then. Decide it here instead, while they are
		// still ours to write.
		if !uw.ctx.Request.Header.IsHTTP11() &&
			!uw.ctx.Request.Header.ConnectionClose() &&
			!uw.ctx.Response.Header.ConnectionClose() {
			uw.ctx.Response.Header.setNonSpecial(strConnection, strKeepAlive)
		}

		n, err := uw.ctx.Response.Header.WriteTo(uw.writer)
		if err != nil {
			return 0, err
		}
		uw.ctx.bytesSent += int(n)
		uw.headersWritten = true
	}
	return 0, nil
}

func (uw *unbufferedWriter) Close() error {
	if uw.writer == nil || uw.ctx == nil {
		return ErrClosedUnbufferedWriter
	}

	// write headers if not already sent (e.g. if there is no body written)
	if !uw.headersWritten {
		// skip body, as we are closing without writing body
		uw.ctx.Response.SkipBody = true
		_, err := uw.WriteHeaders()
		if err != nil {
			return fmt.Errorf("error writing headers: %w", err)
		}
	}

	// finalize chunks. Whether the body was chunked is recorded in
	// bodyChunkStarted; once it was, the terminating chunk is owed no matter
	// what the header says now.
	if uw.bodyChunkStarted && !uw.bodyLastChunkSent {
		_, _ = uw.writer.Write([]byte("0\r\n\r\n"))
		uw.ctx.bytesSent += 5
	}
	_ = uw.writer.Flush()
	uw.bodyLastChunkSent = true
	releaseWriter(uw.ctx.s, uw.writer)
	uw.writer = nil
	uw.ctx = nil
	return nil
}
