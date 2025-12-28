// internal/signaling/server.go
package signaling

import (
    "database/sql"
    "encoding/json"
    "log"
    "net/http"
    "sync"
    "time"

    "github.com/gorilla/websocket"
    "github.com/djeada/E-Goat/internal/storage"
)

const (
    writeWait      = 10 * time.Second
    pongWait       = 60 * time.Second
    pingPeriod     = (pongWait * 9) / 10
    maxMessageSize = 4096 // Increased from 512 for larger messages
)

// ClientState tracks client connection state
type ClientState string

const (
    StateConnecting   ClientState = "connecting"
    StateConnected    ClientState = "connected"
    StateDisconnecting ClientState = "disconnecting"
)

// Client is a middleman between the websocket connection and the Hub for signaling relay.
type Client struct {
    hub       *Hub
    conn      *websocket.Conn
    send      chan []byte     // outbound WS messages (signaling)
    room      string
    peerID    string
    state     ClientState
    connectedAt time.Time
    lastActivity time.Time
    messagesSent int64
    messagesRecv int64
    mu          sync.RWMutex
}

// ClientInfo provides read-only client information
type ClientInfo struct {
    PeerID      string `json:"peer_id"`
    Room        string `json:"room"`
    State       string `json:"state"`
    ConnectedAt int64  `json:"connected_at"`
    LastActivity int64 `json:"last_activity"`
    MessagesSent int64 `json:"messages_sent"`
    MessagesRecv int64 `json:"messages_received"`
}

// GetInfo returns read-only client information
func (c *Client) GetInfo() ClientInfo {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return ClientInfo{
        PeerID:       c.peerID,
        Room:         c.room,
        State:        string(c.state),
        ConnectedAt:  c.connectedAt.Unix(),
        LastActivity: c.lastActivity.Unix(),
        MessagesSent: c.messagesSent,
        MessagesRecv: c.messagesRecv,
    }
}

// RoomStats provides room statistics
type RoomStats struct {
    Name        string       `json:"name"`
    ClientCount int          `json:"client_count"`
    Clients     []ClientInfo `json:"clients,omitempty"`
    CreatedAt   int64        `json:"created_at,omitempty"`
}

// Hub manages active clients and message routing.
type Hub struct {
    db         *sql.DB
    rooms      map[string]map[*Client]bool
    roomMeta   map[string]*roomMetadata
    register   chan *Client
    unregister chan *Client
    broadcast  chan *Message
    mu         sync.RWMutex
    
    // Metrics
    totalConnections   int64
    totalMessages      int64
    activeConnections  int64
}

type roomMetadata struct {
    createdAt   time.Time
    lastActive  time.Time
    messageCount int64
}

// Message is a raw blob to broadcast to a room.
type Message struct {
    Room    string
    Payload []byte
    Sender  *Client
}

// NewHub creates a Hub tied to a SQLite DB.
func NewHub(db *sql.DB) *Hub {
    return &Hub{
        db:         db,
        rooms:      make(map[string]map[*Client]bool),
        roomMeta:   make(map[string]*roomMetadata),
        register:   make(chan *Client),
        unregister: make(chan *Client),
        broadcast:  make(chan *Message),
    }
}

// GetStats returns hub statistics
func (h *Hub) GetStats() map[string]interface{} {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    roomStats := make([]RoomStats, 0, len(h.rooms))
    for roomName, clients := range h.rooms {
        clientInfos := make([]ClientInfo, 0, len(clients))
        for client := range clients {
            clientInfos = append(clientInfos, client.GetInfo())
        }
        
        rs := RoomStats{
            Name:        roomName,
            ClientCount: len(clients),
            Clients:     clientInfos,
        }
        if meta, ok := h.roomMeta[roomName]; ok {
            rs.CreatedAt = meta.createdAt.Unix()
        }
        roomStats = append(roomStats, rs)
    }
    
    return map[string]interface{}{
        "total_connections":  h.totalConnections,
        "active_connections": h.activeConnections,
        "total_messages":     h.totalMessages,
        "room_count":         len(h.rooms),
        "rooms":              roomStats,
    }
}

// GetRoomInfo returns information about a specific room
func (h *Hub) GetRoomInfo(roomName string) *RoomStats {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    clients, ok := h.rooms[roomName]
    if !ok {
        return nil
    }
    
    clientInfos := make([]ClientInfo, 0, len(clients))
    for client := range clients {
        clientInfos = append(clientInfos, client.GetInfo())
    }
    
    rs := &RoomStats{
        Name:        roomName,
        ClientCount: len(clients),
        Clients:     clientInfos,
    }
    if meta, ok := h.roomMeta[roomName]; ok {
        rs.CreatedAt = meta.createdAt.Unix()
    }
    
    return rs
}

// Run drives register/unregister/broadcast loops.
func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.mu.Lock()
            h.totalConnections++
            h.activeConnections++
            
            conns := h.rooms[client.room]
            if conns == nil {
                conns = make(map[*Client]bool)
                h.rooms[client.room] = conns
                h.roomMeta[client.room] = &roomMetadata{
                    createdAt:  time.Now(),
                    lastActive: time.Now(),
                }
                log.Printf("🏠 Room '%s' created", client.room)
            }
            
            // Update room metadata
            if meta, ok := h.roomMeta[client.room]; ok {
                meta.lastActive = time.Now()
            }
            
            // Update client state
            client.mu.Lock()
            client.state = StateConnected
            client.connectedAt = time.Now()
            client.lastActivity = time.Now()
            client.mu.Unlock()
            
            // Notify the new peer about all existing peers in the room
            for existingClient := range conns {
                existingPeerNotification := map[string]interface{}{
                    "type":    "peer_joined",
                    "peer_id": existingClient.peerID,
                    "room":    client.room,
                }
                existingPeerData, err := json.Marshal(existingPeerNotification)
                if err != nil {
                    log.Printf("❌ Failed to marshal notification for new peer %s about existing peer %s: %v", client.peerID, existingClient.peerID, err)
                    continue
                }
                select {
                case client.send <- existingPeerData:
                    log.Printf("✅ Notified new peer %s about existing peer %s", client.peerID, existingClient.peerID)
                default:
                    log.Printf("❌ Failed to notify new peer %s about existing peer %s", client.peerID, existingClient.peerID)
                }
            }
            
            // Add the new client to the room
            h.rooms[client.room][client] = true
            log.Printf("👋 Peer '%s' joined room '%s' (%d clients)", client.peerID[:min(8, len(client.peerID))], client.room, len(h.rooms[client.room]))
            
            // Notify other clients in the room about new peer
            joinNotification := map[string]interface{}{
                "type":    "peer_joined",
                "peer_id": client.peerID,
                "room":    client.room,
            }
            joinData, err := json.Marshal(joinNotification)
            if err != nil {
                log.Printf("❌ Failed to marshal join notification for peer %s: %v", client.peerID, err)
                h.mu.Unlock()
                return
            }
            
            for existingClient := range conns {
                if existingClient.peerID != client.peerID {
                    select {
                    case existingClient.send <- joinData:
                        log.Printf("✅ Notified existing peer %s about new peer %s", existingClient.peerID[:min(8, len(existingClient.peerID))], client.peerID[:min(8, len(client.peerID))])
                    default:
                        log.Printf("❌ Failed to notify %s about new peer %s", existingClient.peerID, client.peerID)
                    }
                }
            }
            h.mu.Unlock()

        case client := <-h.unregister:
            h.mu.Lock()
            h.activeConnections--
            
            // Update client state
            client.mu.Lock()
            client.state = StateDisconnecting
            client.mu.Unlock()
            
            if conns, ok := h.rooms[client.room]; ok {
                if _, exists := conns[client]; exists {
                    delete(conns, client)
                    close(client.send)
                    
                    log.Printf("👋 Peer '%s' left room '%s' (%d clients remaining)", 
                        client.peerID[:min(8, len(client.peerID))], client.room, len(conns))
                    
                    // Notify other clients about peer leaving
                    leaveNotification := map[string]interface{}{
                        "type":    "peer_left",
                        "peer_id": client.peerID,
                        "room":    client.room,
                    }
                    leaveData, err := json.Marshal(leaveNotification)
                    if err == nil {
                        for existingClient := range conns {
                            select {
                            case existingClient.send <- leaveData:
                            default:
                            }
                        }
                    }
                    
                    // Clean up empty rooms
                    if len(conns) == 0 {
                        delete(h.rooms, client.room)
                        delete(h.roomMeta, client.room)
                        log.Printf("🏠 Room '%s' closed (no clients)", client.room)
                    }
                }
            }
            h.mu.Unlock()

        case msg := <-h.broadcast:
            h.mu.Lock()
            h.totalMessages++
            
            // Update room metadata
            if meta, ok := h.roomMeta[msg.Room]; ok {
                meta.lastActive = time.Now()
                meta.messageCount++
            }
            
            // Update sender stats
            if msg.Sender != nil {
                msg.Sender.mu.Lock()
                msg.Sender.messagesSent++
                msg.Sender.lastActivity = time.Now()
                msg.Sender.mu.Unlock()
            }
            h.mu.Unlock()
            
            // persist signaling to DB
            if err := storage.SaveMessage(
                h.db, msg.Room, msg.Sender.peerID,
                "signal", msg.Payload, nil,
            ); err != nil {
                log.Printf("failed to save signaling message: %v", err)
            }

            h.mu.RLock()
            conns := h.rooms[msg.Room]
            h.mu.RUnlock()
            for c := range conns {
                if c != msg.Sender {
                    select {
                    case c.send <- msg.Payload:
                        c.mu.Lock()
                        c.messagesRecv++
                        c.lastActivity = time.Now()
                        c.mu.Unlock()
                    default:
                        log.Printf("send buffer full, dropping client %s in room %s", c.peerID[:min(8, len(c.peerID))], msg.Room)
                        c.conn.Close()
                    }
                }
            }
        }
    }
}

var upgrader = websocket.Upgrader{
    ReadBufferSize:  4096,
    WriteBufferSize: 4096,
    CheckOrigin:     func(r *http.Request) bool { return true },
}

// ServeHTTP upgrades to WebSocket, registers the client for signaling relay.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    room := r.URL.Query().Get("room")
    peerID := r.URL.Query().Get("peer_id")
    if room == "" || peerID == "" {
        http.Error(w, "room and peer_id parameters are required", http.StatusBadRequest)
        return
    }

    // Validate room and peer_id length
    if len(room) > 100 || len(peerID) > 100 {
        http.Error(w, "room and peer_id must be less than 100 characters", http.StatusBadRequest)
        return
    }

    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("WebSocket upgrade error: %v", err)
        return
    }

    client := &Client{
        hub:    h,
        conn:   conn,
        send:   make(chan []byte, 256),
        room:   room,
        peerID: peerID,
        state:  StateConnecting,
    }

    log.Printf("🔌 New WebSocket connection from peer '%s' for room '%s'", peerID[:min(8, len(peerID))], room)

    // Register and start pumps for signaling relay
    client.hub.register <- client
    go client.writePump()
    client.readPump()
}

// readPump reads incoming WS messages, routes them into signaling relay.
func (c *Client) readPump() {
    defer func() {
        c.hub.unregister <- c
        c.conn.Close()
    }()

    c.conn.SetReadLimit(maxMessageSize)
    c.conn.SetReadDeadline(time.Now().Add(pongWait))
    c.conn.SetPongHandler(func(string) error {
        c.conn.SetReadDeadline(time.Now().Add(pongWait))
        return nil
    })

    for {
        _, raw, err := c.conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("readPump error for peer %s: %v", c.peerID, err)
            }
            break
        }

        // Relay signaling messages to other peers in the room
        // Parse the message to see if it has a target peer
        var sigMsg map[string]interface{}
        if err := json.Unmarshal(raw, &sigMsg); err != nil {
            log.Printf("❌ Failed to parse signaling message from %s: %v", c.peerID, err)
            continue
        }
        
        // Check if this is a targeted message
        if targetPeerID, ok := sigMsg["target_peer_id"].(string); ok && targetPeerID != "" {
            // Send to specific peer
            log.Printf("📡 Relaying targeted signaling message from %s to %s: %s", c.peerID, targetPeerID, sigMsg["type"])
            
            c.hub.mu.RLock()
            conns := c.hub.rooms[c.room]
            c.hub.mu.RUnlock()
            
            for client := range conns {
                if client.peerID == targetPeerID {
                    select {
                    case client.send <- raw:
                        log.Printf("✅ Signaling message delivered to %s", targetPeerID)
                    default:
                        log.Printf("❌ Failed to deliver signaling to %s (buffer full)", targetPeerID)
                    }
                    break
                }
            }
        } else {
            // Broadcast to all peers in room (for announcements, etc.)
            log.Printf("📡 Broadcasting signaling message from %s: %s", c.peerID, sigMsg["type"])
            c.hub.broadcast <- &Message{Room: c.room, Payload: raw, Sender: c}
        }
    }
}

// writePump writes outbound signaling messages back over the WS.
func (c *Client) writePump() {
    ticker := time.NewTicker(pingPeriod)
    defer func() {
        ticker.Stop()
        c.conn.Close()
    }()

    for {
        select {
        case message, ok := <-c.send:
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if !ok {
                c.conn.WriteMessage(websocket.CloseMessage, nil)
                return
            }
            if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
                return
            }

        case <-ticker.C:
            c.conn.SetWriteDeadline(time.Now().Add(writeWait))
            if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        }
    }
}

// ServeChat is deprecated - chat now happens via WebRTC data channels P2P.
// This signaling server only handles WebRTC signaling messages.
func (h *Hub) ServeChat(w http.ResponseWriter, r *http.Request) {
    http.Error(w, "Direct chat via server is deprecated. Use WebRTC P2P connections.", http.StatusNotImplemented)
}
