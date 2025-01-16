//go:build h2

package fns

import (
	"log"
	"net"

	"github.com/pablolagos/fns/internal/debuglog"
)

// h2Server Server represents the HTTP/2 server
type h2Server struct {
	s      *Server // the underlying server
	conf   H2ServerConfig
	debug  *debuglog.Logger
	logger Logger
}

// H2ServerConfig stores the configuration for the HTTP/2 server
type H2ServerConfig struct {
	Addr         string
	ReadTimeout  int  // in seconds
	WriteTimeout int  // in seconds
	Debug        bool // True to log debug messages
	Logger       *log.Logger
}

// newH2Server creates a new HTTP/2 server and ensures that the logger is set.
func newH2Server(s *Server, conf H2ServerConfig) *h2Server {
	h2s := &h2Server{
		s:     s,
		conf:  conf,
		debug: debuglog.New(conf.Debug, conf.Logger),
	}
	if h2s.conf.Logger == nil {
		h2s.conf.Logger = log.New(log.Writer(), "[H2] ", log.Lmsgprefix|log.LstdFlags)
	}
	return h2s
}

// DefaultH2Config defaults sets default values for HTTP/2 server
func DefaultH2Config() H2ServerConfig {
	return H2ServerConfig{
		Addr:         ":443",
		ReadTimeout:  10,
		WriteTimeout: 10,
		Debug:        false,
		Logger:       log.New(log.Writer(), "[H2] ", log.Lmsgprefix|log.LstdFlags),
	}
}

// handleHTTP2Conn handles new HTTP/2 connections
func (h2s *h2Server) handleHTTP2Conn(conn net.Conn) error {
	h2ServerConn := newH2ServerConn(conn, h2s)

	// Serve the HTTP/2 connection
	if err := h2ServerConn.Serve(); err != nil {
		h2s.s.Logger.Printf("Error serving connection: %v", err)
		return err
	}

	return nil
}
