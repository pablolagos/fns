package fns

import (
	"log"
	"net"
)

// ServerConfig stores the configuration for the HTTP/2 server
type ServerConfig struct {
	Addr         string
	ReadTimeout  int // in seconds
	WriteTimeout int // in seconds
}

// Server represents the HTTP/2 server
type H2Server struct {
	s    *Server
	conf ServerConfig
}

// EnableHTTP2 initializes the server for http/2 connections
func EnableHTTP2(s *Server, conf ServerConfig) {
	h2s := &H2Server{
		s:    s,
		conf: conf,
	}
	// Assign handler for HTTP/2 connections
	s.NextProto("h2", h2s.HandleHTTP2Conn)
}

// DefaultH2Config defaults sets default values for HTTP/2 server
func DefaultH2Config() ServerConfig {
	return ServerConfig{
		Addr:         ":443",
		ReadTimeout:  10,
		WriteTimeout: 10,
	}
}

// HandleHTTP2Conn handles HTTP/2 connections
func (h2 *H2Server) HandleHTTP2Conn(conn net.Conn) error {
	serverConn := &h2ServerConn{
		conn: conn,
		s:    h2.s,
	}

	// Serve the HTTP/2 connection
	if err := serverConn.Serve(); err != nil {
		log.Printf("Error serving connection: %v", err)
		return err
	}

	return nil
}
