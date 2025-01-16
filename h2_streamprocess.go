//go:build h2

package fns

import (
	"log"

	"github.com/pablolagos/fns/internal/debuglog"

	"github.com/pablolagos/fns/internal/hpack"
)

// StreamProcessor handles the processing of HTTP/2 streams
type StreamProcessor struct {
	logger Logger
	debug  *debuglog.Logger
	hpack  *hpack.Codec
}

// NewStreamProcessor creates a new StreamProcessor
func NewStreamProcessor(logger Logger, debug *debuglog.Logger, hpackCodec *hpack.Codec) *StreamProcessor {
	return &StreamProcessor{
		logger: logger,
		debug:  debug,
		hpack:  hpackCodec,
	}
}

// ProcessStream processes a completed HTTP/2 stream
func (sp *StreamProcessor) ProcessStream(stream *h2Stream, s *Server) {
	sp.debug.Infof("Processing stream %v", stream)

	// Call the handler. TODO: implement a worker pool with limited number of workers
	s.Handler(stream.requestCtx)

	sp.processResponse(stream)

	// Send the response using HEADER and DATA frames
	stream.sendResponse()

}

// processResponse processes the response and updates the stream
func (sp *StreamProcessor) processResponse(stream *h2Stream) {
	// Log the processed response for debugging
	log.Printf("Response Headers: %s", stream.requestCtx.Response.Header.String())
	log.Printf("Response Body: %s", string(stream.requestCtx.Response.Body()))
}
