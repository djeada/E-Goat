// internal/metrics/metrics.go
package metrics

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Collector provides application metrics collection and reporting
type Collector struct {
	mu sync.RWMutex

	// Request metrics
	requestCount     atomic.Int64
	requestErrors    atomic.Int64
	requestLatencies []time.Duration

	// Connection metrics
	activeConnections    atomic.Int64
	totalConnections     atomic.Int64
	connectionErrors     atomic.Int64
	webrtcConnections    atomic.Int64
	websocketConnections atomic.Int64
	httpPollingClients   atomic.Int64

	// Message metrics
	messagesSent     atomic.Int64
	messagesReceived atomic.Int64
	messagesBytes    atomic.Int64

	// Transport metrics
	transportSuccesses map[string]int64
	transportFailures  map[string]int64
	transportLatencies map[string][]time.Duration

	// Peer metrics
	activePeers map[string]*PeerMetrics

	startTime time.Time
}

// PeerMetrics tracks per-peer statistics
type PeerMetrics struct {
	PeerID          string    `json:"peer_id"`
	ConnectedAt     time.Time `json:"connected_at"`
	LastActivity    time.Time `json:"last_activity"`
	MessagesSent    int64     `json:"messages_sent"`
	MessagesRecv    int64     `json:"messages_received"`
	BytesSent       int64     `json:"bytes_sent"`
	BytesRecv       int64     `json:"bytes_received"`
	ConnectionType  string    `json:"connection_type"`
	ConnectionQuality int     `json:"connection_quality"`
	Latency         time.Duration `json:"latency"`
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	return &Collector{
		requestLatencies:   make([]time.Duration, 0, 1000),
		transportSuccesses: make(map[string]int64),
		transportFailures:  make(map[string]int64),
		transportLatencies: make(map[string][]time.Duration),
		activePeers:        make(map[string]*PeerMetrics),
		startTime:          time.Now(),
	}
}

// RecordRequest records an HTTP request
func (c *Collector) RecordRequest(duration time.Duration, isError bool) {
	c.requestCount.Add(1)
	if isError {
		c.requestErrors.Add(1)
	}

	c.mu.Lock()
	c.requestLatencies = append(c.requestLatencies, duration)
	// Keep only last 1000 latencies
	if len(c.requestLatencies) > 1000 {
		c.requestLatencies = c.requestLatencies[1:]
	}
	c.mu.Unlock()
}

// RecordConnection records a new connection
func (c *Collector) RecordConnection(connType string) {
	c.activeConnections.Add(1)
	c.totalConnections.Add(1)

	switch connType {
	case "webrtc":
		c.webrtcConnections.Add(1)
	case "websocket":
		c.websocketConnections.Add(1)
	case "http":
		c.httpPollingClients.Add(1)
	}
}

// RecordDisconnection records a disconnection
func (c *Collector) RecordDisconnection(connType string) {
	c.activeConnections.Add(-1)

	switch connType {
	case "webrtc":
		c.webrtcConnections.Add(-1)
	case "websocket":
		c.websocketConnections.Add(-1)
	case "http":
		c.httpPollingClients.Add(-1)
	}
}

// RecordConnectionError records a connection error
func (c *Collector) RecordConnectionError() {
	c.connectionErrors.Add(1)
}

// RecordMessage records a message
func (c *Collector) RecordMessage(isSent bool, bytes int64) {
	if isSent {
		c.messagesSent.Add(1)
	} else {
		c.messagesReceived.Add(1)
	}
	c.messagesBytes.Add(bytes)
}

// RecordTransportAttempt records a transport attempt
func (c *Collector) RecordTransportAttempt(transportType string, success bool, latency time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if success {
		c.transportSuccesses[transportType]++
	} else {
		c.transportFailures[transportType]++
	}

	if latencies, ok := c.transportLatencies[transportType]; ok {
		c.transportLatencies[transportType] = append(latencies, latency)
		if len(c.transportLatencies[transportType]) > 100 {
			c.transportLatencies[transportType] = c.transportLatencies[transportType][1:]
		}
	} else {
		c.transportLatencies[transportType] = []time.Duration{latency}
	}
}

// RegisterPeer registers a new peer
func (c *Collector) RegisterPeer(peerID, connType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.activePeers[peerID] = &PeerMetrics{
		PeerID:         peerID,
		ConnectedAt:    time.Now(),
		LastActivity:   time.Now(),
		ConnectionType: connType,
	}
}

// UpdatePeerActivity updates peer activity
func (c *Collector) UpdatePeerActivity(peerID string, messagesSent, messagesRecv, bytesSent, bytesRecv int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if peer, ok := c.activePeers[peerID]; ok {
		peer.LastActivity = time.Now()
		peer.MessagesSent += messagesSent
		peer.MessagesRecv += messagesRecv
		peer.BytesSent += bytesSent
		peer.BytesRecv += bytesRecv
	}
}

// UpdatePeerQuality updates peer connection quality
func (c *Collector) UpdatePeerQuality(peerID string, quality int, latency time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if peer, ok := c.activePeers[peerID]; ok {
		peer.ConnectionQuality = quality
		peer.Latency = latency
	}
}

// UnregisterPeer removes a peer
func (c *Collector) UnregisterPeer(peerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.activePeers, peerID)
}

// Metrics contains all collected metrics
type Metrics struct {
	Uptime string `json:"uptime"`
	
	// Request metrics
	Requests struct {
		Total         int64   `json:"total"`
		Errors        int64   `json:"errors"`
		ErrorRate     float64 `json:"error_rate"`
		AvgLatencyMs  float64 `json:"avg_latency_ms"`
		P99LatencyMs  float64 `json:"p99_latency_ms"`
	} `json:"requests"`

	// Connection metrics
	Connections struct {
		Active    int64 `json:"active"`
		Total     int64 `json:"total"`
		Errors    int64 `json:"errors"`
		WebRTC    int64 `json:"webrtc"`
		WebSocket int64 `json:"websocket"`
		HTTP      int64 `json:"http_polling"`
	} `json:"connections"`

	// Message metrics
	Messages struct {
		Sent     int64 `json:"sent"`
		Received int64 `json:"received"`
		Bytes    int64 `json:"bytes"`
	} `json:"messages"`

	// Transport metrics
	Transports map[string]TransportMetrics `json:"transports"`

	// Peer metrics
	Peers struct {
		Active int    `json:"active"`
		List   []PeerSummary `json:"list,omitempty"`
	} `json:"peers"`
}

// TransportMetrics contains metrics for a specific transport type
type TransportMetrics struct {
	Successes    int64   `json:"successes"`
	Failures     int64   `json:"failures"`
	SuccessRate  float64 `json:"success_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// PeerSummary is a simplified view of peer metrics
type PeerSummary struct {
	PeerID      string `json:"peer_id"`
	ConnType    string `json:"connection_type"`
	Quality     int    `json:"quality"`
	LatencyMs   int64  `json:"latency_ms"`
	ActiveSince string `json:"active_since"`
}

// GetMetrics returns all collected metrics
func (c *Collector) GetMetrics() *Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	m := &Metrics{}
	m.Uptime = time.Since(c.startTime).String()

	// Request metrics
	m.Requests.Total = c.requestCount.Load()
	m.Requests.Errors = c.requestErrors.Load()
	if m.Requests.Total > 0 {
		m.Requests.ErrorRate = float64(m.Requests.Errors) / float64(m.Requests.Total) * 100
	}
	m.Requests.AvgLatencyMs = c.calculateAvgLatency(c.requestLatencies)
	m.Requests.P99LatencyMs = c.calculateP99Latency(c.requestLatencies)

	// Connection metrics
	m.Connections.Active = c.activeConnections.Load()
	m.Connections.Total = c.totalConnections.Load()
	m.Connections.Errors = c.connectionErrors.Load()
	m.Connections.WebRTC = c.webrtcConnections.Load()
	m.Connections.WebSocket = c.websocketConnections.Load()
	m.Connections.HTTP = c.httpPollingClients.Load()

	// Message metrics
	m.Messages.Sent = c.messagesSent.Load()
	m.Messages.Received = c.messagesReceived.Load()
	m.Messages.Bytes = c.messagesBytes.Load()

	// Transport metrics
	m.Transports = make(map[string]TransportMetrics)
	for transportType := range c.transportSuccesses {
		successes := c.transportSuccesses[transportType]
		failures := c.transportFailures[transportType]
		total := successes + failures

		tm := TransportMetrics{
			Successes: successes,
			Failures:  failures,
		}
		if total > 0 {
			tm.SuccessRate = float64(successes) / float64(total) * 100
		}
		if latencies, ok := c.transportLatencies[transportType]; ok {
			tm.AvgLatencyMs = c.calculateAvgLatency(latencies)
		}
		m.Transports[transportType] = tm
	}

	// Peer metrics
	m.Peers.Active = len(c.activePeers)
	m.Peers.List = make([]PeerSummary, 0, len(c.activePeers))
	for _, peer := range c.activePeers {
		m.Peers.List = append(m.Peers.List, PeerSummary{
			PeerID:      peer.PeerID[:min(8, len(peer.PeerID))] + "...",
			ConnType:    peer.ConnectionType,
			Quality:     peer.ConnectionQuality,
			LatencyMs:   peer.Latency.Milliseconds(),
			ActiveSince: time.Since(peer.ConnectedAt).String(),
		})
	}

	return m
}

func (c *Collector) calculateAvgLatency(latencies []time.Duration) float64 {
	if len(latencies) == 0 {
		return 0
	}
	var total time.Duration
	for _, l := range latencies {
		total += l
	}
	return float64(total.Milliseconds()) / float64(len(latencies))
}

func (c *Collector) calculateP99Latency(latencies []time.Duration) float64 {
	if len(latencies) == 0 {
		return 0
	}
	// Simple P99: take the 99th percentile element
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	// Simple insertion sort for small arrays
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	idx := int(float64(len(sorted)) * 0.99)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return float64(sorted[idx].Milliseconds())
}

// MetricsHandler returns an HTTP handler for metrics
func (c *Collector) MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c.GetMetrics())
	}
}

// MetricsMiddleware wraps handlers to collect request metrics
func (c *Collector) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		isError := wrapped.status >= 400
		c.RecordRequest(duration, isError)

		if isError {
			log.Printf("Request error: %s %s %d", r.Method, r.URL.Path, wrapped.status)
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
