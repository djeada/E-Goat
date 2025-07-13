// internal/transport/lan_broadcast.go
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// LANBroadcastFactory creates LAN broadcast connections
type LANBroadcastFactory struct {
	broadcastPort int
	listenPort    int
}

// NewLANBroadcastFactory creates a new LAN broadcast factory
func NewLANBroadcastFactory(broadcastPort, listenPort int) *LANBroadcastFactory {
	if broadcastPort == 0 {
		broadcastPort = 9999
	}
	if listenPort == 0 {
		listenPort = 9998
	}
	
	return &LANBroadcastFactory{
		broadcastPort: broadcastPort,
		listenPort:    listenPort,
	}
}

func (f *LANBroadcastFactory) CanCreate(connType ConnectionType) bool {
	return connType == LANBroadcast
}

func (f *LANBroadcastFactory) Priority() int {
	return 20 // Lowest priority - last resort
}

func (f *LANBroadcastFactory) EstimateSuccess(peerID string, networkInfo map[string]interface{}) int {
	// LAN broadcast works well on local networks
	if networkType, ok := networkInfo["network_type"].(string); ok {
		switch networkType {
		case "lan":
			return 80 // Good for LAN
		case "wifi":
			return 70 // Usually works on WiFi
		case "internet":
			return 0 // Won't work over internet
		}
	}
	
	// Check if we're on a local network
	if localIP, ok := networkInfo["local_ip"].(string); ok {
		if isLocalIP(localIP) {
			return 75
		}
	}
	
	return 50 // Default estimate
}

func (f *LANBroadcastFactory) Create(ctx context.Context, peerID string, config map[string]interface{}) (Connection, error) {
	conn := &LANBroadcastConnection{
		peerID:        peerID,
		factory:       f,
		status:        StatusConnecting,
		messages:      make(chan Message, 100),
		closeChan:     make(chan struct{}),
		peers:         make(map[string]string), // peerID -> IP mapping
	}

	// Start listening and discovery
	go conn.startListening(ctx)
	go conn.startDiscovery(ctx)

	return conn, nil
}

// LANBroadcastConnection implements Connection interface for LAN broadcast
type LANBroadcastConnection struct {
	peerID    string
	factory   *LANBroadcastFactory
	status    ConnectionStatus
	messages  chan Message
	closeChan chan struct{}
	mu        sync.RWMutex

	// Network components
	listener   *net.UDPConn
	broadcaster *net.UDPConn
	peers      map[string]string // peerID -> IP mapping

	// Quality metrics
	latency      time.Duration
	quality      int
	lastSeen     map[string]time.Time
	packetsSent  int
	packetsRecv  int
}

func (c *LANBroadcastConnection) startListening(ctx context.Context) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", c.factory.listenPort))
	if err != nil {
		log.Printf("Failed to resolve listen address: %v", err)
		c.mu.Lock()
		c.status = StatusFailed
		c.mu.Unlock()
		return
	}

	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("Failed to start UDP listener: %v", err)
		c.mu.Lock()
		c.status = StatusFailed
		c.mu.Unlock()
		return
	}

	c.mu.Lock()
	c.listener = listener
	c.status = StatusConnected
	c.quality = 70 // Good for local network
	c.lastSeen = make(map[string]time.Time)
	c.mu.Unlock()

	log.Printf("LAN broadcast listening on port %d", c.factory.listenPort)

	// Read incoming messages
	buffer := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closeChan:
			return
		default:
		}

		listener.SetReadDeadline(time.Now().Add(time.Second))
		n, clientAddr, err := listener.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			log.Printf("UDP read error: %v", err)
			continue
		}

		// Parse message
		var envelope struct {
			PeerID  string  `json:"peer_id"`
			Message Message `json:"message"`
		}

		if err := json.Unmarshal(buffer[:n], &envelope); err != nil {
			log.Printf("Failed to unmarshal UDP message: %v", err)
			continue
		}

		// Update peer mapping
		c.mu.Lock()
		c.peers[envelope.PeerID] = clientAddr.IP.String()
		c.lastSeen[envelope.PeerID] = time.Now()
		c.packetsRecv++
		c.mu.Unlock()

		// Handle ping/pong for latency measurement
		if envelope.Message.Type == "ping" {
			c.sendPong(envelope.PeerID, envelope.Message.Timestamp)
			continue
		} else if envelope.Message.Type == "pong" {
			c.handlePong(envelope.Message.Timestamp)
			continue
		}

		// Forward message
		if envelope.PeerID == c.peerID {
			continue // Ignore our own messages
		}

		select {
		case c.messages <- envelope.Message:
		default:
			log.Printf("Message buffer full for LAN broadcast connection")
		}
	}
}

func (c *LANBroadcastConnection) startDiscovery(ctx context.Context) {
	// Send periodic discovery broadcasts
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	// Start quality monitoring
	go c.startQualityMonitoring()

	for {
		select {
		case <-ticker.C:
			c.sendDiscovery()
		case <-ctx.Done():
			return
		case <-c.closeChan:
			return
		}
	}
}

func (c *LANBroadcastConnection) sendDiscovery() {
	discovery := Message{
		From:      c.peerID,
		Type:      "discovery",
		Data:      []byte("ping"),
		Timestamp: time.Now().Unix(),
	}

	c.broadcast(discovery)
}

func (c *LANBroadcastConnection) broadcast(msg Message) error {
	envelope := struct {
		PeerID  string  `json:"peer_id"`
		Message Message `json:"message"`
	}{
		PeerID:  c.peerID,
		Message: msg,
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	// Get broadcast addresses
	broadcastAddrs := c.getBroadcastAddresses()

	c.mu.Lock()
	packetsSent := c.packetsSent
	c.packetsSent++
	c.mu.Unlock()

	for _, addr := range broadcastAddrs {
		conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", addr, c.factory.broadcastPort))
		if err != nil {
			continue
		}

		conn.Write(data)
		conn.Close()
	}

	if packetsSent%10 == 0 {
		log.Printf("Sent %d LAN broadcast packets", packetsSent+1)
	}

	return nil
}

func (c *LANBroadcastConnection) getBroadcastAddresses() []string {
	var broadcasts []string

	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{"255.255.255.255"} // Fallback to global broadcast
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					// Calculate broadcast address
					broadcast := make(net.IP, 4)
					for i := range broadcast {
						broadcast[i] = ipnet.IP[i] | ^ipnet.Mask[i]
					}
					broadcasts = append(broadcasts, broadcast.String())
				}
			}
		}
	}

	if len(broadcasts) == 0 {
		broadcasts = []string{"255.255.255.255"}
	}

	return broadcasts
}

func (c *LANBroadcastConnection) startQualityMonitoring() {
	ticker := time.NewTicker(time.Second * 15)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.sendPing()
			c.updateQuality()
		case <-c.closeChan:
			return
		}
	}
}

func (c *LANBroadcastConnection) sendPing() {
	ping := Message{
		From:      c.peerID,
		Type:      "ping",
		Timestamp: time.Now().UnixNano(),
	}
	c.broadcast(ping)
}

func (c *LANBroadcastConnection) sendPong(toPeerID string, timestamp int64) {
	pong := Message{
		From:      c.peerID,
		To:        toPeerID,
		Type:      "pong",
		Timestamp: timestamp,
	}
	c.broadcast(pong)
}

func (c *LANBroadcastConnection) handlePong(timestamp int64) {
	now := time.Now().UnixNano()
	c.mu.Lock()
	c.latency = time.Duration(now - timestamp)
	c.mu.Unlock()
}

func (c *LANBroadcastConnection) updateQuality() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clean up old peers
	now := time.Now()
	activePeers := 0
	for peerID, lastSeen := range c.lastSeen {
		if now.Sub(lastSeen) > time.Minute*2 {
			delete(c.lastSeen, peerID)
			delete(c.peers, peerID)
		} else {
			activePeers++
		}
	}

	// Update quality based on network activity
	baseQuality := 70
	
	// More active peers = better network
	if activePeers > 3 {
		baseQuality += 10
	} else if activePeers == 0 {
		baseQuality -= 20
	}

	// Latency factor
	if c.latency > 0 {
		latencyMs := c.latency.Milliseconds()
		if latencyMs < 10 {
			baseQuality += 10
		} else if latencyMs > 100 {
			baseQuality -= 10
		}
	}

	// Packet loss estimate
	if c.packetsSent > 0 && c.packetsRecv > 0 {
		ratio := float64(c.packetsRecv) / float64(c.packetsSent)
		if ratio < 0.5 {
			baseQuality -= 20
		} else if ratio > 0.8 {
			baseQuality += 5
		}
	}

	if baseQuality < 0 {
		baseQuality = 0
	} else if baseQuality > 100 {
		baseQuality = 100
	}

	c.quality = baseQuality
}

func (c *LANBroadcastConnection) Send(msg Message) error {
	c.mu.RLock()
	status := c.status
	c.mu.RUnlock()

	if status != StatusConnected {
		return fmt.Errorf("LAN broadcast connection not established")
	}

	return c.broadcast(msg)
}

func (c *LANBroadcastConnection) Receive() <-chan Message {
	return c.messages
}

func (c *LANBroadcastConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.listener != nil {
		c.listener.Close()
		c.listener = nil
	}

	c.status = StatusDisconnected
	close(c.closeChan)
	close(c.messages)
	return nil
}

func (c *LANBroadcastConnection) Status() ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *LANBroadcastConnection) Type() ConnectionType {
	return LANBroadcast
}

func (c *LANBroadcastConnection) Latency() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latency
}

func (c *LANBroadcastConnection) Quality() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.quality
}

// Helper function to check if IP is local
func isLocalIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Check for private IP ranges
	private := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
	}

	for _, cidr := range private {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(parsedIP) {
			return true
		}
	}

	return false
}
