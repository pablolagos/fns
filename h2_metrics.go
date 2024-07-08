package fns

import (
	"log"
	"sync/atomic"
	"time"
)

type Metrics struct {
	totalConnections  int64
	activeConnections int64
	totalStreams      int64
	activeStreams     int64
}

var metrics = &Metrics{}

// IncrementConnections increments the total and active connections count
func IncrementConnections() {
	atomic.AddInt64(&metrics.totalConnections, 1)
	atomic.AddInt64(&metrics.activeConnections, 1)
}

// DecrementConnections decrements the active connections count
func DecrementConnections() {
	atomic.AddInt64(&metrics.activeConnections, -1)
}

// IncrementStreams increments the total and active streams count
func IncrementStreams() {
	atomic.AddInt64(&metrics.totalStreams, 1)
	atomic.AddInt64(&metrics.activeStreams, 1)
}

// DecrementStreams decrements the active streams count
func DecrementStreams() {
	atomic.AddInt64(&metrics.activeStreams, -1)
}

// LogMetrics logs the current metrics periodically
func LogMetrics(interval time.Duration) {
	for range time.Tick(interval) {
		log.Printf("Metrics - Total Connections: %d, Active Connections: %d, Total Streams: %d, Active Streams: %d",
			atomic.LoadInt64(&metrics.totalConnections),
			atomic.LoadInt64(&metrics.activeConnections),
			atomic.LoadInt64(&metrics.totalStreams),
			atomic.LoadInt64(&metrics.activeStreams))
	}
}
