// internal/transport/http_polling.go
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// HTTPPollingFactory creates HTTP polling connections
type HTTPPollingFactory struct {
	client   *http.Client
	pollRate time.Duration
}

// NewHTTPPollingFactory creates a new HTTP polling factory
func NewHTTPPollingFactory(pollRate time.Duration) *HTTPPollingFactory {
	if pollRate == 0 {
		pollRate = time.Second * 2 // Default 2 second polling
	}
	
	return &HTTPPollingFactory{
		client: &http.Client{
			Timeout: time.Second * 10,
		},
		pollRate: pollRate,
	}
}

func (f *HTTPPollingFactory) CanCreate(connType ConnectionType) bool {
	return connType == HTTPPolling
}

func (f *HTTPPollingFactory) Priority() int {
	return 40 // Lower priority fallback
}

func (f *HTTPPollingFactory) EstimateSuccess(peerID string, networkInfo map[string]interface{}) int {
	// HTTP polling should work if we have HTTP access
	if peerURL, ok := networkInfo["peer_http_url"].(string); ok && peerURL != "" {
		return 90 // High success rate for HTTP
	}
	
	// Can work with IP if standard ports are available
	if peerIP, ok := networkInfo["peer_ip"].(string); ok && peerIP != "" {
		return 75
	}
	
	return 20
}

func (f *HTTPPollingFactory) Create(ctx context.Context, peerID string, config map[string]interface{}) (Connection, error) {
	networkInfo, _ := config["networkInfo"].(map[string]interface{})
	
	var baseURL string
	if url, ok := networkInfo["peer_http_url"].(string); ok && url != "" {
		baseURL = url
	} else if ip, ok := networkInfo["peer_ip"].(string); ok && ip != "" {
		baseURL = fmt.Sprintf("http://%s:8080", ip) // Default HTTP port
	} else {
		return nil, fmt.Errorf("peer HTTP URL or IP required for HTTP polling connection")
	}

	conn := &HTTPPollingConnection{
		peerID:      peerID,
		baseURL:     baseURL,
		factory:     f,
		status:      StatusConnecting,
		messages:    make(chan Message, 100),
		closeChan:   make(chan struct{}),
		lastMsgTime: 0,
	}

	// Start polling
	go conn.startPolling(ctx)

	return conn, nil
}

// HTTPPollingConnection implements Connection interface for HTTP polling
type HTTPPollingConnection struct {
	peerID      string
	baseURL     string
	factory     *HTTPPollingFactory
	status      ConnectionStatus
	messages    chan Message
	closeChan   chan struct{}
	mu          sync.RWMutex

	// Polling state
	lastMsgTime int64
	room        string

	// Quality metrics
	latency     time.Duration
	quality     int
	lastPing    time.Time
	lastPoll    time.Time
	pollErrors  int
}

func (c *HTTPPollingConnection) startPolling(ctx context.Context) {
	// First, test connection
	if err := c.testConnection(); err != nil {
		log.Printf("HTTP polling connection test failed for %s: %v", c.peerID, err)
		c.mu.Lock()
		c.status = StatusFailed
		c.mu.Unlock()
		return
	}

	c.mu.Lock()
	c.status = StatusConnected
	c.quality = 60 // Medium quality due to polling overhead
	c.room = fmt.Sprintf("peer-%s", c.peerID) // Default room
	c.mu.Unlock()

	log.Printf("HTTP polling connection to %s established", c.peerID)

	// Start polling loop
	ticker := time.NewTicker(c.factory.pollRate)
	defer ticker.Stop()

	// Start quality monitoring
	go c.startQualityMonitoring()

	for {
		select {
		case <-ticker.C:
			c.poll()
		case <-ctx.Done():
			return
		case <-c.closeChan:
			return
		}
	}
}

func (c *HTTPPollingConnection) testConnection() error {
	testURL := fmt.Sprintf("%s/history", c.baseURL)
	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return err
	}

	q := req.URL.Query()
	q.Add("room", "test")
	q.Add("since", "0")
	req.URL.RawQuery = q.Encode()

	resp, err := c.factory.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPPollingConnection) poll() {
	c.mu.Lock()
	room := c.room
	lastMsgTime := c.lastMsgTime
	c.mu.Unlock()

	pollURL := fmt.Sprintf("%s/history", c.baseURL)
	req, err := http.NewRequest("GET", pollURL, nil)
	if err != nil {
		c.recordPollError()
		return
	}

	q := req.URL.Query()
	q.Add("room", room)
	q.Add("since", strconv.FormatInt(lastMsgTime, 10))
	req.URL.RawQuery = q.Encode()

	startTime := time.Now()
	resp, err := c.factory.client.Do(req)
	if err != nil {
		c.recordPollError()
		return
	}
	defer resp.Body.Close()

	// Update latency
	c.mu.Lock()
	c.latency = time.Since(startTime)
	c.lastPoll = time.Now()
	c.pollErrors = 0 // Reset error count on success
	c.mu.Unlock()

	if resp.StatusCode != http.StatusOK {
		c.recordPollError()
		return
	}

	// Parse messages
	var polledMessages []struct {
		PeerID    string `json:"peer_id"`
		Text      string `json:"text"`
		Timestamp int64  `json:"timestamp"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&polledMessages); err != nil {
		c.recordPollError()
		return
	}

	// Process new messages
	for _, msg := range polledMessages {
		if msg.Timestamp > lastMsgTime {
			message := Message{
				From:      msg.PeerID,
				Type:      "chat",
				Data:      []byte(msg.Text),
				Timestamp: msg.Timestamp,
			}

			select {
			case c.messages <- message:
				c.mu.Lock()
				if msg.Timestamp > c.lastMsgTime {
					c.lastMsgTime = msg.Timestamp
				}
				c.mu.Unlock()
			default:
				log.Printf("Message buffer full for HTTP polling connection to %s", c.peerID)
			}
		}
	}

	// Update quality based on latency and poll rate
	c.updateQuality()
}

func (c *HTTPPollingConnection) recordPollError() {
	c.mu.Lock()
	c.pollErrors++
	if c.pollErrors > 5 {
		c.status = StatusFailed
	}
	c.mu.Unlock()
}

func (c *HTTPPollingConnection) updateQuality() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Base quality on latency and poll rate
	latencyMs := c.latency.Milliseconds()
	pollRateMs := c.factory.pollRate.Milliseconds()

	baseQuality := 60
	if latencyMs < 100 {
		baseQuality = 70
	} else if latencyMs < 500 {
		baseQuality = 60
	} else if latencyMs < 1000 {
		baseQuality = 50
	} else {
		baseQuality = 30
	}

	// Adjust for poll rate (faster polling = better responsiveness)
	if pollRateMs <= 1000 {
		baseQuality += 10
	} else if pollRateMs >= 5000 {
		baseQuality -= 10
	}

	// Adjust for errors
	if c.pollErrors > 0 {
		baseQuality -= c.pollErrors * 10
	}

	if baseQuality < 0 {
		baseQuality = 0
	} else if baseQuality > 100 {
		baseQuality = 100
	}

	c.quality = baseQuality
}

func (c *HTTPPollingConnection) startQualityMonitoring() {
	ticker := time.NewTicker(time.Second * 10)
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

func (c *HTTPPollingConnection) sendPing() {
	ping := Message{
		Type:      "ping",
		Timestamp: time.Now().UnixNano(),
	}
	c.Send(ping)
	c.mu.Lock()
	c.lastPing = time.Now()
	c.mu.Unlock()
}

func (c *HTTPPollingConnection) Send(msg Message) error {
	c.mu.RLock()
	status := c.status
	room := c.room
	c.mu.RUnlock()

	if status != StatusConnected {
		return fmt.Errorf("HTTP polling connection not established")
	}

	// Send message via HTTP POST
	sendURL := fmt.Sprintf("%s/send", c.baseURL)
	
	payload := map[string]interface{}{
		"room":    room,
		"peer_id": msg.From,
		"text":    string(msg.Data),
	}

	if msg.Type == "ping" || msg.Type == "pong" {
		// For ping/pong, encode as JSON in text field
		pingData := map[string]interface{}{
			"type":      msg.Type,
			"timestamp": msg.Timestamp,
		}
		jsonData, _ := json.Marshal(pingData)
		payload["text"] = string(jsonData)
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", sendURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.factory.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("send failed with status: %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPPollingConnection) Receive() <-chan Message {
	return c.messages
}

func (c *HTTPPollingConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.status = StatusDisconnected
	close(c.closeChan)
	close(c.messages)
	return nil
}

func (c *HTTPPollingConnection) Status() ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *HTTPPollingConnection) Type() ConnectionType {
	return HTTPPolling
}

func (c *HTTPPollingConnection) Latency() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latency
}

func (c *HTTPPollingConnection) Quality() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.quality
}
