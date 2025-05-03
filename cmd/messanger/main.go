package main

import (
    "context"
    "database/sql"
    "embed"
    "flag"
    "fmt"
    "io/fs"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/gorilla/websocket"
    "github.com/google/uuid"
    _ "github.com/mattn/go-sqlite3"
)

var (
    //go:embed web/*
    embeddedFS embed.FS

    httpPort int
    wsPort   int
    dbPath   string
    db        *sql.DB
)

func init() {
    flag.IntVar(&httpPort, "http-port", 8080, "Port for the HTTP server (UI)")
    flag.IntVar(&wsPort, "ws-port", 9000, "Port for WebSocket signaling server")
    flag.StringVar(&dbPath, "db", "chat.db", "Path to SQLite database file")
}

// WebSocket upgrader configuration
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
    flag.Parse()

    // Initialize database
    var err error
    db, err = initDB(dbPath)
    if err != nil {
        log.Fatalf("Database initialization failed: %v", err)
    }
    defer db.Close()

    // Prepare embedded web assets
    contentFS, err := fs.Sub(embeddedFS, "web")
    if err != nil {
        log.Fatalf("Failed to locate embedded web assets: %v", err)
    }

    mux := http.NewServeMux()
    mux.HandleFunc("/signal", signalingHandler)
    mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(contentFS))))
    mux.HandleFunc("/", indexHandler)

    // HTTP server
    srv := &http.Server{
        Addr:    fmt.Sprintf(":%d", httpPort),
        Handler: mux,
    }

    // WebSocket (signaling) server on separate port
    go func() {
        wsSrv := &http.Server{
            Addr:    fmt.Sprintf(":%d", wsPort),
            Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if r.URL.Path == "/signal" {
                    signalingHandler(w, r)
                } else {
                    http.NotFound(w, r)
                }
            }),
        }
        log.Printf("WebSocket signaling server listening on %s", wsSrv.Addr)
        if err := wsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("WebSocket server error: %v", err)
        }
    }()

    // Graceful shutdown setup
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

    go func() {
        log.Printf("HTTP server listening on http://localhost%s", srv.Addr)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("HTTP server error: %v", err)
        }
    }()

    <-quit
    log.Println("Shutting down servers...")
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        log.Printf("HTTP server shutdown error: %v", err)
    }
}

// initDB opens or creates the SQLite database and applies the schema
func initDB(path string) (*sql.DB, error) {
    db, err := sql.Open("sqlite3", path)
    if err != nil {
        return nil, fmt.Errorf("opening database: %w", err)
    }

    schema := []string{
        `CREATE TABLE IF NOT EXISTS peers (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            peer_id TEXT NOT NULL UNIQUE,
            last_seen INTEGER
        );`,
        `CREATE TABLE IF NOT EXISTS messages (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            direction TEXT NOT NULL,
            timestamp INTEGER NOT NULL,
            msg_type TEXT NOT NULL,
            content BLOB NOT NULL,
            filename TEXT
        );`,
    }
    for _, stmt := range schema {
        if _, err := db.Exec(stmt); err != nil {
            db.Close()
            return nil, fmt.Errorf("applying schema: %w", err)
        }
    }
    return db, nil
}

// signalingHandler upgrades HTTP connections to WebSocket and echoes messages
func signalingHandler(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("WebSocket upgrade error: %v", err)
        return
    }
    defer conn.Close()

    for {
        msgType, msg, err := conn.ReadMessage()
        if err != nil {
            log.Printf("WebSocket read error: %v", err)
            return
        }
        // Echo back the message (replace with room-based dispatch in production)
        if err := conn.WriteMessage(msgType, msg); err != nil {
            log.Printf("WebSocket write error: %v", err)
            return
        }
    }
}

// indexHandler serves the main web UI page
func indexHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    portScript := fmt.Sprintf(`<script>window.config = { wsPort: %d };</script>`, wsPort)
    if _, err := w.Write([]byte(portScript)); err != nil {
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

// newPeerID generates a unique identifier for peers
func newPeerID() string {
    return uuid.New().String()
}
