// cmd/messanger/transport_integration.go
package main

import (
	"log"
	"time"

	"github.com/djeada/E-Goat/internal/transport"
)

// ChatTransport wraps the transport manager for E-Goat chat
type ChatTransport struct {
	transport    *transport.TransportManager
	peerID       string
	onMessage    func(from, text string)
	onConnection func(peerID string, connType string)
}

// NewChatTransport creates a new chat transport
func NewChatTransport(peerID string) *ChatTransport {
	ct := &ChatTransport{
		transport: transport.NewTransportManager(peerID),
		peerID:    peerID,
	}

	// Set up handlers
	ct.transport.SetMessageHandler(ct.handleMessage)
	ct.transport.SetConnectionHandler(ct.handleConnection)
	ct.transport.SetDisconnectHandler(ct.handleDisconnect)

	return ct
}

func (ct *ChatTransport) handleMessage(msg transport.Message) {
	if msg.Type == "chat" && ct.onMessage != nil {
		ct.onMessage(msg.From, string(msg.Data))
	}
}

func (ct *ChatTransport) handleConnection(peerID string, conn transport.Connection) {
	log.Printf("🔗 Connected to %s via %s (quality: %d%%)", 
		peerID, conn.Type(), conn.Quality())
	
	if ct.onConnection != nil {
		ct.onConnection(peerID, string(conn.Type()))
	}
}

func (ct *ChatTransport) handleDisconnect(peerID string, connType transport.ConnectionType) {
	log.Printf("🔌 Disconnected from %s (was using %s)", peerID, connType)
}

// ConnectToPeer attempts to connect to a peer with automatic fallback
func (ct *ChatTransport) ConnectToPeer(peerID, peerIP, networkType string) error {
	log.Printf("🚀 Starting layered connection to %s...", peerID)
	
	// Create network info for the connection attempt
	networkInfo := ct.transport.CreateNetworkInfo(peerIP, "", networkType)
	
	// Log the connection strategy
	log.Printf("📊 Network Info: %+v", networkInfo)
	log.Printf("🎯 Connection Strategy:")
	log.Printf("   1. WebRTC STUN (direct P2P)")
	log.Printf("   2. WebRTC TURN (relayed P2P)")
	log.Printf("   3. Direct WebSocket")
	log.Printf("   4. HTTP Polling")
	log.Printf("   5. LAN Broadcast")
	
	return ct.transport.ConnectToPeer(peerID, networkInfo)
}

// SendMessage sends a chat message
func (ct *ChatTransport) SendMessage(peerID, text string) error {
	return ct.transport.SendMessage(peerID, "chat", []byte(text))
}

// GetConnectionStatus returns the current connection status
func (ct *ChatTransport) GetConnectionStatus(peerID string) map[string]interface{} {
	return ct.transport.GetConnectionInfo(peerID)
}

// SetMessageHandler sets the message callback
func (ct *ChatTransport) SetMessageHandler(handler func(from, text string)) {
	ct.onMessage = handler
}

// SetConnectionHandler sets the connection callback
func (ct *ChatTransport) SetConnectionHandler(handler func(peerID, connType string)) {
	ct.onConnection = handler
}

// Close shuts down the transport
func (ct *ChatTransport) Close() error {
	return ct.transport.Close()
}

// Example usage function
func demonstrateLayeredConnections() {
	log.Println("🎭 E-Goat Layered Connection Demo")
	log.Println("==================================")

	// Create two chat instances
	alice := NewChatTransport("alice")
	bob := NewChatTransport("bob")

	// Set up message handlers
	alice.SetMessageHandler(func(from, text string) {
		log.Printf("👩 Alice received from %s: %s", from, text)
	})

	bob.SetMessageHandler(func(from, text string) {
		log.Printf("👨 Bob received from %s: %s", from, text)
	})

	// Set up connection handlers
	alice.SetConnectionHandler(func(peerID, connType string) {
		log.Printf("👩 Alice connected to %s via %s", peerID, connType)
		
		// Send a message once connected
		time.AfterFunc(time.Second*2, func() {
			alice.SendMessage(peerID, "Hello Bob! This is Alice.")
		})
	})

	bob.SetConnectionHandler(func(peerID, connType string) {
		log.Printf("👨 Bob connected to %s via %s", peerID, connType)
		
		// Send a response
		time.AfterFunc(time.Second*4, func() {
			bob.SendMessage(peerID, "Hi Alice! Bob here. Connection working!")
		})
	})

	// Simulate different network scenarios
	scenarios := []struct {
		name        string
		networkType string
		peerIP      string
	}{
		{"Local Network", "lan", "192.168.1.100"},
		{"Internet", "internet", "203.0.113.1"},
		{"WiFi Network", "wifi", "10.0.0.50"},
	}

	for _, scenario := range scenarios {
		log.Printf("\n🌐 Testing scenario: %s", scenario.name)
		log.Printf("   Network Type: %s", scenario.networkType)
		log.Printf("   Peer IP: %s", scenario.peerIP)

		// Alice tries to connect to Bob
		if err := alice.ConnectToPeer("bob", scenario.peerIP, scenario.networkType); err != nil {
			log.Printf("❌ Connection failed: %v", err)
		} else {
			log.Printf("✅ Connection initiated")
		}

		// Let the connection attempt run
		time.Sleep(time.Second * 10)

		// Show connection status
		status := alice.GetConnectionStatus("bob")
		log.Printf("📊 Connection Status: %+v", status)

		log.Println("---")
	}

	// Cleanup
	time.AfterFunc(time.Second*30, func() {
		alice.Close()
		bob.Close()
		log.Println("🧹 Demo cleanup completed")
	})
}
