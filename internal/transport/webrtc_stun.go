// internal/transport/webrtc_stun.go
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

// WebRTCSTUNFactory creates WebRTC connections using STUN servers
type WebRTCSTUNFactory struct {
	stunServers []string
	config      webrtc.Configuration
}

// NewWebRTCSTUNFactory creates a new WebRTC STUN factory
func NewWebRTCSTUNFactory(stunServers []string) *WebRTCSTUNFactory {
	iceServers := make([]webrtc.ICEServer, len(stunServers))
	for i, server := range stunServers {
		iceServers[i] = webrtc.ICEServer{URLs: []string{server}}
	}

	return &WebRTCSTUNFactory{
		stunServers: stunServers,
		config: webrtc.Configuration{
			ICEServers: iceServers,
		},
	}
}

func (f *WebRTCSTUNFactory) CanCreate(connType ConnectionType) bool {
	return connType == WebRTCSTUN
}

func (f *WebRTCSTUNFactory) Priority() int {
	return 100 // Highest priority - direct P2P
}

func (f *WebRTCSTUNFactory) EstimateSuccess(peerID string, networkInfo map[string]interface{}) int {
	// Check if we're behind symmetric NAT or firewall
	if natType, ok := networkInfo["nat_type"].(string); ok {
		switch natType {
		case "open":
			return 95
		case "cone":
			return 85
		case "symmetric":
			return 40
		case "blocked":
			return 10
		}
	}
	return 70 // Default estimate
}

func (f *WebRTCSTUNFactory) Create(ctx context.Context, peerID string, config map[string]interface{}) (Connection, error) {
	conn := &WebRTCSTUNConnection{
		peerID:    peerID,
		factory:   f,
		status:    StatusConnecting,
		messages:  make(chan Message, 100),
		closeChan: make(chan struct{}),
	}

	// Create peer connection
	pc, err := webrtc.NewPeerConnection(f.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}
	conn.pc = pc

	// Set up data channel
	dc, err := pc.CreateDataChannel("data", nil)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to create data channel: %w", err)
	}
	conn.dataChannel = dc

	// Set up handlers
	conn.setupHandlers()

	// Start connection process
	go conn.connect(ctx)

	return conn, nil
}

// WebRTCSTUNConnection implements Connection interface for WebRTC STUN
type WebRTCSTUNConnection struct {
	peerID      string
	factory     *WebRTCSTUNFactory
	pc          *webrtc.PeerConnection
	dataChannel *webrtc.DataChannel
	status      ConnectionStatus
	messages    chan Message
	closeChan   chan struct{}
	mu          sync.RWMutex

	// Quality metrics
	latency    time.Duration
	quality    int
	lastPing   time.Time
	packetLoss float64
}

func (c *WebRTCSTUNConnection) setupHandlers() {
	// Connection state changes
	c.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.mu.Lock()
		defer c.mu.Unlock()

		switch state {
		case webrtc.PeerConnectionStateConnected:
			c.status = StatusConnected
			c.quality = 90
			log.Printf("WebRTC STUN connection to %s established", c.peerID)
		case webrtc.PeerConnectionStateDisconnected:
			c.status = StatusDisconnected
		case webrtc.PeerConnectionStateFailed:
			c.status = StatusFailed
		case webrtc.PeerConnectionStateClosed:
			c.status = StatusDisconnected
			close(c.closeChan)
		}
	})

	// Data channel handlers
	c.dataChannel.OnOpen(func() {
		log.Printf("WebRTC data channel to %s opened", c.peerID)
		c.startQualityMonitoring()
	})

	c.dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		var message Message
		if err := json.Unmarshal(msg.Data, &message); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			return
		}

		// Handle ping/pong for latency measurement
		if message.Type == "ping" {
			c.sendPong(message.Timestamp)
			return
		} else if message.Type == "pong" {
			c.handlePong(message.Timestamp)
			return
		}

		select {
		case c.messages <- message:
		default:
			log.Printf("Message buffer full for WebRTC connection to %s", c.peerID)
		}
	})
}

func (c *WebRTCSTUNConnection) connect(ctx context.Context) {
	// This would handle the full WebRTC negotiation process
	// For now, simplified version
	offer, err := c.pc.CreateOffer(nil)
	if err != nil {
		c.mu.Lock()
		c.status = StatusFailed
		c.mu.Unlock()
		return
	}

	if err := c.pc.SetLocalDescription(offer); err != nil {
		c.mu.Lock()
		c.status = StatusFailed
		c.mu.Unlock()
		return
	}

	// In real implementation, would exchange offer/answer via signaling server
	// For now, mark as connected after a delay
	time.AfterFunc(time.Second*2, func() {
		c.mu.Lock()
		if c.status == StatusConnecting {
			c.status = StatusConnected
		}
		c.mu.Unlock()
	})
}

func (c *WebRTCSTUNConnection) startQualityMonitoring() {
	go func() {
		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.sendPing()
			case <-c.closeChan:
				return
			}
		}
	}()
}

func (c *WebRTCSTUNConnection) sendPing() {
	ping := Message{
		Type:      "ping",
		Timestamp: time.Now().UnixNano(),
	}
	c.Send(ping)
	c.mu.Lock()
	c.lastPing = time.Now()
	c.mu.Unlock()
}

func (c *WebRTCSTUNConnection) sendPong(timestamp int64) {
	pong := Message{
		Type:      "pong",
		Timestamp: timestamp,
	}
	c.Send(pong)
}

func (c *WebRTCSTUNConnection) handlePong(timestamp int64) {
	now := time.Now().UnixNano()
	c.mu.Lock()
	c.latency = time.Duration(now - timestamp)
	
	// Update quality based on latency
	latencyMs := c.latency.Milliseconds()
	if latencyMs < 50 {
		c.quality = 95
	} else if latencyMs < 100 {
		c.quality = 85
	} else if latencyMs < 200 {
		c.quality = 70
	} else if latencyMs < 500 {
		c.quality = 50
	} else {
		c.quality = 20
	}
	c.mu.Unlock()
}

func (c *WebRTCSTUNConnection) Send(msg Message) error {
	c.mu.RLock()
	status := c.status
	c.mu.RUnlock()

	if status != StatusConnected {
		return fmt.Errorf("connection not established")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return c.dataChannel.Send(data)
}

func (c *WebRTCSTUNConnection) Receive() <-chan Message {
	return c.messages
}

func (c *WebRTCSTUNConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pc != nil {
		c.pc.Close()
	}
	c.status = StatusDisconnected
	close(c.messages)
	return nil
}

func (c *WebRTCSTUNConnection) Status() ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *WebRTCSTUNConnection) Type() ConnectionType {
	return WebRTCSTUN
}

func (c *WebRTCSTUNConnection) Latency() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latency
}

func (c *WebRTCSTUNConnection) Quality() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.quality
}
