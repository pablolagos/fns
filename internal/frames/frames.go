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
	length    uint32
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

// AcquireFrame retrieves a frame from the pool
func AcquireFrame(frameType int) *Frame {
	frame := framePool.Get().(*Frame)
	frame.Type = uint8(frameType)
	frame.length = 0
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
	if frame.Body != nil {
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
	f.length = binary.BigEndian.Uint32(f.rawHeader[:4]) & 0x00FFFFFF
	f.Type = f.rawHeader[3]
	f.Flags = f.rawHeader[4]
	f.StreamID = binary.BigEndian.Uint32(f.rawHeader[5:9]) & 0x7FFFFFFF

	// Read the body
	if f.length > 0 {
		if cap(f.Body) < int(f.length) {
			// If we modify the capacity of the slice, the os will allocate a new array
			// When released, the old array will be saved and the new one will be garbage collected
			f.Body = make([]byte, f.length)
		} else {
			f.Body = f.Body[:f.length]
		}
		if _, err := io.ReadFull(conn, f.Body); err != nil {
			return nil, fmt.Errorf("error reading frame body: %v", err)
		}
	}

	return f, nil
}

// WriteTo writes a frame to the connection
func (f *Frame) WriteTo(conn net.Conn) error {
	if int(f.length) > len(f.Body) {
		return fmt.Errorf("frame length is greater than body length")
	}
	// Prepare the header
	binary.BigEndian.PutUint32(f.rawHeader[:4], f.length)
	f.rawHeader[3] = f.Type
	f.rawHeader[4] = f.Flags
	binary.BigEndian.PutUint32(f.rawHeader[5:9], f.StreamID)

	// Write the header
	if _, err := conn.Write(f.rawHeader[:]); err != nil {
		return err
	}

	// Write the body
	if f.length > 0 {
		if _, err := conn.Write(f.Body[:f.length]); err != nil {
			return err
		}
	}

	return nil
}

func (f *Frame) Length() uint32 {
	return f.length
}
