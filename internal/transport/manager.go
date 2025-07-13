// internal/transport/manager.go
package transport

import (
	"context"
	"fmt"
	"log"
	"time"
)

// TransportManager provides a simple interface to the layered connection system
type TransportManager struct {
	connectionManager *LayeredConnectionManager
	myPeerID          string
}

// NewTransportManager creates a new transport manager with all connection types
func NewTransportManager(peerID string) *TransportManager {
	tm := &TransportManager{
		connectionManager: NewLayeredConnectionManager(),
		myPeerID:          peerID,
	}

	// Register all connection factories in priority order
	tm.registerFactories()

	return tm
}

func (tm *TransportManager) registerFactories() {
	log.Println("Registering connection factories...")

	// 1. WebRTC STUN - Highest priority (direct P2P)
	stunServers := []string{
		"stun:stun.l.google.com:19302",
		"stun:stun1.l.google.com:19302",
		"stun:stun2.l.google.com:19302",
	}
	stunFactory := NewWebRTCSTUNFactory(stunServers)
	tm.connectionManager.RegisterFactory(stunFactory)

	// 2. WebRTC TURN - High priority (P2P with relay)
	turnServers := []TURNServer{
		{
			URL:      "turn:relay.example.com:3478",
			Username: "user",
			Password: "pass",
		},
		// Add more TURN servers as needed
	}
	turnFactory := NewWebRTCTURNFactory(turnServers)
	tm.connectionManager.RegisterFactory(turnFactory)

	// 3. Direct WebSocket - Medium-high priority
	wsFactory := NewWebSocketDirectFactory([]int{9000, 8080, 80, 443})
	tm.connectionManager.RegisterFactory(wsFactory)

	// 4. HTTP Polling - Medium priority
	httpFactory := NewHTTPPollingFactory(time.Second * 2)
	tm.connectionManager.RegisterFactory(httpFactory)

	// 5. LAN Broadcast - Lowest priority (last resort)
	lanFactory := NewLANBroadcastFactory(9999, 9998)
	tm.connectionManager.RegisterFactory(lanFactory)

	log.Println("All connection factories registered")
}

// ConnectToPeer attempts to connect to a peer using all available methods
func (tm *TransportManager) ConnectToPeer(peerID string, networkInfo map[string]interface{}) error {
	log.Printf("Initiating layered connection to peer %s", peerID)
	
	ctx := context.Background()
	return tm.connectionManager.ConnectToPeer(ctx, peerID, networkInfo)
}

// SendMessage sends a message to a peer
func (tm *TransportManager) SendMessage(peerID string, msgType string, data []byte) error {
	msg := Message{
		From:      tm.myPeerID,
		To:        peerID,
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	return tm.connectionManager.SendMessage(peerID, msg)
}

// SetMessageHandler sets the handler for incoming messages
func (tm *TransportManager) SetMessageHandler(handler func(msg Message)) {
	tm.connectionManager.SetMessageHandler(handler)
}

// SetConnectionHandler sets the handler for connection events
func (tm *TransportManager) SetConnectionHandler(handler func(peerID string, conn Connection)) {
	tm.connectionManager.SetConnectionHandler(func(peerID string, conn Connection) {
		log.Printf("Connected to %s via %s (quality: %d, latency: %v)", 
			peerID, conn.Type(), conn.Quality(), conn.Latency())
		handler(peerID, conn)
	})
}

// SetDisconnectHandler sets the handler for disconnect events
func (tm *TransportManager) SetDisconnectHandler(handler func(peerID string, connType ConnectionType)) {
	tm.connectionManager.SetDisconnectHandler(func(peerID string, connType ConnectionType) {
		log.Printf("Disconnected from %s (was using %s)", peerID, connType)
		handler(peerID, connType)
	})
}

// GetConnectionInfo returns connection information for a peer
func (tm *TransportManager) GetConnectionInfo(peerID string) map[string]interface{} {
	return tm.connectionManager.GetConnectionInfo(peerID)
}

// GetAllConnectionsInfo returns information about all connections
func (tm *TransportManager) GetAllConnectionsInfo() map[string]interface{} {
	allInfo := make(map[string]interface{})
	
	// This would need to be implemented in the connection manager
	// For now, return basic info
	allInfo["transport_manager"] = map[string]interface{}{
		"peer_id": tm.myPeerID,
		"status":  "active",
	}
	
	return allInfo
}

// Close closes all connections
func (tm *TransportManager) Close() error {
	log.Println("Shutting down transport manager...")
	return tm.connectionManager.Close()
}

// CreateNetworkInfo creates network info for connection attempts
func (tm *TransportManager) CreateNetworkInfo(peerIP, peerHTTPURL, networkType string) map[string]interface{} {
	networkInfo := map[string]interface{}{
		"network_type": networkType,
		"timestamp":    time.Now().Unix(),
	}

	if peerIP != "" {
		networkInfo["peer_ip"] = peerIP
		networkInfo["peer_http_url"] = fmt.Sprintf("http://%s:8080", peerIP)
	}

	if peerHTTPURL != "" {
		networkInfo["peer_http_url"] = peerHTTPURL
	}

	// Detect local network info
	if localIP := getLocalIP(); localIP != "" {
		networkInfo["local_ip"] = localIP
		
		if isLocalIP(localIP) {
			if networkType == "" {
				networkInfo["network_type"] = "lan"
			}
		}
	}

	// Detect NAT type (simplified)
	natType := detectNATType()
	networkInfo["nat_type"] = natType

	return networkInfo
}

// Helper functions
func getLocalIP() string {
	// Implementation to get local IP
	// This is a simplified version
	return "192.168.1.100" // Placeholder
}

func detectNATType() string {
	// Implementation to detect NAT type
	// This would involve STUN requests
	return "cone" // Placeholder
}
