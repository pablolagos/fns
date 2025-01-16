//go:build h2

package fns

import (
	"sync"
	"sync/atomic"

	"github.com/pablolagos/fns/internal/h2/h2_frames"
	"github.com/pablolagos/fns/internal/hpack"
)

// StreamState represents the state of a stream
type StreamState int

// The different states a stream can be in. Order is important.
const (
	// StreamIdle The stream is in the IDLE state, where no frames have been exchanged yet.
	StreamIdle StreamState = iota

	// StreamOpen The stream is in the OPEN state, where it is being used to send or receive frames between the client and server.
	// It is set when the first HEADERS frame is received.
	StreamOpen

	// StreamHalfClosedRemote The client has finished sending all frames (sent the END_STREAM), but the server still can transmit data.
	StreamHalfClosedRemote

	StreamProcessing  // The stream is being processed (not part of the HTTP/2 spec)
	StreamProcessed   // The stream has been processed (not part of the HTTP/2 spec)
	StreamDispatching // The stream is being dispatched (not part of the HTTP/2 spec)

	// StreamHalfClosedLocal The server has finished sending all frames but is still capable of receiving frames from the client.
	StreamHalfClosedLocal

	// StreamClosed The stream is closed and no more frames can be exchanged.
	StreamClosed
)

// h2Stream represents an HTTP/2 stream
type h2Stream struct {
	id         uint32
	state      StreamState
	window     atomic.Int32
	priority   atomic.Int32
	next       *h2Stream
	prev       *h2Stream
	mu         sync.Mutex
	conn       *h2ServerConn
	requestCtx *RequestCtx
	rawHeaders []byte // TODO: use a buffer pool
}

var streamPool sync.Pool

// AcquireStream retrieves a stream from the pool.
// This function is intended to be used by the StreamManager.
func acquireStream(id uint32, sc *h2ServerConn) *h2Stream {
	v := streamPool.Get()
	if v == nil {
		ctx := sc.h2Server.s.acquireCtx(nil)
		return &h2Stream{requestCtx: ctx, id: id, conn: sc}
	}
	stream := v.(*h2Stream)
	stream.id = id
	stream.conn = sc
	return stream
}

func ReleaseStream(s *h2Stream) {
	s.Reset()

	streamPool.Put(s)
}

func (s *h2Stream) Reset() {
	s.id = 0
	s.state = StreamIdle
	s.window.Store(0)
	s.priority.Store(0)
	s.requestCtx.reset()
	s.rawHeaders = s.rawHeaders[:0]
	s.next = nil
	s.prev = nil
	s.conn = nil
}

func NewStream(id uint32) *h2Stream {
	return &h2Stream{
		id: id,
	}
}

// AdjustWindow adjusts the flow control window for the stream
func (s *h2Stream) AdjustWindow(delta int32) {
	newWindow := s.window.Add(delta)
	if newWindow < 0 {
		// Handle window underflow
		s.conn.handleWindowUnderflow(s)
	}
}

// UpdatePriority updates the priority of the stream
func (s *h2Stream) UpdatePriority(priority int32) {
	s.priority.Store(priority)
}

// GetRequestCtx returns the stream's request context
func (s *h2Stream) GetRequestCtx() *RequestCtx {
	return s.requestCtx
}

// ProcessHeadersFrame processes HEADERS and CONTINUATION frames.
// It appends the headers data to the stream's raw headers buffer.
// If the END_HEADERS flag is set, it decodes the raw headers into the stream's request headers.
// Change the stream state to StreamHalfClosedRemote if the END_STREAM flag is set, or
// change state to StreamOpen if headers are still incomplete.
func (s *h2Stream) ProcessHeadersFrame(frame *h2_frames.Frame, hpackCodec *hpack.Codec) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Streams receiving headers are in the OPEN state
	s.state = StreamOpen

	// Append headers data
	s.rawHeaders = append(s.rawHeaders, frame.Body...)

	// Check for END_HEADERS flag
	if frame.Flags&h2_frames.FlagEndHeaders != 0 {
		// END_HEADERS flag is set, headers are complete
		s.conn.debug.Infof("Received complete headers for stream %d", s.id)

		// Decode the raw headers into the stream's request headers
		s.decodeRequestHeaders(hpackCodec)

		// If the stream has the END_STREAM flag, process it
		if frame.Flags&h2_frames.FlagEndStream != 0 {
			s.state = StreamHalfClosedRemote
		}
	}
}

// DecodeRequestHeaders decodes stream request body into the stream's request headers
func (s *h2Stream) decodeRequestHeaders(hpack *hpack.Codec) {
	h := &s.requestCtx.Request.Header
	h.Reset()

	// TODO: Fix request URI
	hpack.Decoder.DecodeIterate(s.rawHeaders, func(key, val []byte) {
		// map http2 header fields to http1.1 header fields
		switch string(key) {
		case ":method":
			h.SetMethodBytes(val)
		case ":path":
			h.SetRequestURIBytes(val)
		case ":scheme":
			h.SetRequestURIBytes(val)
		case ":authority":
			h.SetHostBytes(val)
		default:
			h.AddBytesKV(key, val)
		}

		h.AddBytesKV(key, val)
	})

	h.noHTTP11 = true
	h.proto = strHTTP20
}
