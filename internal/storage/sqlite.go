// internal/storage/sqlite.go
package storage

import (
    "database/sql"
    "fmt"

    _ "github.com/mattn/go-sqlite3"
)

// InitDB opens (or creates) the SQLite file at path, applies schema, and returns the *sql.DB.
func InitDB(path string) (*sql.DB, error) {
    db, err := sql.Open("sqlite3", path)
    if err != nil {
        return nil, fmt.Errorf("opening database: %w", err)
    }

    // Schema now includes 'room' and 'peer_id' on messages
    schema := []string{
        `CREATE TABLE IF NOT EXISTS peers (
            id        INTEGER PRIMARY KEY AUTOINCREMENT,
            peer_id   TEXT    NOT NULL UNIQUE,
            last_seen INTEGER
        );`,
        `CREATE TABLE IF NOT EXISTS messages (
            id         INTEGER PRIMARY KEY AUTOINCREMENT,
            room       TEXT    NOT NULL,
            peer_id    TEXT    NOT NULL,
            timestamp  INTEGER NOT NULL,
            msg_type   TEXT    NOT NULL,
            content    BLOB    NOT NULL,
            filename   TEXT
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

// SaveMessage writes a chat message (text or file chunk) into the messages table.
// filename can be empty when msgType != "file".
func SaveMessage(db *sql.DB, room, peerID, msgType string, content []byte, filename *string) error {
    _, err := db.Exec(
        `INSERT INTO messages(room, peer_id, timestamp, msg_type, content, filename)
         VALUES(?, ?, strftime('%s','now'), ?, ?, ?)`,
        room, peerID, msgType, content, filename,
    )
    return err
}
