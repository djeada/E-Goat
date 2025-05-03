package main

import (
    "context"
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

    "github.com/djeada/E-Goat/internal/storage"
    "github.com/djeada/E-Goat/internal/signaling"
)

var (
    //go:embed web/*
    embeddedFS embed.FS

    httpPort int
    wsPort   int
    dbPath   string
)

func init() {
    flag.IntVar(&httpPort, "http-port", 8080, "Port for the HTTP server (UI)")
    flag.IntVar(&wsPort, "ws-port", 9000, "Port for WebSocket signaling server")
    flag.StringVar(&dbPath, "db", "chat.db", "Path to SQLite database file")
}

// WebSocket upgrader config (used by the signaling Hub internally)
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
    flag.Parse()

    // 1. Init DB
    db, err := storage.InitDB(dbPath)
    if err != nil {
        log.Fatalf("Database initialization failed: %v", err)
    }
    defer db.Close()

    // 2. Create & run signaling Hub
    hub := signaling.NewHub(db)  // assumes NewHub takes *sql.DB
    go hub.Run()

    // 3. Prepare embedded assets
    contentFS, err := fs.Sub(embeddedFS, "web")
    if err != nil {
        log.Fatalf("Failed to locate embedded web assets: %v", err)
    }

    // 4. HTTP mux
    mux := http.NewServeMux()
    mux.HandleFunc("/signal", hub.ServeWS) // websocket endpoint for signaling
    mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(contentFS))))
    mux.HandleFunc("/", indexHandler)

    // 5. Start HTTP server
    srv := &http.Server{
        Addr:    fmt.Sprintf(":%d", httpPort),
        Handler: mux,
    }
    go func() {
        log.Printf("HTTP server listening on http://localhost%s", srv.Addr)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("HTTP server error: %v", err)
        }
    }()

    // 6. Start WebSocket (signaling) on its own port if desired
    go func() {
        wsSrv := &http.Server{
            Addr: fmt.Sprintf(":%d", wsPort),
            Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if r.URL.Path == "/signal" {
                    hub.ServeWS(w, r)
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

    // 7. Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
    <-quit
    log.Println("Shutting down servers...")
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        log.Printf("HTTP server shutdown error: %v", err)
    }
}

// indexHandler serves `index.html`, injecting wsPort for the client
func indexHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    // expose wsPort in the browser
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

// newPeerID generates a unique identifier for each connecting peer
func newPeerID() string {
    return uuid.New().String()
}
