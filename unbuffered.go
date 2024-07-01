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

	// Write body. In chunks if content length is not set.
	if uw.ctx.Response.Header.contentLength == -1 && uw.ctx.Response.Header.IsHTTP11() {
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
		if uw.ctx.Response.Header.contentLength == 0 && uw.ctx.Response.Header.IsHTTP11() {
			if uw.ctx.Response.SkipBody {
				uw.ctx.Response.Header.SetContentLength(0)
			} else {
				uw.ctx.Response.Header.SetContentLength(-1) // means Transfer-Encoding = chunked
			}
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

	// finalize chunks
	if uw.bodyChunkStarted && uw.ctx.Response.Header.IsHTTP11() && !uw.bodyLastChunkSent {
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
