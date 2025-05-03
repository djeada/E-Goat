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
    "github.com/djeada/E-Goat/internal/webrtc"
)

const (
    writeWait      = 10 * time.Second
    pongWait       = 60 * time.Second
    pingPeriod     = (pongWait * 9) / 10
    maxMessageSize = 512
)

// Client is a middleman between the websocket connection, the Hub, and a WebRTC Peer.
type Client struct {
    hub    *Hub
    conn   *websocket.Conn
    send   chan []byte     // outbound WS messages (signaling)
    room   string
    peerID string
    peer   *webrtc.Peer    // Pion PeerConnection wrapper
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
            h.mu.Unlock()

        case client := <-h.unregister:
            h.mu.Lock()
            if conns, ok := h.rooms[client.room]; ok {
                if _, exists := conns[client]; exists {
                    delete(conns, client)
                    close(client.send)
                    if client.peer != nil {
                        client.peer.Close()
                    }
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

// ServeHTTP upgrades to WebSocket, registers the client, and sets up its Peer.
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

    // Define how this Peer sends signaling back into the Hub:
    sendSignal := func(msg webrtc.Message) error {
        raw, err := json.Marshal(msg)
        if err != nil {
            return err
        }
        h.broadcast <- &Message{Room: room, Payload: raw, Sender: client}
        return nil
    }
    // No-op onMessage: actual chat data is P2P via DataChannel.
    onMessage := func(from string, data []byte) {}
    // On WebRTC failure/close, unregister the client.
    onClose := func() {
        h.unregister <- client
        client.conn.Close()
    }

    // Create the WebRTC Peer
    p, err := webrtc.NewPeer(peerID, onMessage, sendSignal, onClose)
    if err != nil {
        log.Printf("failed to create WebRTC Peer for %s: %v", peerID, err)
        conn.Close()
        return
    }
    client.peer = p

    // Register and start pumps
    client.hub.register <- client
    go client.writePump()
    client.readPump()
}

// readPump reads incoming WS messages, routes them into Peer.HandleSignal.
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

        var sig webrtc.Message
        if err := json.Unmarshal(raw, &sig); err != nil {
            log.Printf("invalid signal json from %s: %v", c.peerID, err)
            continue
        }

        if err := c.peer.HandleSignal(sig); err != nil {
            log.Printf("peer.HandleSignal error for %s: %v", c.peerID, err)
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

// ServeChat handles plain‐text chat over WebSocket, relays into WebRTC DataChannels,
// and echoes back to the browser so the UI can render it.
// URL query must provide: ?room=<roomID>&peer_id=<yourPeerID>
func (h *Hub) ServeChat(w http.ResponseWriter, r *http.Request) {
    room := r.URL.Query().Get("room")
    peerID := r.URL.Query().Get("peer_id")
    if room == "" || peerID == "" {
        http.Error(w, "room and peer_id parameters are required", http.StatusBadRequest)
        return
    }

    // Upgrade connection
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("Chat WS upgrade error: %v", err)
        return
    }
    defer conn.Close()

    // Simple loop: read JSON {peer_id,text}
    for {
        var msg struct {
            PeerID string `json:"peer_id"`
            Text   string `json:"text"`
        }
        if err := conn.ReadJSON(&msg); err != nil {
            log.Printf("Chat read error: %v", err)
            return
        }

        // Persist chat text
        if err := storage.SaveMessage(
            h.db, room, msg.PeerID,
            "text", []byte(msg.Text), nil,
        ); err != nil {
            log.Printf("failed to save chat message: %v", err)
        }

        // Fan-out into all Pion DataChannels in the room
        h.mu.RLock()
        for c := range h.rooms[room] {
            // do not send back to the same peer’s WS here,
            // but DataChannel is truly P2P
            if c.peer != nil {
                if err := c.peer.SendMessage([]byte(msg.Text)); err != nil {
                    log.Printf("failed to send DataChannel msg to %s: %v", c.peerID, err)
                }
            }
        }
        h.mu.RUnlock()

        // Echo back to the browser WS so the UI can append it
        if err := conn.WriteJSON(msg); err != nil {
            log.Printf("Chat write error: %v", err)
            return
        }
    }
}

