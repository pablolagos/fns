package debuglog

import (
	"fmt"
	"log"
)

// Logger struct to represent a logger
type Logger struct {
	debug  bool
	logger *log.Logger
}

// New creates a new Logger instance
func New(debug bool, logger *log.Logger) *Logger {
	return &Logger{debug: debug, logger: logger}
}

// Log prints the log message if debug is true
func (l *Logger) Log(message string) {
	if l.debug {
		l.logger.Printf("LOG: %s\n", message)
	}
}

// Info prints the info message if debug is true
func (l *Logger) Info(message string) {
	if l.debug {
		l.logger.Printf("INFO: %s\n", message)
	}
}

// Error prints the error message if debug is true
func (l *Logger) Error(message string) {
	if l.debug {
		l.logger.Printf("ERROR: %s\n", message)
	}
}

// Logf prints the formatted log message if debug is true
func (l *Logger) Logf(format string, a ...interface{}) {
	if l.debug {
		message := fmt.Sprintf(format, a...)
		l.logger.Printf("LOG: %s\n", message)
	}
}

// Infof prints the formatted info message if debug is true
func (l *Logger) Infof(format string, a ...interface{}) {
	if l.debug {
		message := fmt.Sprintf(format, a...)
		l.logger.Printf("INFO: %s\n", message)
	}
}

// Errorf prints the formatted error message if debug is true
func (l *Logger) Errorf(format string, a ...interface{}) {
	if l.debug {
		message := fmt.Sprintf(format, a...)
		l.logger.Printf("ERROR: %s\n", message)
	}
}
