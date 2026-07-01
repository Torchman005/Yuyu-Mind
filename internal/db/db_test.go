package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCreatesDatabaseDirectoryAndRunsMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "yuyu-mind.db")

	database, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer database.Close()

	conversation := &Conversation{
		ID:        "conversation-1",
		Title:     "Test",
		Provider:  "openai",
		Model:     "gpt-4o",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.Conversations.Create(context.Background(), conversation); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}
