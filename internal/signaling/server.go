// internal/signaling/server.go
package signaling

import (
    "database/sql"
    "log"
    "net/http"
    "sync"
    "time"

    "github.com/gorilla/websocket"
    "github.com/djeada/E-Goat/internal/storage"
)

const (
    // Time allowed to write a message to the peer.
    writeWait = 10 * time.Second
    // Time allowed to read the next pong message from the peer.
    pongWait = 60 * time.Second
    // Send pings to peer with this period. Must be less than pongWait.
    pingPeriod = (pongWait * 9) / 10
    // Maximum message size allowed from peer.
    maxMessageSize = 512
)

// Client is a middleman between the websocket connection and the Hub.
type Client struct {
    hub    *Hub
    conn   *websocket.Conn
    send   chan []byte
    room   string
    peerID string
}

// Hub maintains the set of active clients and broadcasts messages to rooms.
type Hub struct {
    db         *sql.DB
    rooms      map[string]map[*Client]bool
    register   chan *Client
    unregister chan *Client
    broadcast  chan *Message
    mu         sync.RWMutex
}

// Message holds the room and payload to broadcast.
type Message struct {
    Room    string
    Payload []byte
    Sender  *Client
}

// NewHub initializes a Hub with a database handle.
func NewHub(db *sql.DB) *Hub {
    return &Hub{
        db:         db,
        rooms:      make(map[string]map[*Client]bool),
        register:   make(chan *Client),
        unregister: make(chan *Client),
        broadcast:  make(chan *Message),
    }
}

// Run processes register/unregister and broadcast requests.
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
            h.mu.Unlock()

        case client := <-h.unregister:
            h.mu.Lock()
            if conns, ok := h.rooms[client.room]; ok {
                if _, exists := conns[client]; exists {
                    delete(conns, client)
                    close(client.send)
                    if len(conns) == 0 {
                        delete(h.rooms, client.room)
                    }
                }
            }
            h.mu.Unlock()

        case msg := <-h.broadcast:
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

// ServeHTTP handles WebSocket requests for signaling. URL query must provide:
//   ?room=<roomID>&peer_id=<yourPeerID>
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
    client.hub.register <- client

    // start pumps
    go client.writePump()
    client.readPump()
}

// readPump pumps messages from the WebSocket connection to the Hub,
// and persists them via storage.SaveMessage.
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
        _, message, err := c.conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("readPump error for peer %s: %v", c.peerID, err)
            }
            break
        }

        // Persist the raw signaling message
        if err := storage.SaveMessage(
            c.hub.db,
            c.room,
            c.peerID,
            "signal",
            message,
            nil,
        ); err != nil {
            log.Printf("failed to save signaling message: %v", err)
        }

        // Broadcast to other clients in the room
        c.hub.broadcast <- &Message{Room: c.room, Payload: message, Sender: c}
    }
}

// writePump pumps messages from the Hub to the WebSocket connection.
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
                // Hub closed the channel.
                c.conn.WriteMessage(websocket.CloseMessage, []byte{})
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
