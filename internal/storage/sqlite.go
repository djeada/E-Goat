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
