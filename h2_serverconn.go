package fns

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/pablolagos/fns/internal/frames"
	"github.com/pablolagos/fns/internal/hpack"
)

// h2ServerConn represents a single HTTP/2 connection
type h2ServerConn struct {
	conn            net.Conn
	settings        Settings
	streamManager   *StreamManager
	flowWindow      int32
	mu              sync.Mutex
	encoder         *hpack.Encoder
	decoder         *hpack.Decoder
	streamProcessor *StreamProcessor
	s               *Server
}

// Serve handles the HTTP/2 connection
func (sc *h2ServerConn) Serve() error {
	IncrementConnections()
	defer DecrementConnections()

	sc.encoder = hpack.NewEncoder()
	sc.decoder = hpack.NewDecoder()
	sc.streamProcessor = NewStreamProcessor()

	// Send initial SETTINGS frame
	if err := sc.sendInitialSettings(); err != nil {
		sc.handleError(err, 0, frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
		return err
	}

	// Initialize stream manager
	sc.streamManager = NewStreamManager()
	sc.flowWindow = DefaultInitialWindowSize

	// Main loop to handle frames
	for {
		frame, err := frames.ReadFrame(sc.conn)
		if err != nil {
			sc.handleError(err, 0, frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
			return err
		}

		// Handle the frame based on its type
		switch frame.Type {
		case frames.FrameData:
			sc.handleDataFrame(frame)
		case frames.FrameHeaders, frames.FrameContinuation:
			sc.handleHeadersFrame(frame)
		case frames.FrameSettings:
			sc.handleSettingsFrame(frame)
		case frames.FramePing:
			sc.handlePingFrame(frame)
		case frames.FrameGoAway:
			sc.handleGoAwayFrame(frame)
			return nil
		case frames.FrameWindowUpdate:
			sc.handleWindowUpdateFrame(frame)
		case frames.FrameRSTStream:
			sc.handleRSTStreamFrame(frame)
		case frames.FramePriority:
			sc.handlePriorityFrame(frame)
		case frames.FramePushPromise:
			sc.handlePushPromiseFrame(frame)
		default:
			sc.handleError(fmt.Errorf("unhandled frame type: %v", frame.Type), 0, frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
			return fmt.Errorf("unhandled frame type: %v", frame.Type)
		}

		// Release the frame after handling it
		frames.ReleaseFrame(frame)
	}
}

// handleError handles errors by logging, sending appropriate frames, and closing the connection if necessary
func (sc *h2ServerConn) handleError(err error, streamID uint32, frameType uint8, errorCode uint32) {
	log.Println("Error:", err)
	if frameType == frames.FrameGoAway {
		sc.sendGoAway(streamID, errorCode)
		sc.closeConnection()
	} else if frameType == frames.FrameRSTStream {
		sc.sendRSTStream(streamID, errorCode)
	}
}

// sendInitialSettings performs the HTTP/2 initial settings exchange
func (sc *h2ServerConn) sendInitialSettings() error {
	// Send initial SETTINGS frame
	if err := sendSettings(sc.conn, sc.settings); err != nil {
		return err
	}

	// Receive SETTINGS frame from client
	frame, err := frames.ReadFrame(sc.conn)
	if err != nil {
		return err
	}

	if frame.Type != frames.FrameSettings {
		return fmt.Errorf("expected SETTINGS frame, got %v", frame.Type)
	}

	// Apply the received settings
	applySettings(frame, &sc.settings)

	// Send SETTINGS ACK
	return sendSettingsAck(sc.conn)
}

// handleSettingsFrame handles SETTINGS frames
func (sc *h2ServerConn) handleSettingsFrame(frame *frames.Frame) {
	// Apply the received settings
	applySettings(frame, &sc.settings)

	// Send SETTINGS ACK
	if err := sendSettingsAck(sc.conn); err != nil {
		sc.handleError(err, 0, frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
	}
}

// handleHeadersFrame handles HEADERS and CONTINUATION frames
func (sc *h2ServerConn) handleHeadersFrame(frame *frames.Frame) {
	// Create or update the stream
	stream, exists := sc.streamManager.GetStream(frame.StreamID)
	if !exists {
		stream = sc.streamManager.CreateStream(frame.StreamID, sc)
	}

	// Process the headers
	sc.processHeadersFrame(stream, frame)
}

func (sc *h2ServerConn) processHeadersFrame(stream *Stream, frame *frames.Frame) {
	stream.mu.Lock()
	defer stream.mu.Unlock()

	// Append headers data
	stream.Body = append(stream.Body, frame.Body...)

	// Check for END_HEADERS flag
	if frame.Flags&frames.FlagEndHeaders != 0 {
		// END_HEADERS flag is set, headers are complete
		log.Printf("Received complete headers for stream %d\n", stream.ID)
		headerFields, err := sc.decoder.Decode(stream.Body)
		if err != nil {
			sc.handleError(err, stream.ID, frames.FrameRSTStream, 0x1) // PROTOCOL_ERROR
			return
		}
		stream.Headers = headerFields
		// Reset the body buffer after processing headers
		stream.Body = nil
		// If the stream does not have a body or has received the END_STREAM flag, process it
		if frame.Flags&frames.FlagEndStream != 0 {
			stream.State = StreamHalfClosedRemote
			sc.streamProcessor.ProcessStream(stream, sc.s)
		} else if stream.State == StreamOpen {
			stream.State = StreamHalfClosedLocal
		}
	}
}

// handleWindowUpdateFrame handles WINDOW_UPDATE frames
func (sc *h2ServerConn) handleWindowUpdateFrame(frame *frames.Frame) {
	// Update the flow control window
	if frame.StreamID == 0 {
		// Connection-level window update
		delta := int32(binary.BigEndian.Uint32(frame.Body))
		sc.flowWindow += delta
		if sc.flowWindow < 0 {
			sc.handleError(fmt.Errorf("flow control error"), 0, frames.FrameGoAway, 0x3) // FLOW_CONTROL_ERROR
		}
	} else {
		// Stream-level window update
		stream, exists := sc.streamManager.GetStream(frame.StreamID)
		if !exists {
			sc.handleError(fmt.Errorf("stream closed"), frame.StreamID, frames.FrameRSTStream, 0x5) // STREAM_CLOSED
			return
		}
		stream.AdjustWindow(int32(binary.BigEndian.Uint32(frame.Body)))
	}
}

// handleDataFrame handles DATA frames
func (sc *h2ServerConn) handleDataFrame(frame *frames.Frame) {
	// Retrieve the stream
	stream, exists := sc.streamManager.GetStream(frame.StreamID)
	if !exists {
		sc.handleError(fmt.Errorf("stream closed"), frame.StreamID, frames.FrameRSTStream, 0x5) // Error code: STREAM_CLOSED
		return
	}

	// Process the data
	stream.mu.Lock()
	defer stream.mu.Unlock()

	// Adjust the flow control window
	sc.flowWindow -= int32(len(frame.Body))
	stream.Window -= int32(len(frame.Body))

	if sc.flowWindow < 0 || stream.Window < 0 {
		// Handle window underflow
		sc.handleWindowUnderflow(stream)
		return
	}

	// Append data to stream body
	stream.Body = append(stream.Body, frame.Body...)

	// Check for END_STREAM flag
	if frame.Flags&frames.FlagEndStream != 0 {
		stream.State = StreamHalfClosedRemote
		sc.streamProcessor.ProcessStream(stream, sc.s)
	}
}

// handleRSTStreamFrame handles RST_STREAM frames
func (sc *h2ServerConn) handleRSTStreamFrame(frame *frames.Frame) {
	// Log and close the stream
	log.Printf("Received RST_STREAM frame for stream %d\n", frame.StreamID)
	sc.streamManager.RemoveStream(frame.StreamID)
}

// handlePriorityFrame handles PRIORITY frames
func (sc *h2ServerConn) handlePriorityFrame(frame *frames.Frame) {
	// PRIORITY frames are used to change the priority of a stream
	log.Printf("Received PRIORITY frame for stream %d\n", frame.StreamID)
	stream, exists := sc.streamManager.GetStream(frame.StreamID)
	if !exists {
		sc.handleError(fmt.Errorf("stream closed"), frame.StreamID, frames.FrameRSTStream, 0x5) // STREAM_CLOSED
		return
	}

	// Parse the priority value from the frame body
	if len(frame.Body) < 5 {
		sc.handleError(fmt.Errorf("priority frame body too short"), 0, frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
		return
	}
	priority := frame.Body[0]
	stream.UpdatePriority(priority)
}

// handlePushPromiseFrame handles PUSH_PROMISE frames
func (sc *h2ServerConn) handlePushPromiseFrame(frame *frames.Frame) {
	// PUSH_PROMISE frames are used to initiate server push
	log.Printf("Received PUSH_PROMISE frame, initiating server push\n")

	// Parse the PUSH_PROMISE frame and log the promised stream ID
	if len(frame.Body) < 4 {
		sc.handleError(fmt.Errorf("PUSH_PROMISE frame body too short"), 0, frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
		return
	}
	promisedStreamID := binary.BigEndian.Uint32(frame.Body[:4])
	log.Printf("Promised Stream ID: %d\n", promisedStreamID)

	// Create the promised stream
	sc.streamManager.CreateStream(promisedStreamID, sc)
}

// handlePingFrame handles PING frames
func (sc *h2ServerConn) handlePingFrame(frame *frames.Frame) {
	// Respond with PING ACK
	frame.Flags |= frames.FlagAck // ACK flag
	if err := frame.WriteTo(sc.conn); err != nil {
		sc.handleError(err, 0, frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
	}
}

// handleGoAwayFrame handles GOAWAY frames
func (sc *h2ServerConn) handleGoAwayFrame(frame *frames.Frame) {
	// Log and close the connection
	log.Printf("Received GOAWAY frame, closing connection\n")
	sc.closeConnection()
}

// handleWindowUnderflow handles flow control window underflow
func (sc *h2ServerConn) handleWindowUnderflow(stream *Stream) {
	log.Printf("Flow control window underflow for stream %d\n", stream.ID)
	// Send RST_STREAM for the affected stream
	sc.sendRSTStream(stream.ID, 0x3) // Error code: FLOW_CONTROL_ERROR
	sc.streamManager.RemoveStream(stream.ID)
}

// sendRSTStream sends a RST_STREAM frame
func (sc *h2ServerConn) sendRSTStream(streamID uint32, errorCode uint32) {
	frame := frames.AcquireFrame(frames.FrameRSTStream)
	defer frames.ReleaseFrame(frame)
	frame.StreamID = streamID
	frame.Body = make([]byte, 4)
	binary.BigEndian.PutUint32(frame.Body, errorCode)
	if err := frame.WriteTo(sc.conn); err != nil {
		log.Println("Error sending RST_STREAM frame:", err)
	}
}

// sendGoAway sends a GOAWAY frame
func (sc *h2ServerConn) sendGoAway(lastStreamID uint32, errorCode uint32) {
	frame := frames.AcquireFrame(frames.FrameGoAway)
	defer frames.ReleaseFrame(frame)
	frame.Body = make([]byte, 8)
	binary.BigEndian.PutUint32(frame.Body[:4], lastStreamID)
	binary.BigEndian.PutUint32(frame.Body[4:], errorCode)
	if err := frame.WriteTo(sc.conn); err != nil {
		log.Println("Error sending GOAWAY frame:", err)
	}
}

// closeConnection closes the connection and releases resources
func (sc *h2ServerConn) closeConnection() {
	// Close the connection
	sc.conn.Close()

	// Clean up resources
	for current := sc.streamManager.head; current != nil; current = current.next {
		sc.streamManager.RemoveStream(current.ID)
	}
	log.Println("Connection closed and resources released")
}

// sendSettings sends a SETTINGS frame
func sendSettings(conn net.Conn, settings Settings) error {
	frame := frames.AcquireFrame(frames.FrameSettings)
	defer frames.ReleaseFrame(frame)
	// Fill the frame body with the settings
	frame.Body = make([]byte, 6*settings.Count())
	offset := 0
	for id, value := range settings.Values() {
		binary.BigEndian.PutUint16(frame.Body[offset:offset+2], id)
		binary.BigEndian.PutUint32(frame.Body[offset+2:offset+6], value)
		offset += 6
	}
	return frame.WriteTo(conn)
}

// applySettings applies the received settings
func applySettings(frame *frames.Frame, settings *Settings) {
	offset := 0
	for offset < len(frame.Body) {
		id := binary.BigEndian.Uint16(frame.Body[offset : offset+2])
		value := binary.BigEndian.Uint32(frame.Body[offset+2 : offset+6])
		settings.Set(id, value)
		offset += 6
	}
}

// sendSettingsAck sends a SETTINGS ACK frame
func sendSettingsAck(conn net.Conn) error {
	frame := frames.AcquireFrame(frames.FrameSettings)
	defer frames.ReleaseFrame(frame)
	frame.Flags = frames.FlagAck // ACK flag
	return frame.WriteTo(conn)
}
