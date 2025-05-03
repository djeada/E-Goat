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
    flag.IntVar(&httpPort, "http-port", 8080, "Port for HTTP server (UI + chat WS)")
    flag.IntVar(&wsPort, "ws-port", 9000, "Port for signaling WebSocket server")
    flag.StringVar(&dbPath, "db", "chat.db", "Path to SQLite database file")
}

func main() {
    flag.Parse()

    // 1. Initialize SQLite database
    db, err := storage.InitDB(dbPath)
    if err != nil {
        log.Fatalf("Database initialization failed: %v", err)
    }
    defer db.Close()

    // 2. Create & run the signaling/chat Hub
    hub := signaling.NewHub(db)
    go hub.Run()

    // 3. Prepare embedded web assets
    contentFS, err := fs.Sub(embeddedFS, "web")
    if err != nil {
        log.Fatalf("Failed to locate embedded web assets: %v", err)
    }

    // 4. HTTP mux for UI + chat WS
    httpMux := http.NewServeMux()
    // Chat WebSocket (over HTTP port)
    httpMux.HandleFunc("/chat", hub.ServeChat)
    // Static assets and UI
    httpMux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(contentFS))))
    httpMux.HandleFunc("/", indexHandler)

    httpSrv := &http.Server{
        Addr:    fmt.Sprintf(":%d", httpPort),
        Handler: httpMux,
    }

    go func() {
        log.Printf("HTTP server listening on http://localhost:%d", httpPort)
        if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("HTTP server error: %v", err)
        }
    }()

    // 5. Separate server for signaling WS (over WS port)
    wsMux := http.NewServeMux()
    wsMux.Handle("/signal", hub) // Hub implements ServeHTTP for signaling
    wsSrv := &http.Server{
        Addr:    fmt.Sprintf(":%d", wsPort),
        Handler: wsMux,
    }

    go func() {
        log.Printf("Signaling WebSocket server listening on ws://localhost:%d/signal", wsPort)
        if err := wsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("WebSocket server error: %v", err)
        }
    }()

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

// indexHandler serves `index.html`, injecting ports for the client
func indexHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    // Expose both ports to the browser
    portScript := fmt.Sprintf(
        `<script>window.config = { wsPort: %d, httpPort: %d };</script>`,
        wsPort, httpPort,
    )
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

// newPeerID generates a unique identifier for each client (if you need it elsewhere)
func newPeerID() string {
    return uuid.New().String()
}
