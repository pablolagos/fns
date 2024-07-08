package frames

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

// Frame types
const (
	FrameData         = 0x0
	FrameHeaders      = 0x1
	FramePriority     = 0x2
	FrameRSTStream    = 0x3
	FrameSettings     = 0x4
	FramePushPromise  = 0x5
	FramePing         = 0x6
	FrameGoAway       = 0x7
	FrameWindowUpdate = 0x8
	FrameContinuation = 0x9

	// Default body size
	DefaultFrameBodySize = 16384
)

// Frame flags
const (
	FlagEndStream  = 0x1
	FlagEndHeaders = 0x4
	FlagAck        = 0x1
)

// Frame represents an HTTP/2 frame
type Frame struct {
	rawHeader [9]byte
	Type      uint8
	Flags     uint8
	StreamID  uint32
	Body      []byte
}

// sync.Pool for frames
var framePool = sync.Pool{
	New: func() interface{} {
		return &Frame{
			Body: make([]byte, 0, DefaultFrameBodySize),
		}
	},
}

// AcquireFrame retrieves a frame from the pool. All frames have an empty body allocated, with the default cap size.
func AcquireFrame(frameType int) *Frame {
	frame := framePool.Get().(*Frame)
	frame.Type = uint8(frameType)
	frame.StreamID = 0
	frame.Flags = 0
	if len(frame.Body) > 0 {
		frame.Body = frame.Body[:0]
	}
	return frame
}

// ReleaseFrame returns a frame to the pool if it has an allocated body. Otherwise, it is discarded
func ReleaseFrame(frame *Frame) {
	// Only recycle frames with allocated bodies
	if cap(frame.Body) == DefaultFrameBodySize {
		framePool.Put(frame)
	}
}

// ReadFrame acquires a frame and reads from the connection
func ReadFrame(conn net.Conn) (*Frame, error) {
	// Allocate a new frame with body
	f := AcquireFrame(FrameHeaders)

	// Read the header
	if _, err := io.ReadFull(conn, f.rawHeader[:]); err != nil {
		return nil, fmt.Errorf("error reading frame header: %v", err)
	}

	// Parse the header
	length := (int(f.rawHeader[0]) << 16) | (int(f.rawHeader[1]) << 8) | int(f.rawHeader[2]) // 24 bits length
	f.Type = f.rawHeader[3]
	f.Flags = f.rawHeader[4]
	f.StreamID = binary.BigEndian.Uint32(f.rawHeader[5:9]) & 0x7FFFFFFF

	// Read the body
	if length > 0 {
		if cap(f.Body) < int(length) {
			// If we modify the capacity of the slice, the os will allocate a new array
			// When released, the old array will be saved and the new one will be garbage collected
			f.Body = make([]byte, length)
		} else {
			f.Body = f.Body[:length]
		}
		if _, err := io.ReadFull(conn, f.Body); err != nil {
			return nil, fmt.Errorf("error reading frame body: %v", err)
		}
	}

	return f, nil
}

// WriteTo writes a frame to the connection
func (f *Frame) WriteTo(conn net.Conn) error {
	// Write the length of the frame in the first 24 bits
	length := uint32(len(f.Body))
	f.rawHeader[0] = byte(length >> 16)
	f.rawHeader[1] = byte(length >> 8)
	f.rawHeader[2] = byte(length)

	// Write the type, flags and stream id
	f.rawHeader[3] = f.Type
	f.rawHeader[4] = f.Flags
	f.rawHeader[5] = byte(f.StreamID >> 24)
	f.rawHeader[6] = byte(f.StreamID >> 16)
	f.rawHeader[7] = byte(f.StreamID >> 8)
	f.rawHeader[8] = byte(f.StreamID)

	// Write the header
	if _, err := conn.Write(f.rawHeader[:]); err != nil {
		return err
	}

	// Write the body
	if len(f.Body) > 0 {
		if _, err := conn.Write(f.Body); err != nil {
			return err
		}
	}

	return nil
}

func (f *Frame) Length() uint32 {
	return uint32(len(f.Body))
}

func (f *Frame) SetACK(ack bool) {
	if ack {
		f.Flags |= FlagAck
	} else {
		f.Flags &= ^uint8(FlagAck)
	}
}
