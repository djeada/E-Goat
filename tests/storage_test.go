// tests/storage_test.go
package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/djeada/E-Goat/internal/storage"
)

func setupTestDB(t *testing.T) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "egoat-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return dbPath, cleanup
}

func TestInitDB(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := storage.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Verify we can ping the database
	if err := db.Ping(); err != nil {
		t.Errorf("Database ping failed: %v", err)
	}
}

func TestSaveAndRetrieveMessage(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := storage.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Save a message
	err = storage.SaveMessage(db, "test-room", "peer-123", "text", []byte("Hello, World!"), nil)
	if err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	// Retrieve messages with pagination
	result, err := storage.GetMessagesPaginated(db, "test-room", 1, 50, 0)
	if err != nil {
		t.Fatalf("GetMessagesPaginated failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Expected 1 message, got %d", result.Total)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("Expected 1 message in results, got %d", len(result.Messages))
	}

	if result.Messages[0].Text != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!', got '%s'", result.Messages[0].Text)
	}

	if result.Messages[0].PeerID != "peer-123" {
		t.Errorf("Expected peer-123, got '%s'", result.Messages[0].PeerID)
	}
}

func TestPagination(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := storage.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Save multiple messages
	for i := 0; i < 15; i++ {
		err = storage.SaveMessage(db, "test-room", "peer-1", "text", []byte("Message"), nil)
		if err != nil {
			t.Fatalf("SaveMessage %d failed: %v", i, err)
		}
		time.Sleep(time.Millisecond * 10) // Ensure different timestamps
	}

	// Get first page
	result, err := storage.GetMessagesPaginated(db, "test-room", 1, 10, 0)
	if err != nil {
		t.Fatalf("GetMessagesPaginated failed: %v", err)
	}

	if result.Total != 15 {
		t.Errorf("Expected 15 total messages, got %d", result.Total)
	}

	if len(result.Messages) != 10 {
		t.Errorf("Expected 10 messages on first page, got %d", len(result.Messages))
	}

	if result.TotalPages != 2 {
		t.Errorf("Expected 2 total pages, got %d", result.TotalPages)
	}

	if !result.HasMore {
		t.Error("Expected HasMore to be true")
	}

	// Get second page
	result, err = storage.GetMessagesPaginated(db, "test-room", 2, 10, 0)
	if err != nil {
		t.Fatalf("GetMessagesPaginated page 2 failed: %v", err)
	}

	if len(result.Messages) != 5 {
		t.Errorf("Expected 5 messages on second page, got %d", len(result.Messages))
	}

	if result.HasMore {
		t.Error("Expected HasMore to be false on last page")
	}
}

func TestSearchMessages(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := storage.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Save messages with different content
	storage.SaveMessage(db, "test-room", "peer-1", "text", []byte("Hello world"), nil)
	storage.SaveMessage(db, "test-room", "peer-1", "text", []byte("Goodbye world"), nil)
	storage.SaveMessage(db, "test-room", "peer-1", "text", []byte("Hello there"), nil)

	// Search for "Hello"
	results, err := storage.SearchMessages(db, "test-room", "Hello", 50)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 messages matching 'Hello', got %d", len(results))
	}

	// Search for "world"
	results, err = storage.SearchMessages(db, "test-room", "world", 50)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 messages matching 'world', got %d", len(results))
	}
}

func TestRoomInfo(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := storage.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Save messages from different peers
	storage.SaveMessage(db, "test-room", "peer-1", "text", []byte("Message 1"), nil)
	storage.SaveMessage(db, "test-room", "peer-2", "text", []byte("Message 2"), nil)
	storage.SaveMessage(db, "test-room", "peer-1", "text", []byte("Message 3"), nil)

	// Get room info
	info, err := storage.GetRoomInfo(db, "test-room")
	if err != nil {
		t.Fatalf("GetRoomInfo failed: %v", err)
	}

	if info == nil {
		t.Fatal("Expected room info, got nil")
	}

	if info.Name != "test-room" {
		t.Errorf("Expected room name 'test-room', got '%s'", info.Name)
	}

	if info.MessageCount != 3 {
		t.Errorf("Expected 3 messages, got %d", info.MessageCount)
	}

	if info.PeerCount != 2 {
		t.Errorf("Expected 2 peers, got %d", info.PeerCount)
	}
}

func TestListRooms(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := storage.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Create multiple rooms
	storage.SaveMessage(db, "room-1", "peer-1", "text", []byte("Message"), nil)
	storage.SaveMessage(db, "room-2", "peer-1", "text", []byte("Message"), nil)
	storage.SaveMessage(db, "room-3", "peer-1", "text", []byte("Message"), nil)

	// List rooms
	rooms, err := storage.ListRooms(db)
	if err != nil {
		t.Fatalf("ListRooms failed: %v", err)
	}

	if len(rooms) != 3 {
		t.Errorf("Expected 3 rooms, got %d", len(rooms))
	}
}

func TestPeerInfo(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := storage.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Create messages
	storage.SaveMessage(db, "room-1", "peer-123", "text", []byte("Message 1"), nil)
	storage.SaveMessage(db, "room-1", "peer-123", "text", []byte("Message 2"), nil)

	// Get peer info
	info, err := storage.GetPeerInfo(db, "peer-123")
	if err != nil {
		t.Fatalf("GetPeerInfo failed: %v", err)
	}

	if info == nil {
		t.Fatal("Expected peer info, got nil")
	}

	if info.PeerID != "peer-123" {
		t.Errorf("Expected peer ID 'peer-123', got '%s'", info.PeerID)
	}

	if info.MessageCount != 2 {
		t.Errorf("Expected 2 messages from peer, got %d", info.MessageCount)
	}
}

func TestDatabaseStats(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := storage.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Create some data
	storage.SaveMessage(db, "room-1", "peer-1", "text", []byte("Message"), nil)
	storage.SaveMessage(db, "room-2", "peer-2", "text", []byte("Message"), nil)

	// Get stats
	stats, err := storage.GetDatabaseStats(db)
	if err != nil {
		t.Fatalf("GetDatabaseStats failed: %v", err)
	}

	if stats.TotalMessages != 2 {
		t.Errorf("Expected 2 messages, got %d", stats.TotalMessages)
	}

	if stats.TotalRooms != 2 {
		t.Errorf("Expected 2 rooms, got %d", stats.TotalRooms)
	}

	if stats.TotalPeers != 2 {
		t.Errorf("Expected 2 peers, got %d", stats.TotalPeers)
	}

	if stats.DatabaseSize <= 0 {
		t.Error("Expected positive database size")
	}
}
