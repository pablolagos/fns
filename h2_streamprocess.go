package fns

import (
	"log"

	"github.com/pablolagos/fns/internal/hpack"
)

// StreamProcessor handles the processing of HTTP/2 streams
type StreamProcessor struct{}

// NewStreamProcessor creates a new StreamProcessor
func NewStreamProcessor() *StreamProcessor {
	return &StreamProcessor{}
}

// ProcessStream processes a completed HTTP/2 stream
func (sp *StreamProcessor) ProcessStream(stream *Stream, s *Server) {
	// Create a new RequestCtx
	ctx := &RequestCtx{}

	// Populate the RequestCtx with the headers and body from the stream
	sp.populateRequestCtx(ctx, stream)

	// Call the handler
	s.Handler(ctx)

	// Process the response from the handler
	sp.processResponse(ctx, stream)
}

// populateRequestCtx populates the RequestCtx with data from the stream
func (sp *StreamProcessor) populateRequestCtx(ctx *RequestCtx, stream *Stream) {
	// Parse headers from the stream and populate the RequestCtx
	for _, headerField := range stream.Headers {
		ctx.Request.Header.Set(headerField.Name, headerField.Value)
	}

	// Determine the HTTP method
	method := ctx.Request.Header.Method()
	if len(method) == 0 {
		method = []byte("GET") // Default method if not set
	}
	ctx.Request.Header.SetMethodBytes(method)

	// Set the request body
	ctx.Request.SetBody(stream.Body)

	// Extract the URI from the headers
	uri := ctx.Request.Header.Peek(":path")
	if uri != nil {
		ctx.Request.SetRequestURIBytes(uri)
	}

	// Set the host
	host := ctx.Request.Header.Peek("host")
	if host != nil {
		ctx.Request.SetHostBytes(host)
	}

	// Set the scheme
	scheme := ctx.Request.Header.Peek(":scheme")
	if scheme == nil {
		scheme = []byte("https") // Default to https if not set
	}
	ctx.Request.URI().SetSchemeBytes(scheme)

	// Set the authority (host:port)
	authority := ctx.Request.Header.Peek(":authority")
	if authority != nil {
		ctx.Request.URI().SetHostBytes(authority)
	}

	// Set the remote address
	remoteAddr := stream.conn.conn.RemoteAddr()
	ctx.Init(&ctx.Request, remoteAddr, ctx.Logger())

	// Log the populated request context for debugging
	log.Printf("Request Headers: %s", ctx.Request.Header.String())
	log.Printf("Request URI: %s", ctx.Request.URI().String())
	log.Printf("Request Body: %s", string(ctx.Request.Body()))
}

// processResponse processes the response and updates the stream
func (sp *StreamProcessor) processResponse(ctx *RequestCtx, stream *Stream) {
	// Copy the response headers and body from the RequestCtx to the stream
	response := ctx.Response
	stream.ResponseHeaders = make([]hpack.HeaderField, 0, response.Header.Len())
	response.Header.VisitAll(func(key, value []byte) {
		stream.ResponseHeaders = append(stream.ResponseHeaders, hpack.HeaderField{
			Name:  string(key),
			Value: string(value),
		})
	})
	stream.ResponseBody = response.Body()

	// Log the processed response for debugging
	log.Printf("Response Headers: %s", response.Header.String())
	log.Printf("Response Body: %s", string(response.Body()))
}
