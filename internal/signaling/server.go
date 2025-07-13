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
    maxMessageSize = 512
)

// Client is a middleman between the websocket connection and the Hub for signaling relay.
type Client struct {
    hub    *Hub
    conn   *websocket.Conn
    send   chan []byte     // outbound WS messages (signaling)
    room   string
    peerID string
    // Note: No WebRTC peer here - this is a pure signaling relay server
}

// Hub manages active clients and message routing.
type Hub struct {
    db         *sql.DB
    rooms      map[string]map[*Client]bool
    register   chan *Client
    unregister chan *Client
    broadcast  chan *Message
    mu         sync.RWMutex
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
        register:   make(chan *Client),
        unregister: make(chan *Client),
        broadcast:  make(chan *Message),
    }
}

// Run drives register/unregister/broadcast loops.
func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.mu.Lock()
            conns := h.rooms[client.room]
            if conns == nil {
                conns = make(map[*Client]bool)
                h.rooms[client.room] = conns
            }
            h.rooms[client.room][client] = true
            
            // Notify other clients in the room about new peer
            if len(conns) > 0 {
                joinNotification := map[string]interface{}{
                    "type":    "peer_joined",
                    "peer_id": client.peerID,
                    "room":    client.room,
                }
                joinData, _ := json.Marshal(joinNotification)
                
                for existingClient := range conns {
                    if existingClient.peerID != client.peerID {
                        select {
                        case existingClient.send <- joinData:
                        default:
                            log.Printf("Failed to notify %s about new peer %s", existingClient.peerID, client.peerID)
                        }
                    }
                }
            }
            h.mu.Unlock()

        case client := <-h.unregister:
            h.mu.Lock()
            if conns, ok := h.rooms[client.room]; ok {
                if _, exists := conns[client]; exists {
                    delete(conns, client)
                    close(client.send)
                    // Note: No WebRTC peer to close - this is pure signaling relay
                    if len(conns) == 0 {
                        delete(h.rooms, client.room)
                    }
                }
            }
            h.mu.Unlock()

        case msg := <-h.broadcast:
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
                    default:
                        log.Printf("send buffer full, dropping client %s in room %s", c.peerID, msg.Room)
                        c.conn.Close()
                    }
                }
            }
        }
    }
}

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
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
    }

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
