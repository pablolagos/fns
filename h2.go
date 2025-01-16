//go:build h2

package fns

// EnableHTTP2 initializes the server for http/2 connections
func EnableHTTP2(s *Server, conf H2ServerConfig) {
	h2s := newH2Server(s, conf)

	// Assign handler for HTTP/2 connections
	s.NextProto("h2", h2s.handleHTTP2Conn)
}
