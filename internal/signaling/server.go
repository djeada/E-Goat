package signaling

import (
    "log"
    "net/http"
    "sync"
    "time"

    "github.com/gorilla/websocket"
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
    hub  *Hub
    conn *websocket.Conn
    send chan []byte
    room string
}

// Hub maintains the set of active clients and broadcasts messages to rooms.
type Hub struct {
    // Registered clients.
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

// NewHub initializes a Hub.
func NewHub() *Hub {
    return &Hub{
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
                        log.Printf("send buffer full, dropping client in room %s", msg.Room)
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

// ServeHTTP handles WebSocket requests for signaling.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    room := r.URL.Query().Get("room")
    if room == "" {
        http.Error(w, "room parameter is required", http.StatusBadRequest)
        return
    }
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("WebSocket upgrade error: %v", err)
        return
    }
    client := &Client{hub: h, conn: conn, send: make(chan []byte, 256), room: room}
    client.hub.register <- client

    // Start pumps
    go client.writePump()
    client.readPump()
}

// readPump pumps messages from the WebSocket connection to the Hub.
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
                log.Printf("readPump error: %v", err)
            }
            break
        }
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
