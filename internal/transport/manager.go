// internal/transport/manager.go
package transport

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// TransportManager provides a simple interface to the layered connection system
type TransportManager struct {
	connectionManager *LayeredConnectionManager
	myPeerID          string
	strategies        map[string]TransportStrategy
	currentStrategy   string
}

// NewTransportManager creates a new transport manager with all connection types
func NewTransportManager(peerID string) *TransportManager {
	tm := &TransportManager{
		connectionManager: NewLayeredConnectionManager(),
		myPeerID:          peerID,
		strategies:        make(map[string]TransportStrategy),
	}

	// Register all connection factories in priority order
	tm.registerFactories()
	tm.registerDefaultStrategies()
	_ = tm.SetStrategy("auto")

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

func (tm *TransportManager) registerDefaultStrategies() {
	for _, strategy := range defaultStrategies() {
		tm.RegisterStrategy(strategy)
	}
}

// RegisterStrategy registers a new transport strategy.
func (tm *TransportManager) RegisterStrategy(strategy TransportStrategy) {
	tm.strategies[strategy.Name()] = strategy
}

// ListStrategies returns available transport strategies.
func (tm *TransportManager) ListStrategies() []TransportStrategy {
	out := make([]TransportStrategy, 0, len(tm.strategies))
	for _, strategy := range tm.strategies {
		out = append(out, strategy)
	}
	return out
}

// GetStrategy returns the current strategy, if set.
func (tm *TransportManager) GetStrategy() (TransportStrategy, bool) {
	strategy, ok := tm.strategies[tm.currentStrategy]
	return strategy, ok
}

// SetStrategy changes the active strategy and updates allowed connection types.
func (tm *TransportManager) SetStrategy(name string) error {
	strategy, ok := tm.strategies[name]
	if !ok {
		return fmt.Errorf("unknown strategy: %s", name)
	}
	tm.currentStrategy = name
	tm.connectionManager.SetAllowedTypes(strategy.AllowedTypes())
	return nil
}

// TransportOption describes a transport's availability estimate.
type TransportOption struct {
	Type             string `json:"type"`
	Priority         int    `json:"priority"`
	EstimatedSuccess int    `json:"estimated_success"`
	Enabled          bool   `json:"enabled"`
}

// StrategyInfo is a serializable view of a strategy.
type StrategyInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	AllowedTypes []string `json:"allowed_types"`
}

// ListTransportOptions lists transports with availability estimates.
func (tm *TransportManager) ListTransportOptions(networkInfo map[string]interface{}) []TransportOption {
	allowed := make(map[ConnectionType]bool)
	if strategy, ok := tm.GetStrategy(); ok {
		for _, t := range strategy.AllowedTypes() {
			allowed[t] = true
		}
	} else {
		for _, t := range allConnectionTypes() {
			allowed[t] = true
		}
	}

	factories := tm.connectionManager.ListFactories()
	options := make([]TransportOption, 0, len(factories))
	for _, factory := range factories {
		provider, ok := factory.(ConnectionTypeProvider)
		if !ok {
			continue
		}
		connType := provider.Type()
		options = append(options, TransportOption{
			Type:             string(connType),
			Priority:         factory.Priority(),
			EstimatedSuccess: factory.EstimateSuccess(tm.myPeerID, networkInfo),
			Enabled:          allowed[connType],
		})
	}
	return options
}

// ListStrategyInfos returns a serializable list of strategies.
func (tm *TransportManager) ListStrategyInfos() []StrategyInfo {
	strategies := tm.ListStrategies()
	out := make([]StrategyInfo, 0, len(strategies))
	for _, strategy := range strategies {
		types := strategy.AllowedTypes()
		typeNames := make([]string, 0, len(types))
		for _, t := range types {
			typeNames = append(typeNames, string(t))
		}
		out = append(out, StrategyInfo{
			Name:         strategy.Name(),
			Description:  strategy.Description(),
			AllowedTypes: typeNames,
		})
	}
	return out
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

// ====== ENHANCED TRANSPORT FEATURES ======

// ConnectionPool manages a pool of reusable connections
type ConnectionPool struct {
	mu          sync.RWMutex
	connections map[string][]Connection
	maxPerPeer  int
	maxTotal    int
	totalCount  int
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(maxPerPeer, maxTotal int) *ConnectionPool {
	if maxPerPeer <= 0 {
		maxPerPeer = 3
	}
	if maxTotal <= 0 {
		maxTotal = 100
	}
	return &ConnectionPool{
		connections: make(map[string][]Connection),
		maxPerPeer:  maxPerPeer,
		maxTotal:    maxTotal,
	}
}

// Get retrieves a connection from the pool
func (cp *ConnectionPool) Get(peerID string) (Connection, bool) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conns, ok := cp.connections[peerID]; ok && len(conns) > 0 {
		// Get the last connection (most recently added)
		conn := conns[len(conns)-1]
		cp.connections[peerID] = conns[:len(conns)-1]
		cp.totalCount--
		return conn, true
	}
	return nil, false
}

// Put adds a connection to the pool
func (cp *ConnectionPool) Put(peerID string, conn Connection) bool {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Check if pool is full
	if cp.totalCount >= cp.maxTotal {
		return false
	}

	// Check if peer has too many connections
	if len(cp.connections[peerID]) >= cp.maxPerPeer {
		return false
	}

	cp.connections[peerID] = append(cp.connections[peerID], conn)
	cp.totalCount++
	return true
}

// Size returns the total number of pooled connections
func (cp *ConnectionPool) Size() int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.totalCount
}

// Clear closes all pooled connections
func (cp *ConnectionPool) Clear() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for _, conns := range cp.connections {
		for _, conn := range conns {
			conn.Close()
		}
	}
	cp.connections = make(map[string][]Connection)
	cp.totalCount = 0
}

// ReconnectionManager handles automatic reconnection with exponential backoff
type ReconnectionManager struct {
	mu            sync.RWMutex
	attempts      map[string]int
	lastAttempt   map[string]time.Time
	maxAttempts   int
	baseDelay     time.Duration
	maxDelay      time.Duration
	onReconnect   func(peerID string, conn Connection)
}

// NewReconnectionManager creates a new reconnection manager
func NewReconnectionManager(maxAttempts int, baseDelay, maxDelay time.Duration) *ReconnectionManager {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if baseDelay <= 0 {
		baseDelay = time.Second
	}
	if maxDelay <= 0 {
		maxDelay = time.Minute
	}
	return &ReconnectionManager{
		attempts:    make(map[string]int),
		lastAttempt: make(map[string]time.Time),
		maxAttempts: maxAttempts,
		baseDelay:   baseDelay,
		maxDelay:    maxDelay,
	}
}

// SetReconnectHandler sets the callback for successful reconnections
func (rm *ReconnectionManager) SetReconnectHandler(handler func(peerID string, conn Connection)) {
	rm.onReconnect = handler
}

// ShouldReconnect determines if a reconnection should be attempted
func (rm *ReconnectionManager) ShouldReconnect(peerID string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	attempts := rm.attempts[peerID]
	return attempts < rm.maxAttempts
}

// GetBackoffDelay returns the current backoff delay for a peer
func (rm *ReconnectionManager) GetBackoffDelay(peerID string) time.Duration {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	attempts := rm.attempts[peerID]
	delay := rm.baseDelay * time.Duration(1<<uint(attempts)) // Exponential backoff

	if delay > rm.maxDelay {
		delay = rm.maxDelay
	}
	return delay
}

// RecordAttempt records a reconnection attempt
func (rm *ReconnectionManager) RecordAttempt(peerID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.attempts[peerID]++
	rm.lastAttempt[peerID] = time.Now()
}

// RecordSuccess resets the attempt counter on successful reconnection
func (rm *ReconnectionManager) RecordSuccess(peerID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	delete(rm.attempts, peerID)
	delete(rm.lastAttempt, peerID)
}

// GetAttemptCount returns the number of attempts for a peer
func (rm *ReconnectionManager) GetAttemptCount(peerID string) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.attempts[peerID]
}

// ConnectionStats tracks connection statistics
type ConnectionStats struct {
	mu sync.RWMutex
	
	// Per-connection type stats
	typeStats map[ConnectionType]*TypeStats
	
	// Global stats
	totalConnections     int64
	successfulConns      int64
	failedConns          int64
	activeConns          int64
	totalMessagesSent    int64
	totalMessagesRecv    int64
	totalBytesSent       int64
	totalBytesRecv       int64
}

// TypeStats contains statistics for a specific connection type
type TypeStats struct {
	Attempts     int64         `json:"attempts"`
	Successes    int64         `json:"successes"`
	Failures     int64         `json:"failures"`
	Active       int64         `json:"active"`
	AvgLatency   time.Duration `json:"avg_latency"`
	latencies    []time.Duration
}

// NewConnectionStats creates a new connection stats tracker
func NewConnectionStats() *ConnectionStats {
	return &ConnectionStats{
		typeStats: make(map[ConnectionType]*TypeStats),
	}
}

// RecordConnectionAttempt records a connection attempt
func (cs *ConnectionStats) RecordConnectionAttempt(connType ConnectionType, success bool, latency time.Duration) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.totalConnections++
	
	if _, ok := cs.typeStats[connType]; !ok {
		cs.typeStats[connType] = &TypeStats{
			latencies: make([]time.Duration, 0, 100),
		}
	}
	
	stats := cs.typeStats[connType]
	stats.Attempts++
	
	if success {
		cs.successfulConns++
		cs.activeConns++ // Increment global active connections
		stats.Successes++
		stats.Active++
		
		// Track latency
		stats.latencies = append(stats.latencies, latency)
		if len(stats.latencies) > 100 {
			stats.latencies = stats.latencies[1:]
		}
		
		// Calculate average latency
		var total time.Duration
		for _, l := range stats.latencies {
			total += l
		}
		stats.AvgLatency = total / time.Duration(len(stats.latencies))
	} else {
		cs.failedConns++
		stats.Failures++
	}
}

// RecordDisconnection records a disconnection
func (cs *ConnectionStats) RecordDisconnection(connType ConnectionType) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.activeConns--
	if stats, ok := cs.typeStats[connType]; ok {
		stats.Active--
	}
}

// RecordMessageSent records a sent message
func (cs *ConnectionStats) RecordMessageSent(bytes int64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.totalMessagesSent++
	cs.totalBytesSent += bytes
}

// RecordMessageReceived records a received message
func (cs *ConnectionStats) RecordMessageReceived(bytes int64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.totalMessagesRecv++
	cs.totalBytesRecv += bytes
}

// StatsSnapshot represents a snapshot of connection stats
type StatsSnapshot struct {
	TotalConnections  int64                     `json:"total_connections"`
	SuccessfulConns   int64                     `json:"successful_connections"`
	FailedConns       int64                     `json:"failed_connections"`
	ActiveConns       int64                     `json:"active_connections"`
	SuccessRate       float64                   `json:"success_rate"`
	TotalMessagesSent int64                     `json:"total_messages_sent"`
	TotalMessagesRecv int64                     `json:"total_messages_received"`
	TotalBytesSent    int64                     `json:"total_bytes_sent"`
	TotalBytesRecv    int64                     `json:"total_bytes_received"`
	ByType            map[string]TypeStatsView  `json:"by_type"`
}

// TypeStatsView is a JSON-serializable view of TypeStats
type TypeStatsView struct {
	Attempts    int64   `json:"attempts"`
	Successes   int64   `json:"successes"`
	Failures    int64   `json:"failures"`
	Active      int64   `json:"active"`
	SuccessRate float64 `json:"success_rate"`
	AvgLatencyMs int64  `json:"avg_latency_ms"`
}

// GetSnapshot returns a snapshot of current stats
func (cs *ConnectionStats) GetSnapshot() *StatsSnapshot {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	snapshot := &StatsSnapshot{
		TotalConnections:  cs.totalConnections,
		SuccessfulConns:   cs.successfulConns,
		FailedConns:       cs.failedConns,
		ActiveConns:       cs.activeConns,
		TotalMessagesSent: cs.totalMessagesSent,
		TotalMessagesRecv: cs.totalMessagesRecv,
		TotalBytesSent:    cs.totalBytesSent,
		TotalBytesRecv:    cs.totalBytesRecv,
		ByType:            make(map[string]TypeStatsView),
	}

	if cs.totalConnections > 0 {
		snapshot.SuccessRate = float64(cs.successfulConns) / float64(cs.totalConnections) * 100
	}

	for connType, stats := range cs.typeStats {
		view := TypeStatsView{
			Attempts:     stats.Attempts,
			Successes:    stats.Successes,
			Failures:     stats.Failures,
			Active:       stats.Active,
			AvgLatencyMs: stats.AvgLatency.Milliseconds(),
		}
		if stats.Attempts > 0 {
			view.SuccessRate = float64(stats.Successes) / float64(stats.Attempts) * 100
		}
		snapshot.ByType[string(connType)] = view
	}

	return snapshot
}

// BandwidthEstimator estimates available bandwidth
type BandwidthEstimator struct {
	mu            sync.RWMutex
	samples       []bandwidthSample
	maxSamples    int
	estimatedBps  float64
}

type bandwidthSample struct {
	bytes     int64
	duration  time.Duration
	timestamp time.Time
}

// NewBandwidthEstimator creates a new bandwidth estimator
func NewBandwidthEstimator() *BandwidthEstimator {
	return &BandwidthEstimator{
		samples:    make([]bandwidthSample, 0, 100),
		maxSamples: 100,
	}
}

// RecordTransfer records a data transfer for bandwidth estimation
func (be *BandwidthEstimator) RecordTransfer(bytes int64, duration time.Duration) {
	be.mu.Lock()
	defer be.mu.Unlock()

	sample := bandwidthSample{
		bytes:     bytes,
		duration:  duration,
		timestamp: time.Now(),
	}

	be.samples = append(be.samples, sample)
	if len(be.samples) > be.maxSamples {
		be.samples = be.samples[1:]
	}

	// Recalculate estimate
	be.recalculate()
}

func (be *BandwidthEstimator) recalculate() {
	if len(be.samples) == 0 {
		be.estimatedBps = 0
		return
	}

	// Use exponential moving average with more weight on recent samples
	var weightedSum, weightSum float64
	for i, sample := range be.samples {
		if sample.duration > 0 {
			bps := float64(sample.bytes) / sample.duration.Seconds()
			weight := float64(i + 1) // More weight to newer samples
			weightedSum += bps * weight
			weightSum += weight
		}
	}

	if weightSum > 0 {
		be.estimatedBps = weightedSum / weightSum
	}
}

// GetEstimate returns the estimated bandwidth in bits per second
func (be *BandwidthEstimator) GetEstimate() float64 {
	be.mu.RLock()
	defer be.mu.RUnlock()
	return be.estimatedBps * 8 // Convert bytes to bits
}

// GetEstimateMbps returns the estimated bandwidth in megabits per second
func (be *BandwidthEstimator) GetEstimateMbps() float64 {
	return be.GetEstimate() / 1_000_000
}
