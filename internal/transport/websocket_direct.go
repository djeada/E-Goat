// internal/transport/websocket_direct.go
package transport

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketDirectFactory creates direct WebSocket connections
type WebSocketDirectFactory struct {
	defaultPorts []int
}

// NewWebSocketDirectFactory creates a new WebSocket direct factory
func NewWebSocketDirectFactory(ports []int) *WebSocketDirectFactory {
	if len(ports) == 0 {
		ports = []int{9000, 8080, 80} // Default ports to try
	}
	return &WebSocketDirectFactory{
		defaultPorts: ports,
	}
}

func (f *WebSocketDirectFactory) CanCreate(connType ConnectionType) bool {
	return connType == WebSocketDirect
}

func (f *WebSocketDirectFactory) Priority() int {
	return 60 // Medium priority
}

func (f *WebSocketDirectFactory) EstimateSuccess(peerID string, networkInfo map[string]interface{}) int {
	// Check if we have peer's IP and if ports might be open
	if peerIP, ok := networkInfo["peer_ip"].(string); ok && peerIP != "" {
		if networkType, ok := networkInfo["network_type"].(string); ok {
			switch networkType {
			case "lan":
				return 85 // High success on LAN
			case "internet":
				return 40 // Lower success due to firewalls
			}
		}
		return 65
	}
	return 20 // Low success without IP
}

func (f *WebSocketDirectFactory) Create(ctx context.Context, peerID string, config map[string]interface{}) (Connection, error) {
	networkInfo, _ := config["networkInfo"].(map[string]interface{})
	peerIP, ok := networkInfo["peer_ip"].(string)
	if !ok || peerIP == "" {
		return nil, fmt.Errorf("peer IP required for direct WebSocket connection")
	}

	conn := &WebSocketDirectConnection{
		peerID:    peerID,
		peerIP:    peerIP,
		factory:   f,
		status:    StatusConnecting,
		messages:  make(chan Message, 100),
		closeChan: make(chan struct{}),
	}

	// Try to connect on different ports
	go conn.connect(ctx)

	return conn, nil
}

// WebSocketDirectConnection implements Connection interface for direct WebSocket
type WebSocketDirectConnection struct {
	peerID    string
	peerIP    string
	factory   *WebSocketDirectFactory
	ws        *websocket.Conn
	status    ConnectionStatus
	messages  chan Message
	closeChan chan struct{}
	mu        sync.RWMutex

	// Quality metrics
	latency   time.Duration
	quality   int
	lastPing  time.Time
	connected time.Time
}

func (c *WebSocketDirectConnection) connect(ctx context.Context) {
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = time.Second * 10

	// Try each port until one succeeds
	for _, port := range c.factory.defaultPorts {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.status = StatusFailed
			c.mu.Unlock()
			return
		default:
		}

		wsURL := url.URL{
			Scheme: "ws",
			Host:   fmt.Sprintf("%s:%d", c.peerIP, port),
			Path:   "/direct",
		}

		log.Printf("Attempting direct WebSocket connection to %s", wsURL.String())

		ws, _, err := dialer.DialContext(ctx, wsURL.String(), nil)
		if err != nil {
			log.Printf("Failed to connect to %s: %v", wsURL.String(), err)
			continue
		}

		// Connection successful
		c.mu.Lock()
		c.ws = ws
		c.status = StatusConnected
		c.quality = 85 // High quality for direct connection
		c.connected = time.Now()
		c.mu.Unlock()

		log.Printf("Direct WebSocket connection to %s established", c.peerID)

		// Start message handling
		go c.readMessages()
		go c.startQualityMonitoring()

		return
	}

	// All ports failed
	c.mu.Lock()
	c.status = StatusFailed
	c.mu.Unlock()
	log.Printf("All direct WebSocket connection attempts to %s failed", c.peerID)
}

func (c *WebSocketDirectConnection) readMessages() {
	defer func() {
		c.mu.Lock()
		c.status = StatusDisconnected
		c.mu.Unlock()
		close(c.closeChan)
	}()

	for {
		c.mu.RLock()
		ws := c.ws
		c.mu.RUnlock()

		if ws == nil {
			return
		}

		var message Message
		err := ws.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error for %s: %v", c.peerID, err)
			}
			return
		}

		// Handle ping/pong for latency measurement
		if message.Type == "ping" {
			c.sendPong(message.Timestamp)
			continue
		} else if message.Type == "pong" {
			c.handlePong(message.Timestamp)
			continue
		}

		select {
		case c.messages <- message:
		default:
			log.Printf("Message buffer full for WebSocket connection to %s", c.peerID)
		}
	}
}

func (c *WebSocketDirectConnection) startQualityMonitoring() {
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
}

func (c *WebSocketDirectConnection) sendPing() {
	ping := Message{
		Type:      "ping",
		Timestamp: time.Now().UnixNano(),
	}
	c.Send(ping)
	c.mu.Lock()
	c.lastPing = time.Now()
	c.mu.Unlock()
}

func (c *WebSocketDirectConnection) sendPong(timestamp int64) {
	pong := Message{
		Type:      "pong",
		Timestamp: timestamp,
	}
	c.Send(pong)
}

func (c *WebSocketDirectConnection) handlePong(timestamp int64) {
	now := time.Now().UnixNano()
	c.mu.Lock()
	c.latency = time.Duration(now - timestamp)
	
	// Update quality based on latency
	latencyMs := c.latency.Milliseconds()
	if latencyMs < 20 {
		c.quality = 90
	} else if latencyMs < 50 {
		c.quality = 85
	} else if latencyMs < 100 {
		c.quality = 75
	} else if latencyMs < 200 {
		c.quality = 60
	} else {
		c.quality = 40
	}
	c.mu.Unlock()
}

func (c *WebSocketDirectConnection) Send(msg Message) error {
	c.mu.RLock()
	ws := c.ws
	status := c.status
	c.mu.RUnlock()

	if status != StatusConnected || ws == nil {
		return fmt.Errorf("WebSocket connection not established")
	}

	return ws.WriteJSON(msg)
}

func (c *WebSocketDirectConnection) Receive() <-chan Message {
	return c.messages
}

func (c *WebSocketDirectConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ws != nil {
		c.ws.Close()
		c.ws = nil
	}
	c.status = StatusDisconnected
	close(c.messages)
	return nil
}

func (c *WebSocketDirectConnection) Status() ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *WebSocketDirectConnection) Type() ConnectionType {
	return WebSocketDirect
}

func (c *WebSocketDirectConnection) Latency() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latency
}

func (c *WebSocketDirectConnection) Quality() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.quality
}
