//go:build h2

package fns

import (
	"sync"
)

// StreamManager manages active streams using a priority queue
type StreamManager struct {
	mu    sync.Mutex
	head  *h2Stream
	tail  *h2Stream
	count int
}

// NewStreamManager creates a new StreamManager
func NewStreamManager() *StreamManager {
	return &StreamManager{}
}

// GetStream returns a stream by id or nil if it does not exist
func (sm *StreamManager) GetStream(streamID uint32) *h2Stream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for stream := sm.head; stream != nil; stream = stream.next {
		if stream.id == streamID {
			return stream
		}
	}

	return nil
}

// CreateStream creates a new stream and adds it to the manager
func (sm *StreamManager) CreateStream(streamID uint32, conn *h2ServerConn) *h2Stream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	stream := acquireStream(streamID, conn)

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

// SetStreamPriority sets the priority of a stream.
// If the stream does not exist, it creates an idle stream with the given priority.
func (sm *StreamManager) SetStreamPriority(streamID uint32, priority int32, conn *h2ServerConn) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Find the stream by id and update its priority
	for stream := sm.head; stream != nil; stream = stream.next {
		if stream.id == streamID {
			sm.updateStreamPriority(stream, priority)
			return
		}
	}

	// If the stream is not found, create a new stream with the given priority
	stream := acquireStream(streamID, conn)
	stream.id = streamID
	stream.priority.Store(priority)

	if sm.tail == nil {
		sm.head = stream
		sm.tail = stream
	} else {
		sm.tail.next = stream
		stream.prev = sm.tail
		sm.tail = stream
	}

	sm.reorderStream(stream)
}

// SetStreamState sets the state of a stream. Also, a stream state can be directly set in the sream, directly.
func (sm *StreamManager) SetStreamState(streamID uint32, state StreamState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var stream *h2Stream
	// Find the stream by id and update its state
	for stream = sm.head; stream != nil; stream = stream.next {
		if stream.id == streamID {
			stream.state = state
		}
	}
}

// GetReadyStreams returns up to n streams that are ready for processing, respecting priority and dependencies
func (sm *StreamManager) GetReadyStreams(n int) []*h2Stream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var readyStreams []*h2Stream
	var toProcess []*h2Stream

	// Collect streams that are ready for processing (Open or Half-Closed Local)
	for current := sm.head; current != nil; current = current.next {
		if current.state == StreamHalfClosedRemote {
			readyStreams = append(readyStreams, current)
		}
	}

	// Process these streams, ensuring parents are processed before children
	for len(readyStreams) > 0 && len(toProcess) < n {
		current := readyStreams[0]
		readyStreams = readyStreams[1:]

		// Check if the stream's parent has been processed
		if current.prev != nil && !sm.isParentProcessed(current) {
			// Re-add the stream at the end of the ready list if its parent hasn't been processed
			readyStreams = append(readyStreams, current)
		} else {
			// Add the stream to the list to be processed
			toProcess = append(toProcess, current)
		}
	}

	return toProcess
}

// isParentProcessed checks if the parent of the stream has been processed
func (sm *StreamManager) isParentProcessed(stream *h2Stream) bool {
	parent := stream.prev
	if parent == nil {
		return true // No parent means no dependency
	}
	// The parent is considered "processed" if it is in a state where it has finished processing
	return parent.state >= StreamProcessed
}

// updateStreamPriority updates the priority of a stream and reorders the list if necessary
func (sm *StreamManager) updateStreamPriority(stream *h2Stream, newPriority int32) {
	stream.priority.Store(newPriority)
	sm.reorderStream(stream)
}

// reorderStream reorders a stream in the list based on its new priority
func (sm *StreamManager) reorderStream(stream *h2Stream) {
	// If stream is at the head or tail, there's no need to reorder
	if stream == sm.head || stream == sm.tail {
		return
	}

	// Remove the stream from its current position
	if stream.prev != nil {
		stream.prev.next = stream.next
	}
	if stream.next != nil {
		stream.next.prev = stream.prev
	}

	// Re-insert the stream in the correct position based on priority
	current := sm.head
	for current != nil && current.priority.Load() <= stream.priority.Load() {
		current = current.next
	}

	if current == nil {
		// Insert at the tail
		sm.tail.next = stream
		stream.prev = sm.tail
		stream.next = nil
		sm.tail = stream
	} else if current == sm.head {
		// Insert at the head
		stream.next = sm.head
		sm.head.prev = stream
		sm.head = stream
		stream.prev = nil
	} else {
		// Insert in the middle
		stream.prev = current.prev
		stream.next = current
		if current.prev != nil {
			current.prev.next = stream
		}
		current.prev = stream
	}
}

// removeStreamNode removes a stream node from the list
func (sm *StreamManager) removeStreamNode(stream *h2Stream) {
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

	stream.next = nil
	stream.prev = nil
}
