//go:build h2

package fns

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/pablolagos/fns/internal/h2/h2_frames"

	"github.com/pablolagos/fns/internal/debuglog"
	"github.com/pablolagos/fns/internal/hpack"
)

// Default serverSettings
var defaultServerSettings = Settings{
	headerTableSize:      ProtocolDefaultSettings[SettingHeaderTableSize],
	enablePush:           ProtocolDefaultSettings[SettingEnablePush],
	maxConcurrentStreams: 100,
	initialWindowSize:    ProtocolDefaultSettings[SettingInitialWindowSize],
	maxFrameSize:         ProtocolDefaultSettings[SettingMaxFrameSize],
	maxHeaderListSize:    100,
}

const ClientPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

var ErrInvalidPreface = errors.New("invalid client preface")

// h2ServerConn represents a single HTTP/2 connection
type h2ServerConn struct {
	h2Server        *h2Server
	netConn         net.Conn
	serverSettings  Settings
	clientSettings  Settings
	streamManager   *StreamManager
	streamProcessor *StreamProcessor
	flowWindow      int32
	mu              sync.Mutex
	hpack           *hpack.Codec
	logger          Logger
	debug           *debuglog.Logger
}

func newH2ServerConn(conn net.Conn, h2s *h2Server) *h2ServerConn {
	return &h2ServerConn{
		h2Server: h2s,
		netConn:  conn,
		logger:   h2s.conf.Logger,
		debug:    h2s.debug,
	}
}

// Serve handles the HTTP/2 connection
func (sc *h2ServerConn) Serve() error {
	sc.debug.Infof("Serving connection from %v", sc.netConn.RemoteAddr())
	IncrementConnections()
	defer func() {
		sc.debug.Infof("Closing connection for %v", sc.netConn.RemoteAddr())
		sc.netConn.Close()
		DecrementConnections()
	}()

	sc.hpack = hpack.NewCodec()
	sc.streamProcessor = NewStreamProcessor(sc.logger, sc.debug, sc.hpack)
	sc.serverSettings = defaultServerSettings
	sc.clientSettings = NewSettings()

	// Initialize stream manager
	sc.streamManager = NewStreamManager()
	sc.flowWindow = int32(sc.serverSettings.Get(SettingInitialWindowSize))

	// Send initial SETTINGS frame
	if err := sc.handshake(); err != nil {
		sc.debug.Errorf("Handshake error: %v", err)
		sc.handleError(err, 0, h2_frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
		return err
	}

	// Main loop to handle frames
	for {
		frame, err := h2_frames.ReadFrame(sc.netConn)
		if err != nil {
			if errors.Is(err, h2_frames.ErrEOF) {
				h2_frames.ReleaseFrame(frame)
				return nil
			}
			sc.handleError(err, 0, h2_frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
			return err
		}

		sc.debug.Infof("Received frame: stream %d, type %s (%x)", frame.StreamID, h2_frames.FrameTypeString[frame.Type], frame.Type)

		// Handle the frame based on its type
		switch frame.Type {
		case h2_frames.FrameData:
			sc.handleDataFrame(frame)
		case h2_frames.FrameHeaders, h2_frames.FrameContinuation:
			sc.handleHeadersFrame(frame)
		case h2_frames.FrameSettings:
			sc.handleSettingsFrame(frame)
		case h2_frames.FramePing:
			sc.handlePingFrame(frame)
		case h2_frames.FrameGoAway:
			sc.handleGoAwayFrame(frame)
			return nil
		case h2_frames.FrameWindowUpdate:
			sc.handleWindowUpdateFrame(frame)
		case h2_frames.FrameRSTStream:
			sc.handleRSTStreamFrame(frame)
		case h2_frames.FramePriority:
			sc.handlePriorityFrame(frame)
		case h2_frames.FramePushPromise:
			sc.handlePushPromiseFrame(frame)
		default:
			sc.handleError(fmt.Errorf("unhandled frame type: %v", frame.Type), 0, h2_frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
			return fmt.Errorf("unhandled frame type: %v", frame.Type)
		}

		// Release the frame after handling it
		h2_frames.ReleaseFrame(frame)
	}
}

// handleError handles errors by logging, sending appropriate frames, and closing the connection if necessary
func (sc *h2ServerConn) handleError(err error, streamID uint32, frameType uint8, errorCode uint32) {
	log.Println("[H2] Error:", err)
	if frameType == h2_frames.FrameGoAway {
		sc.sendGoAway(streamID, errorCode)
		sc.closeConnection()
	} else if frameType == h2_frames.FrameRSTStream {
		sc.sendRSTStream(streamID, errorCode)
	}
}

// handshake performs the HTTP/2 connection handshake
func (sc *h2ServerConn) handshake() error {
	// Read the client preface
	sc.debug.Info("Reading client preface")
	preface := make([]byte, len(ClientPreface))
	if _, err := sc.netConn.Read(preface); err != nil {
		return fmt.Errorf("error reading client preface: %v", err)
	}
	if string(preface) != ClientPreface {
		return ErrInvalidPreface
	}

	// Send our initial SETTINGS frame
	sc.debug.Info("Sending initial SETTINGS frame")
	if err := sendSettings(sc.netConn, sc.serverSettings); err != nil {
		return fmt.Errorf("error sending initial SETTINGS frame: %v", err)
	}

	// Receive SETTINGS frame from client
	sc.debug.Info("Reading initial SETTINGS frame")
	frame, err := h2_frames.ReadFrame(sc.netConn)
	if err != nil {
		return err
	}
	if frame.Type != h2_frames.FrameSettings {
		return fmt.Errorf("expected SETTINGS frame, got %v", frame.Type)
	}

	// Apply the received serverSettings
	applySettings(frame, &sc.clientSettings)

	// Send SETTINGS ACK
	sc.debug.Info("Sending SETTINGS ACK")
	if err := sendSettingsAck(sc.netConn); err != nil {
		return fmt.Errorf("error sending SETTINGS ACK: %v", err)
	}
	return nil
}

// handleSettingsFrame handles SETTINGS frames
func (sc *h2ServerConn) handleSettingsFrame(frame *h2_frames.Frame) {
	// Apply the received serverSettings
	applySettings(frame, &sc.serverSettings)

	// Send SETTINGS ACK
	if err := sendSettingsAck(sc.netConn); err != nil {
		sc.handleError(err, 0, h2_frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
	}
}

// handleHeadersFrame handles HEADERS and CONTINUATION frames
func (sc *h2ServerConn) handleHeadersFrame(frame *h2_frames.Frame) {
	// Create or update the stream
	sc.debug.Infof("Handling HEADERS frame for stream %d", frame.StreamID)
	stream := sc.streamManager.GetStream(frame.StreamID)
	if stream == nil {
		stream = sc.streamManager.CreateStream(frame.StreamID, sc)
	}

	stream.ProcessHeadersFrame(frame, sc.hpack)
	// If the stream has the END_STREAM flag, process it
	if stream.state == StreamHalfClosedRemote {
		// Call early check hook
		if sc.h2Server.s.CheckReceivedHeaders != nil {
			closeConnection := sc.h2Server.s.CheckReceivedHeaders(stream.GetRequestCtx())
			if closeConnection {
				sc.sendGoAway(stream.id, h2_frames.FrameGoAway) // REFUSED_STREAM
				sc.closeConnection()
				return
			}
		}
		// Call on-headers-received hook. TODO: Do something with the return value
		if sc.h2Server.s.HeaderReceived != nil {
			sc.h2Server.s.HeaderReceived(&stream.requestCtx.Request.Header)
		}

		sc.streamProcessor.ProcessStream(stream, sc.h2Server.s)

	}

	// TODO: Execute stream
}

// handleWindowUpdateFrame handles WINDOW_UPDATE frames
func (sc *h2ServerConn) handleWindowUpdateFrame(frame *h2_frames.Frame) {
	sc.debug.Infof("Received WINDOW_UPDATE frame for stream %d", frame.StreamID)
	// Update the flow control window
	if frame.StreamID == 0 {
		// Connection-level window update
		delta := int32(binary.BigEndian.Uint32(frame.Body))
		sc.flowWindow += delta
		if sc.flowWindow < 0 {
			sc.handleError(errors.New("flow control error"), 0, h2_frames.FrameGoAway, 0x3) // FLOW_CONTROL_ERROR
		}
	} else {
		// Stream-level window update
		stream := sc.streamManager.GetStream(frame.StreamID)
		if stream == nil {
			sc.handleError(errors.New("stream closed"), frame.StreamID, h2_frames.FrameRSTStream, 0x5) // STREAM_CLOSED
			return
		}
		stream.AdjustWindow(int32(binary.BigEndian.Uint32(frame.Body)))
	}
}

// handleDataFrame handles DATA frames
func (sc *h2ServerConn) handleDataFrame(frame *h2_frames.Frame) {
	// Retrieve the stream
	stream := sc.streamManager.GetStream(frame.StreamID)
	if stream == nil {
		sc.handleError(fmt.Errorf("stream closed"), frame.StreamID, h2_frames.FrameRSTStream, 0x5) // Error code: STREAM_CLOSED
		return
	}

	// Adjust the flow control window
	delta := int32(len(frame.Body))
	sc.flowWindow -= delta
	stream.window.Add(-delta)

	if sc.flowWindow < 0 || stream.window.Load() < 0 {
		// Handle window underflow
		sc.handleWindowUnderflow(stream)
		return
	}

	// Append data to stream body
	stream.requestCtx.Request.AppendBody(frame.Body)

	// Check for END_STREAM flag
	if frame.Flags&h2_frames.FlagEndStream != 0 {
		stream.state = StreamHalfClosedRemote
		sc.streamProcessor.ProcessStream(stream, sc.s)
	}
}

// handleRSTStreamFrame handles RST_STREAM frames
func (sc *h2ServerConn) handleRSTStreamFrame(frame *h2_frames.Frame) {
	// Log and close the stream
	log.Printf("Received RST_STREAM frame for stream %d\n", frame.StreamID)
	sc.streamManager.RemoveStream(frame.StreamID)
}

// handlePriorityFrame handles PRIORITY frames
func (sc *h2ServerConn) handlePriorityFrame(frame *h2_frames.Frame) {
	// PRIORITY frames are used to change the priority of a stream
	sc.debug.Infof("Received PRIORITY frame for stream %d", frame.StreamID)

	// Parse the priority value from the frame body
	if len(frame.Body) < 5 {
		sc.handleError(fmt.Errorf("priority frame body too short"), 0, h2_frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
		return
	}

	sc.streamManager.SetStreamPriority(frame.StreamID, int32(frame.Body[0]), sc)
}

// handlePushPromiseFrame handles PUSH_PROMISE frames
func (sc *h2ServerConn) handlePushPromiseFrame(frame *h2_frames.Frame) {
	// PUSH_PROMISE frames are used to initiate server push
	log.Printf("Received PUSH_PROMISE frame, initiating server push\n")

	// Parse the PUSH_PROMISE frame and log the promised stream id
	if len(frame.Body) < 4 {
		sc.handleError(fmt.Errorf("PUSH_PROMISE frame body too short"), 0, h2_frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
		return
	}
	promisedStreamID := binary.BigEndian.Uint32(frame.Body[:4])
	log.Printf("Promised Stream ID: %d\n", promisedStreamID)

	// Create the promised stream
	sc.streamManager.CreateStream(promisedStreamID, sc)
}

// handlePingFrame handles PING frames
func (sc *h2ServerConn) handlePingFrame(frame *h2_frames.Frame) {
	// Respond with PING ACK
	frame.Flags |= h2_frames.FlagAck // ACK flag
	if err := frame.WriteTo(sc.netConn); err != nil {
		sc.handleError(err, 0, h2_frames.FrameGoAway, 0x1) // PROTOCOL_ERROR
	}
}

// handleGoAwayFrame handles GOAWAY frames
func (sc *h2ServerConn) handleGoAwayFrame(frame *h2_frames.Frame) {
	// Log and close the connection
	log.Printf("Received GOAWAY frame, closing connection\n")
	sc.closeConnection()
}

// handleWindowUnderflow handles flow control window underflow
func (sc *h2ServerConn) handleWindowUnderflow(stream *h2Stream) {
	log.Printf("Flow control window underflow for stream %d\n", stream.id)
	// Send RST_STREAM for the affected stream
	sc.sendRSTStream(stream.id, 0x3) // Error code: FLOW_CONTROL_ERROR
	sc.streamManager.RemoveStream(stream.id)
}

// sendRSTStream sends a RST_STREAM frame
func (sc *h2ServerConn) sendRSTStream(streamID uint32, errorCode uint32) {
	frame := h2_frames.AcquireFrame(h2_frames.FrameRSTStream)
	defer h2_frames.ReleaseFrame(frame)
	frame.StreamID = streamID
	frame.Body = make([]byte, 4)
	binary.BigEndian.PutUint32(frame.Body, errorCode)
	if err := frame.WriteTo(sc.netConn); err != nil {
		log.Println("Error sending RST_STREAM frame:", err)
	}
}

// sendGoAway sends a GOAWAY frame
func (sc *h2ServerConn) sendGoAway(lastStreamID uint32, errorCode uint32) {
	frame := h2_frames.AcquireFrame(h2_frames.FrameGoAway)
	defer h2_frames.ReleaseFrame(frame)
	frame.Body = make([]byte, 8)
	binary.BigEndian.PutUint32(frame.Body[:4], lastStreamID)
	binary.BigEndian.PutUint32(frame.Body[4:], errorCode)
	if err := frame.WriteTo(sc.netConn); err != nil {
		log.Println("Error sending GOAWAY frame:", err)
	}
}

// closeConnection closes the connection and releases resources
func (sc *h2ServerConn) closeConnection() {
	// Close the connection
	sc.netConn.Close()

	// Clean up resources
	for current := sc.streamManager.head; current != nil; current = current.next {
		sc.streamManager.RemoveStream(current.id)
	}
	log.Println("Connection closed and resources released")
}

// sendSettings sends a SETTINGS frame
func sendSettings(conn net.Conn, settings Settings) error {
	frame := h2_frames.AcquireFrame(h2_frames.FrameSettings)
	defer h2_frames.ReleaseFrame(frame)

	err := settings.PutParams(&frame.Body)
	if err != nil {
		return fmt.Errorf("error putting serverSettings params: %v", err)
	}

	return frame.WriteTo(conn)
}

// applySettings applies the received serverSettings
func applySettings(frame *h2_frames.Frame, settings *Settings) {
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
	frame := h2_frames.AcquireFrame(h2_frames.FrameSettings)
	defer h2_frames.ReleaseFrame(frame)
	frame.Flags = h2_frames.FlagAck // ACK flag
	return frame.WriteTo(conn)
}
