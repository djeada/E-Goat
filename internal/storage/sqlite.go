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

    schema := []string{
        `CREATE TABLE IF NOT EXISTS peers (
            id        INTEGER PRIMARY KEY AUTOINCREMENT,
            peer_id   TEXT    NOT NULL UNIQUE,
            last_seen INTEGER
        );`,
        `CREATE TABLE IF NOT EXISTS messages (
            id        INTEGER PRIMARY KEY AUTOINCREMENT,
            direction TEXT    NOT NULL,
            timestamp INTEGER NOT NULL,
            msg_type  TEXT    NOT NULL,
            content   BLOB    NOT NULL,
            filename  TEXT
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
