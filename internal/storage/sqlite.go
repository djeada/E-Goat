// internal/storage/sqlite.go
package storage

import (
    "database/sql"
    "fmt"
    "os"
    "path/filepath"
    "time"

    _ "github.com/mattn/go-sqlite3"
)

// InitDB opens (or creates) the SQLite file at path, applies schema, and returns the *sql.DB.
func InitDB(path string) (*sql.DB, error) {
    dir := filepath.Dir(path)
    if dir != "." {
        if err := os.MkdirAll(dir, 0o700); err != nil {
            return nil, fmt.Errorf("creating database directory: %w", err)
        }
    }

    db, err := sql.Open("sqlite3", path)
    if err != nil {
        return nil, fmt.Errorf("opening database: %w", err)
    }

    if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
        db.Close()
        return nil, fmt.Errorf("enabling foreign keys: %w", err)
    }

    if err := ensureSchema(db); err != nil {
        db.Close()
        return nil, err
    }

    return db, nil
}

// SaveMessage writes a chat message (text or file chunk) into the messages table.
// filename can be empty when msgType != "file".
func SaveMessage(db *sql.DB, room, peerID, msgType string, content []byte, filename *string) error {
    now := time.Now().Unix()
    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer func() {
        if err != nil {
            _ = tx.Rollback()
        }
    }()

    if _, err = tx.Exec(
        `INSERT INTO chats(name, created_at, last_active)
         VALUES(?, ?, ?)
         ON CONFLICT(name) DO UPDATE SET
           last_active = excluded.last_active,
           created_at = CASE
             WHEN chats.created_at IS NULL OR chats.created_at > excluded.created_at
             THEN excluded.created_at
             ELSE chats.created_at
           END`,
        room, now, now,
    ); err != nil {
        return err
    }

    if _, err = tx.Exec(
        `INSERT INTO peers(peer_id, first_seen, last_seen)
         VALUES(?, ?, ?)
         ON CONFLICT(peer_id) DO UPDATE SET
           last_seen = excluded.last_seen,
           first_seen = CASE
             WHEN peers.first_seen IS NULL OR peers.first_seen > excluded.first_seen
             THEN excluded.first_seen
             ELSE peers.first_seen
           END`,
        peerID, now, now,
    ); err != nil {
        return err
    }

    var chatID int64
    if err = tx.QueryRow(`SELECT id FROM chats WHERE name = ?`, room).Scan(&chatID); err != nil {
        return err
    }
    var peerRowID int64
    if err = tx.QueryRow(`SELECT id FROM peers WHERE peer_id = ?`, peerID).Scan(&peerRowID); err != nil {
        return err
    }

    if _, err = tx.Exec(
        `INSERT INTO messages(chat_id, peer_id, timestamp, msg_type, content, filename)
         VALUES(?, ?, ?, ?, ?, ?)`,
        chatID, peerRowID, now, msgType, content, filename,
    ); err != nil {
        return err
    }

    return tx.Commit()
}

func ensureSchema(db *sql.DB) error {
    schema := []string{
        `CREATE TABLE IF NOT EXISTS peers (
            id         INTEGER PRIMARY KEY AUTOINCREMENT,
            peer_id    TEXT    NOT NULL UNIQUE,
            first_seen INTEGER,
            last_seen  INTEGER
        );`,
        `CREATE TABLE IF NOT EXISTS chats (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            name        TEXT    NOT NULL UNIQUE,
            created_at  INTEGER NOT NULL,
            last_active INTEGER
        );`,
    }

    for _, stmt := range schema {
        if _, err := db.Exec(stmt); err != nil {
            return fmt.Errorf("applying schema: %w", err)
        }
    }

    if err := ensurePeersColumns(db); err != nil {
        return err
    }

    if hasColumn(db, "messages", "room") {
        if err := migrateMessages(db); err != nil {
            return err
        }
    } else {
        if err := ensureMessagesTable(db); err != nil {
            return err
        }
    }

    if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_chat_time ON messages(chat_id, timestamp)`); err != nil {
        return fmt.Errorf("creating messages index: %w", err)
    }
    if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_peer ON messages(peer_id)`); err != nil {
        return fmt.Errorf("creating peer index: %w", err)
    }

    return nil
}

type sqlExecer interface {
    Exec(query string, args ...any) (sql.Result, error)
}

func ensureMessagesTable(db sqlExecer) error {
    _, err := db.Exec(
        `CREATE TABLE IF NOT EXISTS messages (
            id         INTEGER PRIMARY KEY AUTOINCREMENT,
            chat_id    INTEGER NOT NULL,
            peer_id    INTEGER NOT NULL,
            timestamp  INTEGER NOT NULL,
            msg_type   TEXT    NOT NULL,
            content    BLOB    NOT NULL,
            filename   TEXT,
            FOREIGN KEY(chat_id) REFERENCES chats(id),
            FOREIGN KEY(peer_id) REFERENCES peers(id)
        );`,
    )
    if err != nil {
        return fmt.Errorf("creating messages table: %w", err)
    }
    return nil
}

func ensurePeersColumns(db *sql.DB) error {
    if !hasColumn(db, "peers", "first_seen") {
        if _, err := db.Exec(`ALTER TABLE peers ADD COLUMN first_seen INTEGER`); err != nil {
            return fmt.Errorf("adding peers.first_seen: %w", err)
        }
    }
    if !hasColumn(db, "peers", "last_seen") {
        if _, err := db.Exec(`ALTER TABLE peers ADD COLUMN last_seen INTEGER`); err != nil {
            return fmt.Errorf("adding peers.last_seen: %w", err)
        }
    }
    return nil
}

func hasColumn(db *sql.DB, table, column string) bool {
    rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s);`, table))
    if err != nil {
        return false
    }
    defer rows.Close()

    var (
        cid        int
        name       string
        ctype      string
        notnull    int
        dfltValue  sql.NullString
        pk         int
    )
    for rows.Next() {
        if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
            return false
        }
        if name == column {
            return true
        }
    }
    return false
}

func migrateMessages(db *sql.DB) error {
    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer func() {
        if err != nil {
            _ = tx.Rollback()
        }
    }()

    if _, err = tx.Exec(`ALTER TABLE messages RENAME TO messages_old`); err != nil {
        return fmt.Errorf("renaming messages table: %w", err)
    }
    if err = ensureMessagesTable(tx); err != nil {
        return err
    }

    if _, err = tx.Exec(
        `INSERT INTO chats(name, created_at, last_active)
         SELECT room, MIN(timestamp), MAX(timestamp)
           FROM messages_old
          GROUP BY room
         ON CONFLICT(name) DO UPDATE SET
           last_active = MAX(chats.last_active, excluded.last_active),
           created_at = MIN(chats.created_at, excluded.created_at)`,
    ); err != nil {
        return fmt.Errorf("migrating chats: %w", err)
    }

    if _, err = tx.Exec(
        `INSERT INTO peers(peer_id, first_seen, last_seen)
         SELECT peer_id, MIN(timestamp), MAX(timestamp)
           FROM messages_old
          GROUP BY peer_id
         ON CONFLICT(peer_id) DO UPDATE SET
           first_seen = MIN(peers.first_seen, excluded.first_seen),
           last_seen = MAX(peers.last_seen, excluded.last_seen)`,
    ); err != nil {
        return fmt.Errorf("migrating peers: %w", err)
    }

    if _, err = tx.Exec(
        `INSERT INTO messages(chat_id, peer_id, timestamp, msg_type, content, filename)
         SELECT c.id, p.id, m.timestamp, m.msg_type, m.content, m.filename
           FROM messages_old m
           JOIN chats c ON c.name = m.room
           JOIN peers p ON p.peer_id = m.peer_id`,
    ); err != nil {
        return fmt.Errorf("migrating messages: %w", err)
    }

    if _, err = tx.Exec(`DROP TABLE messages_old`); err != nil {
        return fmt.Errorf("dropping old messages table: %w", err)
    }

    return tx.Commit()
}

// ====== ENHANCED STORAGE FUNCTIONS ======

// PaginatedMessage represents a message with metadata for pagination
type PaginatedMessage struct {
    ID        int64  `json:"id"`
    PeerID    string `json:"peer_id"`
    Text      string `json:"text"`
    MsgType   string `json:"msg_type"`
    Timestamp int64  `json:"timestamp"`
    Filename  string `json:"filename,omitempty"`
}

// PaginatedResult contains paginated messages with metadata
type PaginatedResult struct {
    Messages   []PaginatedMessage `json:"messages"`
    Total      int64              `json:"total"`
    Page       int                `json:"page"`
    PerPage    int                `json:"per_page"`
    TotalPages int                `json:"total_pages"`
    HasMore    bool               `json:"has_more"`
}

// GetMessagesPaginated retrieves messages with pagination support
func GetMessagesPaginated(db *sql.DB, room string, page, perPage int, since int64) (*PaginatedResult, error) {
    if page < 1 {
        page = 1
    }
    if perPage < 1 || perPage > 100 {
        perPage = 50
    }
    offset := (page - 1) * perPage

    // Get total count
    var total int64
    err := db.QueryRow(`
        SELECT COUNT(*)
        FROM messages m
        JOIN chats c ON c.id = m.chat_id
        WHERE c.name = ? AND m.timestamp > ?
    `, room, since).Scan(&total)
    if err != nil {
        return nil, fmt.Errorf("counting messages: %w", err)
    }

    // Get paginated messages
    rows, err := db.Query(`
        SELECT m.id, p.peer_id, m.content, m.msg_type, m.timestamp, COALESCE(m.filename, '')
        FROM messages m
        JOIN chats c ON c.id = m.chat_id
        JOIN peers p ON p.id = m.peer_id
        WHERE c.name = ? AND m.timestamp > ?
        ORDER BY m.timestamp ASC
        LIMIT ? OFFSET ?
    `, room, since, perPage, offset)
    if err != nil {
        return nil, fmt.Errorf("querying messages: %w", err)
    }
    defer rows.Close()

    var messages []PaginatedMessage
    for rows.Next() {
        var msg PaginatedMessage
        var content []byte
        if err := rows.Scan(&msg.ID, &msg.PeerID, &content, &msg.MsgType, &msg.Timestamp, &msg.Filename); err != nil {
            continue
        }
        msg.Text = string(content)
        messages = append(messages, msg)
    }

    totalPages := int(total) / perPage
    if int(total)%perPage != 0 {
        totalPages++
    }

    return &PaginatedResult{
        Messages:   messages,
        Total:      total,
        Page:       page,
        PerPage:    perPage,
        TotalPages: totalPages,
        HasMore:    page < totalPages,
    }, nil
}

// SearchMessages searches messages by text content
func SearchMessages(db *sql.DB, room, query string, limit int) ([]PaginatedMessage, error) {
    if limit < 1 || limit > 100 {
        limit = 50
    }

    rows, err := db.Query(`
        SELECT m.id, p.peer_id, m.content, m.msg_type, m.timestamp, COALESCE(m.filename, '')
        FROM messages m
        JOIN chats c ON c.id = m.chat_id
        JOIN peers p ON p.id = m.peer_id
        WHERE c.name = ? AND m.content LIKE ?
        ORDER BY m.timestamp DESC
        LIMIT ?
    `, room, "%"+query+"%", limit)
    if err != nil {
        return nil, fmt.Errorf("searching messages: %w", err)
    }
    defer rows.Close()

    var messages []PaginatedMessage
    for rows.Next() {
        var msg PaginatedMessage
        var content []byte
        if err := rows.Scan(&msg.ID, &msg.PeerID, &content, &msg.MsgType, &msg.Timestamp, &msg.Filename); err != nil {
            continue
        }
        msg.Text = string(content)
        messages = append(messages, msg)
    }

    return messages, nil
}

// RoomInfo contains room metadata
type RoomInfo struct {
    Name         string `json:"name"`
    CreatedAt    int64  `json:"created_at"`
    LastActive   int64  `json:"last_active"`
    MessageCount int64  `json:"message_count"`
    PeerCount    int64  `json:"peer_count"`
}

// GetRoomInfo retrieves detailed room information
func GetRoomInfo(db *sql.DB, room string) (*RoomInfo, error) {
    var info RoomInfo
    info.Name = room

    err := db.QueryRow(`
        SELECT c.created_at, c.last_active,
               (SELECT COUNT(*) FROM messages m WHERE m.chat_id = c.id),
               (SELECT COUNT(DISTINCT m.peer_id) FROM messages m WHERE m.chat_id = c.id)
        FROM chats c
        WHERE c.name = ?
    `, room).Scan(&info.CreatedAt, &info.LastActive, &info.MessageCount, &info.PeerCount)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, nil
        }
        return nil, fmt.Errorf("getting room info: %w", err)
    }

    return &info, nil
}

// ListRooms lists all rooms with basic info
func ListRooms(db *sql.DB) ([]RoomInfo, error) {
    rows, err := db.Query(`
        SELECT c.name, c.created_at, c.last_active,
               (SELECT COUNT(*) FROM messages m WHERE m.chat_id = c.id),
               (SELECT COUNT(DISTINCT m.peer_id) FROM messages m WHERE m.chat_id = c.id)
        FROM chats c
        ORDER BY c.last_active DESC
    `)
    if err != nil {
        return nil, fmt.Errorf("listing rooms: %w", err)
    }
    defer rows.Close()

    var rooms []RoomInfo
    for rows.Next() {
        var info RoomInfo
        if err := rows.Scan(&info.Name, &info.CreatedAt, &info.LastActive, &info.MessageCount, &info.PeerCount); err != nil {
            continue
        }
        rooms = append(rooms, info)
    }

    return rooms, nil
}

// PeerInfo contains peer metadata
type PeerInfo struct {
    PeerID       string `json:"peer_id"`
    FirstSeen    int64  `json:"first_seen"`
    LastSeen     int64  `json:"last_seen"`
    MessageCount int64  `json:"message_count"`
}

// GetPeerInfo retrieves peer information
func GetPeerInfo(db *sql.DB, peerID string) (*PeerInfo, error) {
    var info PeerInfo

    err := db.QueryRow(`
        SELECT p.peer_id, p.first_seen, p.last_seen,
               (SELECT COUNT(*) FROM messages m WHERE m.peer_id = p.id)
        FROM peers p
        WHERE p.peer_id = ?
    `, peerID).Scan(&info.PeerID, &info.FirstSeen, &info.LastSeen, &info.MessageCount)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, nil
        }
        return nil, fmt.Errorf("getting peer info: %w", err)
    }

    return &info, nil
}

// ListPeersInRoom lists all peers who have messaged in a room
func ListPeersInRoom(db *sql.DB, room string) ([]PeerInfo, error) {
    rows, err := db.Query(`
        SELECT DISTINCT p.peer_id, p.first_seen, p.last_seen,
               (SELECT COUNT(*) FROM messages m2 WHERE m2.peer_id = p.id)
        FROM peers p
        JOIN messages m ON m.peer_id = p.id
        JOIN chats c ON c.id = m.chat_id
        WHERE c.name = ?
        ORDER BY p.last_seen DESC
    `, room)
    if err != nil {
        return nil, fmt.Errorf("listing peers: %w", err)
    }
    defer rows.Close()

    var peers []PeerInfo
    for rows.Next() {
        var info PeerInfo
        if err := rows.Scan(&info.PeerID, &info.FirstSeen, &info.LastSeen, &info.MessageCount); err != nil {
            continue
        }
        peers = append(peers, info)
    }

    return peers, nil
}

// DatabaseStats contains database statistics
type DatabaseStats struct {
    TotalMessages int64 `json:"total_messages"`
    TotalRooms    int64 `json:"total_rooms"`
    TotalPeers    int64 `json:"total_peers"`
    DatabaseSize  int64 `json:"database_size_bytes"`
}

// GetDatabaseStats retrieves database statistics
func GetDatabaseStats(db *sql.DB) (*DatabaseStats, error) {
    var stats DatabaseStats

    db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&stats.TotalMessages)
    db.QueryRow(`SELECT COUNT(*) FROM chats`).Scan(&stats.TotalRooms)
    db.QueryRow(`SELECT COUNT(*) FROM peers`).Scan(&stats.TotalPeers)

    // Get database size
    var pageCount, pageSize int64
    db.QueryRow(`PRAGMA page_count`).Scan(&pageCount)
    db.QueryRow(`PRAGMA page_size`).Scan(&pageSize)
    stats.DatabaseSize = pageCount * pageSize

    return &stats, nil
}

// OptimizeDatabase runs VACUUM and ANALYZE on the database
func OptimizeDatabase(db *sql.DB) error {
    if _, err := db.Exec(`VACUUM`); err != nil {
        return fmt.Errorf("vacuum failed: %w", err)
    }
    if _, err := db.Exec(`ANALYZE`); err != nil {
        return fmt.Errorf("analyze failed: %w", err)
    }
    return nil
}

// DeleteOldMessages removes messages older than the specified timestamp
func DeleteOldMessages(db *sql.DB, olderThan int64) (int64, error) {
    result, err := db.Exec(`DELETE FROM messages WHERE timestamp < ?`, olderThan)
    if err != nil {
        return 0, fmt.Errorf("deleting old messages: %w", err)
    }
    return result.RowsAffected()
}
