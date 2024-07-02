package fns

import (
	"log"
	"net"
)

// ServerConfig stores the configuration for the HTTP/2 server
type ServerConfig struct {
	Addr         string
	ReadTimeout  int
	WriteTimeout int
}

// Server represents the HTTP/2 server
type H2Server struct {
	s    *Server
	conf ServerConfig
}

// ConfigureH2 initializes the server for http/2 connections
func ConfigureH2(s *Server, conf ServerConfig) {
	// Assign handler for HTTP/2 connections
	s.NextProto("h2", s.HandleHTTP2Conn)
}

// defaults sets default values for ServerConfig
func (conf *ServerConfig) defaults() {
	if conf.Addr == "" {
		conf.Addr = ":443"
	}
	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = 10 // default read timeout in seconds
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 10 // default write timeout in seconds
	}
}

// HandleHTTP2Conn handles HTTP/2 connections
func (srv *Server) HandleHTTP2Conn(conn net.Conn) error {
	serverConn := &h2ServerConn{
		conn: conn,
		s:    srv.s,
	}

	// Serve the HTTP/2 connection
	if err := serverConn.Serve(); err != nil {
		log.Printf("Error serving connection: %v", err)
		return err
	}

	return nil
}
