package tests

import (
	"context"
	"testing"
	"time"

	"github.com/djeada/E-Goat/internal/transport"
)

func TestConnectionFactory(t *testing.T) {
	tests := []struct {
		name        string
		factoryType transport.ConnectionType
	}{
		{"WebRTC STUN Factory", transport.WebRTCSTUN},
		{"WebRTC TURN Factory", transport.WebRTCTURN},
		{"WebSocket Direct Factory", transport.WebSocketDirect},
		{"HTTP Polling Factory", transport.HTTPPolling},
		{"LAN Broadcast Factory", transport.LANBroadcast},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := createTestFactory(tt.factoryType)
			if factory == nil {
				t.Fatalf("Failed to create factory for %s", tt.factoryType)
			}

			// Test factory properties
			if factory.Priority() <= 0 {
				t.Errorf("Factory priority should be > 0, got %d", factory.Priority())
			}

			if !factory.CanCreate(tt.factoryType) {
				t.Errorf("Factory should be able to create its own type %s", tt.factoryType)
			}

			// Test connection creation (may fail in test env, that's ok)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			config := map[string]interface{}{
				"test": true,
			}

			_, err := factory.Create(ctx, "test-peer", config)
			if err != nil {
				// Connection creation can fail in test environment, that's expected
				t.Logf("Connection creation failed (expected in test env): %v", err)
			}
		})
	}
}

func TestLayeredConnectionManager(t *testing.T) {
	manager := transport.NewLayeredConnectionManager()
	if manager == nil {
		t.Fatal("LayeredConnectionManager should not be nil")
	}

	// Test that manager is created properly
	// In a real implementation, we'd add factories and test them
	// but the current implementation doesn't expose factory list
}

func TestConnectionPriorities(t *testing.T) {
	expectedPriorities := map[transport.ConnectionType]int{
		transport.WebRTCSTUN:      100,
		transport.WebRTCTURN:      80,
		transport.WebSocketDirect: 60,
		transport.HTTPPolling:     40,
		transport.LANBroadcast:    20,
	}

	for factoryType, expectedPriority := range expectedPriorities {
		factory := createTestFactory(factoryType)
		if factory == nil {
			t.Fatalf("Failed to create factory for %s", factoryType)
		}

		if factory.Priority() != expectedPriority {
			t.Errorf("Factory %s should have priority %d, got %d",
				factoryType, expectedPriority, factory.Priority())
		}
	}
}

func TestConnectionTypes(t *testing.T) {
	connectionTypes := []transport.ConnectionType{
		transport.WebRTCSTUN,
		transport.WebRTCTURN,
		transport.WebSocketDirect,
		transport.HTTPPolling,
		transport.LANBroadcast,
	}

	for _, connType := range connectionTypes {
		factory := createTestFactory(connType)
		if factory == nil {
			t.Fatalf("Failed to create factory for connection type %s", connType)
		}

		if !factory.CanCreate(connType) {
			t.Errorf("Factory should be able to create connection type %s", connType)
		}

		// Test that factory cannot create other types
		for _, otherType := range connectionTypes {
			if otherType != connType && factory.CanCreate(otherType) {
				t.Errorf("Factory for %s should not be able to create %s", connType, otherType)
			}
		}
	}
}

// Helper function to create test factories with default configurations
func createTestFactory(factoryType transport.ConnectionType) transport.ConnectionFactory {
	switch factoryType {
	case transport.WebRTCSTUN:
		return transport.NewWebRTCSTUNFactory([]string{"stun:stun.l.google.com:19302"})
	case transport.WebRTCTURN:
		return transport.NewWebRTCTURNFactory([]transport.TURNServer{
			{URL: "turn:test.com", Username: "test", Password: "test"},
		})
	case transport.WebSocketDirect:
		return transport.NewWebSocketDirectFactory([]int{8080, 9000})
	case transport.HTTPPolling:
		return transport.NewHTTPPollingFactory(5 * time.Second)
	case transport.LANBroadcast:
		return transport.NewLANBroadcastFactory(12345, 54321)
	default:
		return nil
	}
}
