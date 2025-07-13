// internal/transport/connection.go
package transport

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ConnectionType represents different connection methods
type ConnectionType string

const (
	WebRTCSTUN    ConnectionType = "webrtc-stun"
	WebRTCTURN    ConnectionType = "webrtc-turn"
	WebSocketDirect ConnectionType = "websocket-direct"
	HTTPPolling   ConnectionType = "http-polling"
	LANBroadcast  ConnectionType = "lan-broadcast"
)

// Message represents a message sent between peers
type Message struct {
	From      string                 `json:"from"`
	To        string                 `json:"to"`
	Type      string                 `json:"type"`
	Data      []byte                 `json:"data"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp int64                  `json:"timestamp"`
}

// Connection represents a single connection to a peer
type Connection interface {
	// Send sends a message to the peer
	Send(msg Message) error
	
	// Receive returns a channel for incoming messages
	Receive() <-chan Message
	
	// Close closes the connection
	Close() error
	
	// Status returns the current connection status
	Status() ConnectionStatus
	
	// Type returns the connection type
	Type() ConnectionType
	
	// Latency returns the current latency (if measurable)
	Latency() time.Duration
	
	// Quality returns connection quality score (0-100)
	Quality() int
}

// ConnectionStatus represents the status of a connection
type ConnectionStatus string

const (
	StatusConnecting  ConnectionStatus = "connecting"
	StatusConnected   ConnectionStatus = "connected"
	StatusDisconnected ConnectionStatus = "disconnected"
	StatusFailed      ConnectionStatus = "failed"
)

// ConnectionFactory creates connections of specific types
type ConnectionFactory interface {
	// CanCreate returns true if this factory can create the given connection type
	CanCreate(connType ConnectionType) bool
	
	// Create creates a new connection to the given peer
	Create(ctx context.Context, peerID string, config map[string]interface{}) (Connection, error)
	
	// Priority returns the priority of this connection type (higher = more preferred)
	Priority() int
	
	// EstimateSuccess estimates the probability of success (0-100)
	EstimateSuccess(peerID string, networkInfo map[string]interface{}) int
}

// LayeredConnectionManager manages multiple connection attempts with fallback
type LayeredConnectionManager struct {
	factories  []ConnectionFactory
	connections map[string]*PeerConnection
	mu         sync.RWMutex
	
	// Configuration
	maxRetries        int
	retryDelay        time.Duration
	connectionTimeout time.Duration
	
	// Callbacks
	onMessage     func(msg Message)
	onConnection  func(peerID string, conn Connection)
	onDisconnect  func(peerID string, connType ConnectionType)
}

// PeerConnection manages all connections to a single peer
type PeerConnection struct {
	PeerID      string
	Connections map[ConnectionType]Connection
	Primary     Connection
	mu          sync.RWMutex
	
	// Connection state
	isConnected bool
	lastMessage time.Time
	
	// Quality tracking
	latencyHistory []time.Duration
	qualityHistory []int
}

// NewLayeredConnectionManager creates a new connection manager
func NewLayeredConnectionManager() *LayeredConnectionManager {
	return &LayeredConnectionManager{
		factories:         make([]ConnectionFactory, 0),
		connections:       make(map[string]*PeerConnection),
		maxRetries:        3,
		retryDelay:        time.Second * 2,
		connectionTimeout: time.Second * 30,
	}
}

// RegisterFactory registers a new connection factory
func (lcm *LayeredConnectionManager) RegisterFactory(factory ConnectionFactory) {
	lcm.mu.Lock()
	defer lcm.mu.Unlock()
	
	// Insert in priority order (higher priority first)
	inserted := false
	for i, f := range lcm.factories {
		if factory.Priority() > f.Priority() {
			lcm.factories = append(lcm.factories[:i], append([]ConnectionFactory{factory}, lcm.factories[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		lcm.factories = append(lcm.factories, factory)
	}
	
	log.Printf("Registered connection factory with priority %d", factory.Priority())
}

// ConnectToPeer attempts to connect to a peer using all available methods
func (lcm *LayeredConnectionManager) ConnectToPeer(ctx context.Context, peerID string, networkInfo map[string]interface{}) error {
	lcm.mu.Lock()
	defer lcm.mu.Unlock()
	
	if _, exists := lcm.connections[peerID]; exists {
		return fmt.Errorf("already connected to peer %s", peerID)
	}
	
	peerConn := &PeerConnection{
		PeerID:      peerID,
		Connections: make(map[ConnectionType]Connection),
	}
	lcm.connections[peerID] = peerConn
	
	// Start connection attempts in parallel
	go lcm.attemptConnections(ctx, peerConn, networkInfo)
	
	return nil
}

// attemptConnections tries multiple connection methods with fallback
func (lcm *LayeredConnectionManager) attemptConnections(ctx context.Context, peerConn *PeerConnection, networkInfo map[string]interface{}) {
	log.Printf("Starting layered connection attempt to peer %s", peerConn.PeerID)
	
	// Create context with timeout
	connCtx, cancel := context.WithTimeout(ctx, lcm.connectionTimeout)
	defer cancel()
	
	// Channel to receive successful connections
	connChan := make(chan Connection, len(lcm.factories))
	errorChan := make(chan error, len(lcm.factories))
	
	// Try all connection methods in parallel, but with staggered start times
	for i, factory := range lcm.factories {
		go func(f ConnectionFactory, delay time.Duration) {
			// Stagger connection attempts to prefer higher priority methods
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-connCtx.Done():
					return
				}
			}
			
			// Estimate success probability
			successProb := f.EstimateSuccess(peerConn.PeerID, networkInfo)
			if successProb < 10 { // Skip very unlikely connections
				log.Printf("Skipping connection type with low success probability: %d%%", successProb)
				return
			}
			
			log.Printf("Attempting connection via factory (priority %d, success prob %d%%)", f.Priority(), successProb)
			
			// Try to create connection
			conn, err := f.Create(connCtx, peerConn.PeerID, map[string]interface{}{
				"networkInfo": networkInfo,
				"timeout":     lcm.connectionTimeout,
			})
			
			if err != nil {
				log.Printf("Connection attempt failed: %v", err)
				errorChan <- err
				return
			}
			
			// Wait for connection to establish
			ticker := time.NewTicker(time.Millisecond * 100)
			defer ticker.Stop()
			
			for {
				select {
				case <-connCtx.Done():
					conn.Close()
					return
				case <-ticker.C:
					if conn.Status() == StatusConnected {
						connChan <- conn
						return
					} else if conn.Status() == StatusFailed {
						errorChan <- fmt.Errorf("connection failed")
						return
					}
				}
			}
		}(factory, time.Duration(i)*time.Millisecond*500) // Stagger by 500ms
	}
	
	// Wait for first successful connection or all failures
	var firstConnection Connection
	errors := make([]error, 0)
	
	for i := 0; i < len(lcm.factories); i++ {
		select {
		case conn := <-connChan:
			if firstConnection == nil {
				firstConnection = conn
				lcm.setPrimaryConnection(peerConn, conn)
				log.Printf("Primary connection established to %s via %s", peerConn.PeerID, conn.Type())
			} else {
				// Add as backup connection
				lcm.addBackupConnection(peerConn, conn)
				log.Printf("Backup connection established to %s via %s", peerConn.PeerID, conn.Type())
			}
			
		case err := <-errorChan:
			errors = append(errors, err)
			
		case <-connCtx.Done():
			log.Printf("Connection timeout for peer %s", peerConn.PeerID)
			if firstConnection == nil {
				lcm.removeConnection(peerConn.PeerID)
				return
			}
		}
	}
	
	if firstConnection == nil {
		log.Printf("All connection attempts failed for peer %s: %v", peerConn.PeerID, errors)
		lcm.removeConnection(peerConn.PeerID)
		return
	}
	
	// Start monitoring connections
	go lcm.monitorPeerConnection(peerConn)
}

// setPrimaryConnection sets the primary connection for a peer
func (lcm *LayeredConnectionManager) setPrimaryConnection(peerConn *PeerConnection, conn Connection) {
	peerConn.mu.Lock()
	defer peerConn.mu.Unlock()
	
	peerConn.Primary = conn
	peerConn.Connections[conn.Type()] = conn
	peerConn.isConnected = true
	
	// Start message handling
	go lcm.handleMessages(peerConn, conn)
	
	if lcm.onConnection != nil {
		lcm.onConnection(peerConn.PeerID, conn)
	}
}

// addBackupConnection adds a backup connection
func (lcm *LayeredConnectionManager) addBackupConnection(peerConn *PeerConnection, conn Connection) {
	peerConn.mu.Lock()
	defer peerConn.mu.Unlock()
	
	peerConn.Connections[conn.Type()] = conn
	
	// Start message handling for backup too
	go lcm.handleMessages(peerConn, conn)
}

// handleMessages handles incoming messages from a connection
func (lcm *LayeredConnectionManager) handleMessages(peerConn *PeerConnection, conn Connection) {
	for msg := range conn.Receive() {
		peerConn.mu.Lock()
		peerConn.lastMessage = time.Now()
		peerConn.mu.Unlock()
		
		if lcm.onMessage != nil {
			lcm.onMessage(msg)
		}
	}
}

// monitorPeerConnection monitors connection health and handles failover
func (lcm *LayeredConnectionManager) monitorPeerConnection(peerConn *PeerConnection) {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			lcm.checkConnectionHealth(peerConn)
		}
	}
}

// checkConnectionHealth checks and manages connection health
func (lcm *LayeredConnectionManager) checkConnectionHealth(peerConn *PeerConnection) {
	peerConn.mu.Lock()
	defer peerConn.mu.Unlock()
	
	if peerConn.Primary == nil {
		return
	}
	
	// Check primary connection health
	if peerConn.Primary.Status() != StatusConnected {
		log.Printf("Primary connection to %s failed, attempting failover", peerConn.PeerID)
		
		// Find best backup connection
		var bestBackup Connection
		bestQuality := -1
		
		for connType, conn := range peerConn.Connections {
			if connType != peerConn.Primary.Type() && conn.Status() == StatusConnected {
				quality := conn.Quality()
				if quality > bestQuality {
					bestQuality = quality
					bestBackup = conn
				}
			}
		}
		
		if bestBackup != nil {
			log.Printf("Failing over to backup connection via %s", bestBackup.Type())
			oldPrimary := peerConn.Primary
			peerConn.Primary = bestBackup
			
			// Close old primary
			oldPrimary.Close()
			delete(peerConn.Connections, oldPrimary.Type())
			
			if lcm.onConnection != nil {
				lcm.onConnection(peerConn.PeerID, bestBackup)
			}
		} else {
			log.Printf("No backup connections available for %s", peerConn.PeerID)
			peerConn.isConnected = false
			
			if lcm.onDisconnect != nil {
				lcm.onDisconnect(peerConn.PeerID, peerConn.Primary.Type())
			}
		}
	}
	
	// Update quality metrics
	if peerConn.Primary != nil {
		latency := peerConn.Primary.Latency()
		quality := peerConn.Primary.Quality()
		
		peerConn.latencyHistory = append(peerConn.latencyHistory, latency)
		peerConn.qualityHistory = append(peerConn.qualityHistory, quality)
		
		// Keep only recent history (last 10 measurements)
		if len(peerConn.latencyHistory) > 10 {
			peerConn.latencyHistory = peerConn.latencyHistory[1:]
		}
		if len(peerConn.qualityHistory) > 10 {
			peerConn.qualityHistory = peerConn.qualityHistory[1:]
		}
	}
}

// SendMessage sends a message to a peer using the best available connection
func (lcm *LayeredConnectionManager) SendMessage(peerID string, msg Message) error {
	lcm.mu.RLock()
	peerConn, exists := lcm.connections[peerID]
	lcm.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("no connection to peer %s", peerID)
	}
	
	peerConn.mu.RLock()
	primary := peerConn.Primary
	peerConn.mu.RUnlock()
	
	if primary == nil {
		return fmt.Errorf("no active connection to peer %s", peerID)
	}
	
	return primary.Send(msg)
}

// GetConnectionInfo returns information about connections to a peer
func (lcm *LayeredConnectionManager) GetConnectionInfo(peerID string) map[string]interface{} {
	lcm.mu.RLock()
	peerConn, exists := lcm.connections[peerID]
	lcm.mu.RUnlock()
	
	if !exists {
		return nil
	}
	
	peerConn.mu.RLock()
	defer peerConn.mu.RUnlock()
	
	info := map[string]interface{}{
		"peer_id":     peerConn.PeerID,
		"connected":   peerConn.isConnected,
		"connections": make(map[string]interface{}),
	}
	
	if peerConn.Primary != nil {
		info["primary_type"] = string(peerConn.Primary.Type())
		info["primary_quality"] = peerConn.Primary.Quality()
		info["primary_latency"] = peerConn.Primary.Latency().String()
	}
	
	connections := make(map[string]interface{})
	for connType, conn := range peerConn.Connections {
		connections[string(connType)] = map[string]interface{}{
			"status":  string(conn.Status()),
			"quality": conn.Quality(),
			"latency": conn.Latency().String(),
		}
	}
	info["connections"] = connections
	
	return info
}

// removeConnection removes a peer connection
func (lcm *LayeredConnectionManager) removeConnection(peerID string) {
	lcm.mu.Lock()
	defer lcm.mu.Unlock()
	
	if peerConn, exists := lcm.connections[peerID]; exists {
		peerConn.mu.Lock()
		for _, conn := range peerConn.Connections {
			conn.Close()
		}
		peerConn.mu.Unlock()
		delete(lcm.connections, peerID)
	}
}

// SetMessageHandler sets the message handler
func (lcm *LayeredConnectionManager) SetMessageHandler(handler func(msg Message)) {
	lcm.onMessage = handler
}

// SetConnectionHandler sets the connection event handler
func (lcm *LayeredConnectionManager) SetConnectionHandler(handler func(peerID string, conn Connection)) {
	lcm.onConnection = handler
}

// SetDisconnectHandler sets the disconnect event handler
func (lcm *LayeredConnectionManager) SetDisconnectHandler(handler func(peerID string, connType ConnectionType)) {
	lcm.onDisconnect = handler
}

// Close closes all connections
func (lcm *LayeredConnectionManager) Close() error {
	lcm.mu.Lock()
	defer lcm.mu.Unlock()
	
	for peerID := range lcm.connections {
		lcm.removeConnection(peerID)
	}
	
	return nil
}
