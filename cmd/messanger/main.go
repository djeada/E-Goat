package main

import (
    "context"
    "database/sql"
    "embed"
    "encoding/json"
    "flag"
    "fmt"
    "io/fs"
    "log"
    "net/http"
    "os"
    "os/signal"
    "path/filepath"
    "strconv"
    "syscall"
    "time"

    _ "github.com/mattn/go-sqlite3"

    "github.com/djeada/E-Goat/internal/storage"
    "github.com/djeada/E-Goat/internal/signaling"
    "github.com/djeada/E-Goat/internal/transport"
    "github.com/djeada/E-Goat/internal/middleware"
    "github.com/djeada/E-Goat/internal/metrics"
)

// Version information
const Version = "2.0.0"

var (
    //go:embed web/*
    embeddedFS embed.FS

    httpPort int
    wsPort   int
    dbPath   string
    publicBase string
    rateLimit  int
    
    // Global transport manager for the instance
    globalTransport *transport.TransportManager
    globalInstanceID string
    
    // Global metrics collector
    globalMetrics *metrics.Collector
    
    // Application start time
    startTime time.Time
)

func init() {
    flag.IntVar(&httpPort, "http-port", 8080, "Port for HTTP server (UI + polling endpoints)")
    flag.IntVar(&wsPort, "ws-port", 9000, "Port for signaling WebSocket server")
    flag.StringVar(&dbPath, "db", defaultDBPath(), "Path to SQLite database file")
    flag.StringVar(&publicBase, "public-base", os.Getenv("EGOAT_PUBLIC_BASE"), "Public base URL for invite links (optional)")
    flag.IntVar(&rateLimit, "rate-limit", 100, "Maximum requests per minute per IP (0 to disable)")
}

func defaultDBPath() string {
    home, err := os.UserHomeDir()
    if err != nil || home == "" {
        return "chat.db"
    }
    return filepath.Join(home, "tmp", "e-goat", "chat.db")
}

func main() {
    flag.Parse()
    startTime = time.Now()

    log.Printf("🐐 E-Goat v%s starting...", Version)

    // 1. Initialize SQLite database
    db, err := storage.InitDB(dbPath)
    if err != nil {
        log.Fatalf("Database initialization failed: %v", err)
    }
    defer db.Close()
    log.Printf("💾 Database initialized at %s", dbPath)

    // 1.5. Initialize global metrics collector
    globalMetrics = metrics.NewCollector()
    log.Printf("📊 Metrics collector initialized")

    // 2. Create & run the signaling Hub
    hub := signaling.NewHub(db)
    go hub.Run()
    log.Printf("📡 Signaling hub started")

    // 2.5. Initialize global transport manager
    globalInstanceID = fmt.Sprintf("instance-%d", httpPort)
    globalTransport = transport.NewTransportManager(globalInstanceID)
    
    // Set up transport message handlers
    globalTransport.SetMessageHandler(func(msg transport.Message) {
        log.Printf("🔄 Transport message from %s: %s", msg.From, string(msg.Data))
        // Store transport messages in database
        if err := storage.SaveMessage(db, "transport", msg.From, "transport", msg.Data, nil); err != nil {
            log.Printf("Failed to save transport message: %v", err)
        }
        // Record metrics
        globalMetrics.RecordMessage(false, int64(len(msg.Data)))
    })
    
    globalTransport.SetConnectionHandler(func(peerID string, conn transport.Connection) {
        log.Printf("🔗 Transport connected to %s via %s (quality: %d%%)", 
            peerID, conn.Type(), conn.Quality())
        globalMetrics.RecordConnection(string(conn.Type()))
        globalMetrics.RegisterPeer(peerID, string(conn.Type()))
    })
    
    globalTransport.SetDisconnectHandler(func(peerID string, connType transport.ConnectionType) {
        log.Printf("🔌 Transport disconnected from %s (was using %s)", peerID, connType)
        globalMetrics.RecordDisconnection(string(connType))
        globalMetrics.UnregisterPeer(peerID)
    })

    // Note: Transport manager doesn't need explicit Start() - it's ready after creation
    log.Printf("🚀 Transport manager initialized for peer: %s", globalInstanceID)

    // 3. Prepare embedded web assets
    contentFS, err := fs.Sub(embeddedFS, "web")
    if err != nil {
        log.Fatalf("Failed to locate embedded web assets: %v", err)
    }

    // 4. HTTP mux for UI + polling/send endpoints
    httpMux := http.NewServeMux()
    
    // Static and main routes
    httpMux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(contentFS))))
    httpMux.HandleFunc("/", indexHandler)
    
    // Core chat endpoints
    httpMux.HandleFunc("/history", historyHandler(db))
    httpMux.HandleFunc("/send", sendHandler(db))
    
    // Transport layer endpoints
    httpMux.HandleFunc("/transport/connect", transportConnectHandler)
    httpMux.HandleFunc("/transport/send", transportSendHandler)
    httpMux.HandleFunc("/transport/status", transportStatusHandler)
    httpMux.HandleFunc("/transport/options", transportOptionsHandler)
    httpMux.HandleFunc("/transport/strategy", transportStrategyHandler)
    httpMux.HandleFunc("/transport/stats", transportStatsHandler)
    
    // Enhanced API endpoints
    httpMux.HandleFunc("/api/v2/messages", messagesV2Handler(db))
    httpMux.HandleFunc("/api/v2/rooms", roomsHandler(db))
    httpMux.HandleFunc("/api/v2/rooms/info", roomInfoHandler(db))
    httpMux.HandleFunc("/api/v2/peers", peersHandler(db))
    httpMux.HandleFunc("/api/v2/search", searchHandler(db))
    httpMux.HandleFunc("/api/v2/stats", dbStatsHandler(db))
    
    // Health and metrics endpoints
    httpMux.HandleFunc("/health", middleware.HealthCheck(db, Version, startTime))
    httpMux.HandleFunc("/metrics", globalMetrics.MetricsHandler())
    httpMux.HandleFunc("/ready", readinessHandler(db))

    // Apply middleware chain
    var handler http.Handler = httpMux
    
    // Apply rate limiting if enabled
    if rateLimit > 0 {
        rateLimiter := middleware.NewRateLimiter(rateLimit, time.Minute)
        handler = middleware.RateLimitMiddleware(rateLimiter)(handler)
    }
    
    // Apply middleware chain
    handler = middleware.Chain(
        middleware.RecoveryMiddleware,
        middleware.RequestIDMiddleware,
        middleware.LoggingMiddleware,
        middleware.SecurityHeadersMiddleware,
        middleware.CORSMiddleware([]string{"*"}),
        globalMetrics.MetricsMiddleware,
    )(handler)

    httpSrv := &http.Server{
        Addr:         fmt.Sprintf(":%d", httpPort),
        Handler:      handler,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    go func() {
        log.Printf("🌐 HTTP server listening on http://localhost:%d", httpPort)
        log.Printf("📊 Metrics available at http://localhost:%d/metrics", httpPort)
        log.Printf("💚 Health check at http://localhost:%d/health", httpPort)
        if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("HTTP server error: %v", err)
        }
    }()

    // 5. Separate server for signaling WS (over WS port)
    wsMux := http.NewServeMux()
    wsMux.Handle("/signal", hub) // Hub implements ServeHTTP for signaling
    wsSrv := &http.Server{
        Addr:         fmt.Sprintf(":%d", wsPort),
        Handler:      wsMux,
        ReadTimeout:  0, // No timeout for WebSocket
        WriteTimeout: 0,
    }

    go func() {
        log.Printf("📡 Signaling WebSocket server listening on ws://localhost:%d/signal", wsPort)
        if err := wsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("WebSocket server error: %v", err)
        }
    }()

    log.Printf("✅ E-Goat v%s fully operational!", Version)

    // 6. Graceful shutdown on SIGINT/SIGTERM
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
    <-quit

    log.Println("Shutting down servers...")
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := httpSrv.Shutdown(ctx); err != nil {
        log.Printf("HTTP server shutdown error: %v", err)
    }
    if err := wsSrv.Shutdown(ctx); err != nil {
        log.Printf("WebSocket server shutdown error: %v", err)
    }
}

// indexHandler serves `index.html`, injecting wsPort and httpPort for the client
func indexHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    config := struct {
        WsPort     int    `json:"wsPort"`
        HttpPort   int    `json:"httpPort"`
        PublicBase string `json:"publicBase"`
    }{
        WsPort:     wsPort,
        HttpPort:   httpPort,
        PublicBase: publicBase,
    }
    configJSON, err := json.Marshal(config)
    if err != nil {
        log.Printf("Error marshaling config: %v", err)
        configJSON = []byte(`{}`)
    }
    configScript := fmt.Sprintf(`<script>window.config = %s;</script>`, configJSON)
    if _, err := w.Write([]byte(configScript)); err != nil {
        log.Printf("Error writing port script: %v", err)
    }
    data, err := embeddedFS.ReadFile("web/index.html")
    if err != nil {
        http.Error(w, "Index page not found", http.StatusInternalServerError)
        return
    }
    if _, err := w.Write(data); err != nil {
        log.Printf("Error writing index HTML: %v", err)
    }
}

// historyHandler returns JSON array of new chat messages since the given timestamp.
func historyHandler(db *sql.DB) http.HandlerFunc {
    type outMsg struct {
        PeerID    string `json:"peer_id"`
        Text      string `json:"text"`
        Timestamp int64  `json:"timestamp"`
    }
    return func(w http.ResponseWriter, r *http.Request) {
        room := r.URL.Query().Get("room")
        sinceStr := r.URL.Query().Get("since")
        since, _ := strconv.ParseInt(sinceStr, 10, 64)

        rows, err := db.Query(`
            SELECT p.peer_id, m.content, m.timestamp
              FROM messages m
              JOIN chats c ON c.id = m.chat_id
              JOIN peers p ON p.id = m.peer_id
             WHERE c.name = ?
               AND m.msg_type = 'text'
               AND m.timestamp > ?
             ORDER BY m.timestamp ASC
        `, room, since)
        if err != nil {
            http.Error(w, "query error", http.StatusInternalServerError)
            return
        }
        defer rows.Close()

        var out []outMsg
        for rows.Next() {
            var peerID string
            var content []byte
            var ts int64
            if err := rows.Scan(&peerID, &content, &ts); err != nil {
                continue
            }
            out = append(out, outMsg{peerID, string(content), ts})
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(out)
    }
}

// sendHandler accepts a new chat message and saves it, returning its timestamp.
func sendHandler(db *sql.DB) http.HandlerFunc {
    type inMsg struct {
        Room   string `json:"room"`
        PeerID string `json:"peer_id"`
        Text   string `json:"text"`
    }
    type out struct {
        Timestamp int64 `json:"timestamp"`
    }
    return func(w http.ResponseWriter, r *http.Request) {
        var m inMsg
        if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
            http.Error(w, "bad JSON", http.StatusBadRequest)
            return
        }

        now := time.Now().Unix()
        if err := storage.SaveMessage(db, m.Room, m.PeerID, "text", []byte(m.Text), nil); err != nil {
            http.Error(w, "save error", http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(out{Timestamp: now})
    }
}

// transportConnectHandler initiates a transport connection to a peer
func transportConnectHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    
    type connectReq struct {
        PeerID string `json:"peer_id"`
        Room   string `json:"room"`
    }
    
    var req connectReq
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        log.Printf("Transport connect JSON decode error: %v", err)
        http.Error(w, fmt.Sprintf("bad JSON: %v", err), http.StatusBadRequest)
        return
    }
    
    if globalTransport == nil {
        http.Error(w, "transport not initialized", http.StatusInternalServerError)
        return
    }
    
    // Attempt to connect using the layered transport system
    go func() {
        networkInfo := globalTransport.CreateNetworkInfo("", "", "")
        if err := globalTransport.ConnectToPeer(req.PeerID, networkInfo); err != nil {
            log.Printf("Failed to connect to peer %s: %v", req.PeerID, err)
        }
    }()
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "connecting"})
}

// transportSendHandler sends a message via the transport layer
func transportSendHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    
    type sendReq struct {
        PeerID string `json:"peer_id"`
        Text   string `json:"text"`
        Room   string `json:"room"`
    }
    
    var req sendReq
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        log.Printf("Transport send JSON decode error: %v", err)
        http.Error(w, fmt.Sprintf("bad JSON: %v", err), http.StatusBadRequest)
        return
    }
    
    if globalTransport == nil {
        http.Error(w, "transport not initialized", http.StatusInternalServerError)
        return
    }
    
    // Send via transport layer
    if err := globalTransport.SendMessage(req.PeerID, "chat", []byte(req.Text)); err != nil {
        http.Error(w, fmt.Sprintf("transport send error: %v", err), http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

// transportStatusHandler returns the current transport status
func transportStatusHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    
    if globalTransport == nil {
        http.Error(w, "transport not initialized", http.StatusInternalServerError)
        return
    }
    
    strategyName := ""
    if strategy, ok := globalTransport.GetStrategy(); ok {
        strategyName = strategy.Name()
    }
    status := map[string]interface{}{
        "peer_id": globalInstanceID,
        "connections": globalTransport.GetAllConnectionsInfo(),
        "available_transports": []string{"WebRTC_STUN", "WebRTC_TURN", "WebSocket", "HTTP_Polling", "LAN_Broadcast"},
        "strategy": strategyName,
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(status)
}

// transportOptionsHandler returns available strategies and transport estimates.
func transportOptionsHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    if globalTransport == nil {
        http.Error(w, "transport not initialized", http.StatusInternalServerError)
        return
    }

    networkInfo := globalTransport.CreateNetworkInfo("", "", "")
    current := ""
    if strategy, ok := globalTransport.GetStrategy(); ok {
        current = strategy.Name()
    }

    payload := map[string]interface{}{
        "current_strategy": current,
        "strategies":        globalTransport.ListStrategyInfos(),
        "transports":        globalTransport.ListTransportOptions(networkInfo),
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(payload)
}

// transportStrategyHandler updates the active transport strategy.
func transportStrategyHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    if globalTransport == nil {
        http.Error(w, "transport not initialized", http.StatusInternalServerError)
        return
    }

    type reqBody struct {
        Name string `json:"name"`
    }
    var req reqBody
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "bad JSON", http.StatusBadRequest)
        return
    }
    if req.Name == "" {
        http.Error(w, "strategy name required", http.StatusBadRequest)
        return
    }

    if err := globalTransport.SetStrategy(req.Name); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ====== NEW ENHANCED API HANDLERS ======

// transportStatsHandler returns detailed transport statistics
func transportStatsHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    stats := map[string]interface{}{
        "peer_id":   globalInstanceID,
        "uptime":    time.Since(startTime).String(),
        "version":   Version,
    }

    if globalTransport != nil {
        stats["connections"] = globalTransport.GetAllConnectionsInfo()
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(stats)
}

// messagesV2Handler returns paginated messages with enhanced features
func messagesV2Handler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        room := r.URL.Query().Get("room")
        if room == "" {
            http.Error(w, "room parameter required", http.StatusBadRequest)
            return
        }

        page := 1
        if p := r.URL.Query().Get("page"); p != "" {
            if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
                page = parsed
            }
        }

        perPage := 50
        if pp := r.URL.Query().Get("per_page"); pp != "" {
            if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
                perPage = parsed
            }
        }

        since := int64(0)
        if s := r.URL.Query().Get("since"); s != "" {
            if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
                since = parsed
            }
        }

        result, err := storage.GetMessagesPaginated(db, room, page, perPage, since)
        if err != nil {
            http.Error(w, fmt.Sprintf("database error: %v", err), http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(result)
    }
}

// roomsHandler lists all rooms
func roomsHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        rooms, err := storage.ListRooms(db)
        if err != nil {
            http.Error(w, fmt.Sprintf("database error: %v", err), http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "rooms": rooms,
            "count": len(rooms),
        })
    }
}

// roomInfoHandler returns detailed info about a specific room
func roomInfoHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        room := r.URL.Query().Get("room")
        if room == "" {
            http.Error(w, "room parameter required", http.StatusBadRequest)
            return
        }

        info, err := storage.GetRoomInfo(db, room)
        if err != nil {
            http.Error(w, fmt.Sprintf("database error: %v", err), http.StatusInternalServerError)
            return
        }

        if info == nil {
            http.Error(w, "room not found", http.StatusNotFound)
            return
        }

        // Get peers in room
        peers, _ := storage.ListPeersInRoom(db, room)

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "room":  info,
            "peers": peers,
        })
    }
}

// peersHandler lists peers, optionally filtered by room
func peersHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        room := r.URL.Query().Get("room")
        peerID := r.URL.Query().Get("peer_id")

        var result interface{}
        var err error

        if peerID != "" {
            // Get specific peer info
            result, err = storage.GetPeerInfo(db, peerID)
            if result == nil && err == nil {
                http.Error(w, "peer not found", http.StatusNotFound)
                return
            }
        } else if room != "" {
            // Get peers in specific room
            result, err = storage.ListPeersInRoom(db, room)
        } else {
            http.Error(w, "room or peer_id parameter required", http.StatusBadRequest)
            return
        }

        if err != nil {
            http.Error(w, fmt.Sprintf("database error: %v", err), http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(result)
    }
}

// searchHandler searches messages
func searchHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        room := r.URL.Query().Get("room")
        query := r.URL.Query().Get("q")

        if room == "" || query == "" {
            http.Error(w, "room and q parameters required", http.StatusBadRequest)
            return
        }

        limit := 50
        if l := r.URL.Query().Get("limit"); l != "" {
            if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
                limit = parsed
            }
        }

        messages, err := storage.SearchMessages(db, room, query, limit)
        if err != nil {
            http.Error(w, fmt.Sprintf("search error: %v", err), http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "query":    query,
            "room":     room,
            "count":    len(messages),
            "messages": messages,
        })
    }
}

// dbStatsHandler returns database statistics
func dbStatsHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        stats, err := storage.GetDatabaseStats(db)
        if err != nil {
            http.Error(w, fmt.Sprintf("stats error: %v", err), http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(stats)
    }
}

// readinessHandler checks if the service is ready to accept traffic
func readinessHandler(db *sql.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Check database
        if err := db.Ping(); err != nil {
            http.Error(w, "database not ready", http.StatusServiceUnavailable)
            return
        }

        // Check transport manager
        if globalTransport == nil {
            http.Error(w, "transport not ready", http.StatusServiceUnavailable)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status": "ready",
            "checks": map[string]string{
                "database":  "ok",
                "transport": "ok",
            },
        })
    }
}
