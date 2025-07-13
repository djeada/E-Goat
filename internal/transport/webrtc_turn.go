// internal/transport/webrtc_turn.go
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

// WebRTCTURNFactory creates WebRTC connections using TURN servers
type WebRTCTURNFactory struct {
	turnServers []TURNServer
	config      webrtc.Configuration
}

type TURNServer struct {
	URL      string
	Username string
	Password string
}

// NewWebRTCTURNFactory creates a new WebRTC TURN factory
func NewWebRTCTURNFactory(turnServers []TURNServer) *WebRTCTURNFactory {
	iceServers := make([]webrtc.ICEServer, len(turnServers))
	for i, server := range turnServers {
		iceServers[i] = webrtc.ICEServer{
			URLs:       []string{server.URL},
			Username:   server.Username,
			Credential: server.Password,
		}
	}

	return &WebRTCTURNFactory{
		turnServers: turnServers,
		config: webrtc.Configuration{
			ICEServers: iceServers,
		},
	}
}

func (f *WebRTCTURNFactory) CanCreate(connType ConnectionType) bool {
	return connType == WebRTCTURN
}

func (f *WebRTCTURNFactory) Priority() int {
	return 80 // High priority, fallback from STUN
}

func (f *WebRTCTURNFactory) EstimateSuccess(peerID string, networkInfo map[string]interface{}) int {
	// TURN should work in most scenarios, but uses server resources
	if natType, ok := networkInfo["nat_type"].(string); ok {
		switch natType {
		case "open":
			return 85 // STUN would be better
		case "cone":
			return 90
		case "symmetric":
			return 95 // TURN is ideal for symmetric NAT
		case "blocked":
			return 85
		}
	}
	return 90 // High success rate with TURN
}

func (f *WebRTCTURNFactory) Create(ctx context.Context, peerID string, config map[string]interface{}) (Connection, error) {
	conn := &WebRTCTURNConnection{
		peerID:    peerID,
		factory:   f,
		status:    StatusConnecting,
		messages:  make(chan Message, 100),
		closeChan: make(chan struct{}),
	}

	// Create peer connection with TURN
	pc, err := webrtc.NewPeerConnection(f.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create TURN peer connection: %w", err)
	}
	conn.pc = pc

	// Set up data channel
	dc, err := pc.CreateDataChannel("data", nil)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to create TURN data channel: %w", err)
	}
	conn.dataChannel = dc

	// Set up handlers
	conn.setupHandlers()

	// Start connection process
	go conn.connect(ctx)

	return conn, nil
}

// WebRTCTURNConnection implements Connection interface for WebRTC TURN
type WebRTCTURNConnection struct {
	peerID      string
	factory     *WebRTCTURNFactory
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
}

func (c *WebRTCTURNConnection) setupHandlers() {
	c.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.mu.Lock()
		defer c.mu.Unlock()

		switch state {
		case webrtc.PeerConnectionStateConnected:
			c.status = StatusConnected
			c.quality = 75 // Lower than STUN due to server relay
			log.Printf("WebRTC TURN connection to %s established", c.peerID)
		case webrtc.PeerConnectionStateDisconnected:
			c.status = StatusDisconnected
		case webrtc.PeerConnectionStateFailed:
			c.status = StatusFailed
		case webrtc.PeerConnectionStateClosed:
			c.status = StatusDisconnected
			close(c.closeChan)
		}
	})

	c.dataChannel.OnOpen(func() {
		log.Printf("WebRTC TURN data channel to %s opened", c.peerID)
		c.startQualityMonitoring()
	})

	c.dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		var message Message
		if err := json.Unmarshal(msg.Data, &message); err != nil {
			log.Printf("Failed to unmarshal TURN message: %v", err)
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
			log.Printf("Message buffer full for WebRTC TURN connection to %s", c.peerID)
		}
	})
}

func (c *WebRTCTURNConnection) connect(ctx context.Context) {
	// Simplified connection process
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

	// Simulate connection establishment
	time.AfterFunc(time.Second*3, func() {
		c.mu.Lock()
		if c.status == StatusConnecting {
			c.status = StatusConnected
		}
		c.mu.Unlock()
	})
}

func (c *WebRTCTURNConnection) startQualityMonitoring() {
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

func (c *WebRTCTURNConnection) sendPing() {
	ping := Message{
		Type:      "ping",
		Timestamp: time.Now().UnixNano(),
	}
	c.Send(ping)
	c.mu.Lock()
	c.lastPing = time.Now()
	c.mu.Unlock()
}

func (c *WebRTCTURNConnection) sendPong(timestamp int64) {
	pong := Message{
		Type:      "pong",
		Timestamp: timestamp,
	}
	c.Send(pong)
}

func (c *WebRTCTURNConnection) handlePong(timestamp int64) {
	now := time.Now().UnixNano()
	c.mu.Lock()
	c.latency = time.Duration(now - timestamp)
	
	// Update quality based on latency (TURN adds overhead)
	latencyMs := c.latency.Milliseconds()
	if latencyMs < 100 {
		c.quality = 80
	} else if latencyMs < 200 {
		c.quality = 70
	} else if latencyMs < 300 {
		c.quality = 60
	} else if latencyMs < 500 {
		c.quality = 40
	} else {
		c.quality = 20
	}
	c.mu.Unlock()
}

func (c *WebRTCTURNConnection) Send(msg Message) error {
	c.mu.RLock()
	status := c.status
	c.mu.RUnlock()

	if status != StatusConnected {
		return fmt.Errorf("TURN connection not established")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return c.dataChannel.Send(data)
}

func (c *WebRTCTURNConnection) Receive() <-chan Message {
	return c.messages
}

func (c *WebRTCTURNConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pc != nil {
		c.pc.Close()
	}
	c.status = StatusDisconnected
	close(c.messages)
	return nil
}

func (c *WebRTCTURNConnection) Status() ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *WebRTCTURNConnection) Type() ConnectionType {
	return WebRTCTURN
}

func (c *WebRTCTURNConnection) Latency() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latency
}

func (c *WebRTCTURNConnection) Quality() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.quality
}
