package fns

import (
	"log"
	"sort"
	"sync"

	"github.com/pablolagos/fns/internal/hpack"
)

// StreamState represents the state of a stream
type StreamState int

const (
	StreamIdle StreamState = iota
	StreamOpen
	StreamHalfClosedLocal
	StreamHalfClosedRemote
	StreamClosed
)

// Stream represents an HTTP/2 stream
type Stream struct {
	ID              uint32
	State           StreamState
	Body            []byte
	Window          int32
	Priority        uint8
	Headers         []hpack.HeaderField
	ResponseHeaders []hpack.HeaderField
	ResponseBody    []byte
	next            *Stream
	prev            *Stream
	mu              sync.Mutex
	conn            *h2ServerConn
}

// AdjustWindow adjusts the flow control window for the stream
func (s *Stream) AdjustWindow(delta int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Window += delta
	if s.Window < 0 {
		// Handle window underflow
		s.conn.handleWindowUnderflow(s)
	}
}

// UpdatePriority updates the priority of the stream
func (s *Stream) UpdatePriority(priority uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Priority = priority
}

// StreamManager manages active streams
type StreamManager struct {
	mu    sync.Mutex
	head  *Stream
	tail  *Stream
	count int
}

// NewStreamManager creates a new StreamManager
func NewStreamManager() *StreamManager {
	return &StreamManager{}
}

// CreateStream creates a new stream and adds it to the manager
func (sm *StreamManager) CreateStream(streamID uint32, conn *h2ServerConn) *Stream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	stream := &Stream{
		ID:   streamID,
		conn: conn,
	}

	if sm.tail == nil {
		sm.head = stream
		sm.tail = stream
	} else {
		sm.tail.next = stream
		stream.prev = sm.tail
		sm.tail = stream
	}

	sm.count++
	return stream
}

// GetStream retrieves a stream by its ID
func (sm *StreamManager) GetStream(streamID uint32) (*Stream, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for stream := sm.head; stream != nil; stream = stream.next {
		if stream.ID == streamID {
			return stream, true
		}
	}
	return nil, false
}

// RemoveStream removes a stream by its ID
func (sm *StreamManager) RemoveStream(streamID uint32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for stream := sm.head; stream != nil; stream = stream.next {
		if stream.ID == streamID {
			if stream.prev != nil {
				stream.prev.next = stream.next
			} else {
				sm.head = stream.next
			}

			if stream.next != nil {
				stream.next.prev = stream.prev
			} else {
				sm.tail = stream.prev
			}

			sm.count--
			break
		}
	}
}

// UpdateStreamState updates the state of a stream
func (sm *StreamManager) UpdateStreamState(id uint32, state StreamState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for current := sm.head; current != nil; current = current.next {
		if current.ID == id {
			current.State = state
			return
		}
	}
}

// ScheduleStreams schedules the streams based on their priority
func (sm *StreamManager) ScheduleStreams() []*Stream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Gather all streams and sort by priority
	var streams []*Stream
	for current := sm.head; current != nil; current = current.next {
		streams = append(streams, current)
	}

	// Sort streams by priority
	sort.Slice(streams, func(i, j int) bool {
		return streams[i].Priority < streams[j].Priority
	})

	return streams
}

// ProcessStreams processes the streams based on their scheduled order
func (sm *StreamManager) ProcessStreams() {
	streams := sm.ScheduleStreams()

	for _, stream := range streams {
		// Process each stream based on its priority
		log.Printf("Processing stream %d with priority %d\n", stream.ID, stream.Priority)
		// Here you would add the logic to handle the actual stream data,
		// for example, reading data from the stream, writing data to the stream,
		// handling flow control, etc.
	}
}

// Count returns the number of active streams
func (sm *StreamManager) Count() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.count
}
