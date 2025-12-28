// tests/metrics_test.go
package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/djeada/E-Goat/internal/metrics"
)

func TestMetricsCollector(t *testing.T) {
	collector := metrics.NewCollector()

	// Test request recording
	collector.RecordRequest(100*time.Millisecond, false)
	collector.RecordRequest(200*time.Millisecond, false)
	collector.RecordRequest(50*time.Millisecond, true)

	m := collector.GetMetrics()

	if m.Requests.Total != 3 {
		t.Errorf("Expected 3 total requests, got %d", m.Requests.Total)
	}

	if m.Requests.Errors != 1 {
		t.Errorf("Expected 1 error, got %d", m.Requests.Errors)
	}
}

func TestConnectionMetrics(t *testing.T) {
	collector := metrics.NewCollector()

	// Record connections
	collector.RecordConnection("webrtc")
	collector.RecordConnection("websocket")
	collector.RecordConnection("http")

	m := collector.GetMetrics()

	if m.Connections.Active != 3 {
		t.Errorf("Expected 3 active connections, got %d", m.Connections.Active)
	}

	if m.Connections.WebRTC != 1 {
		t.Errorf("Expected 1 WebRTC connection, got %d", m.Connections.WebRTC)
	}

	// Test disconnection
	collector.RecordDisconnection("webrtc")
	m = collector.GetMetrics()

	if m.Connections.Active != 2 {
		t.Errorf("Expected 2 active connections after disconnection, got %d", m.Connections.Active)
	}
}

func TestMessageMetrics(t *testing.T) {
	collector := metrics.NewCollector()

	collector.RecordMessage(true, 100)  // sent
	collector.RecordMessage(false, 200) // received
	collector.RecordMessage(true, 150)  // sent

	m := collector.GetMetrics()

	if m.Messages.Sent != 2 {
		t.Errorf("Expected 2 sent messages, got %d", m.Messages.Sent)
	}

	if m.Messages.Received != 1 {
		t.Errorf("Expected 1 received message, got %d", m.Messages.Received)
	}

	if m.Messages.Bytes != 450 {
		t.Errorf("Expected 450 bytes, got %d", m.Messages.Bytes)
	}
}

func TestPeerMetrics(t *testing.T) {
	collector := metrics.NewCollector()

	collector.RegisterPeer("peer-123", "webrtc")
	collector.UpdatePeerActivity("peer-123", 5, 3, 500, 300)
	collector.UpdatePeerQuality("peer-123", 85, 50*time.Millisecond)

	m := collector.GetMetrics()

	if m.Peers.Active != 1 {
		t.Errorf("Expected 1 active peer, got %d", m.Peers.Active)
	}

	// Unregister peer
	collector.UnregisterPeer("peer-123")
	m = collector.GetMetrics()

	if m.Peers.Active != 0 {
		t.Errorf("Expected 0 active peers after unregister, got %d", m.Peers.Active)
	}
}

func TestTransportMetrics(t *testing.T) {
	collector := metrics.NewCollector()

	collector.RecordTransportAttempt("webrtc-stun", true, 100*time.Millisecond)
	collector.RecordTransportAttempt("webrtc-stun", true, 150*time.Millisecond)
	collector.RecordTransportAttempt("webrtc-stun", false, 5*time.Second)

	m := collector.GetMetrics()

	if tm, ok := m.Transports["webrtc-stun"]; ok {
		if tm.Successes != 2 {
			t.Errorf("Expected 2 successes, got %d", tm.Successes)
		}
		if tm.Failures != 1 {
			t.Errorf("Expected 1 failure, got %d", tm.Failures)
		}
	} else {
		t.Error("Expected webrtc-stun transport metrics")
	}
}

func TestMetricsHandler(t *testing.T) {
	collector := metrics.NewCollector()
	collector.RecordRequest(100*time.Millisecond, false)

	handler := collector.MetricsHandler()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var response metrics.Metrics
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse JSON response: %v", err)
	}

	if response.Requests.Total != 1 {
		t.Errorf("Expected 1 request in response, got %d", response.Requests.Total)
	}
}

func TestMetricsMiddleware(t *testing.T) {
	collector := metrics.NewCollector()

	handler := collector.MetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Make some requests
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	m := collector.GetMetrics()

	if m.Requests.Total != 5 {
		t.Errorf("Expected 5 requests recorded, got %d", m.Requests.Total)
	}
}
