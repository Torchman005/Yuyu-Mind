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

func TestMessageEmotionRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "emotion.db")
	database, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer database.Close()

	conversation := &Conversation{
		ID:        "conversation-emotion",
		Title:     "Emotion",
		Provider:  "openai",
		Model:     "gpt-4o",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.Conversations.Create(context.Background(), conversation); err != nil {
		t.Fatalf("Create conversation: %v", err)
	}

	now := time.Now()
	msg := &Message{
		ID:             "message-emotion-1",
		ConversationID: conversation.ID,
		Role:           "assistant",
		Content:        "太好了，我们开始吧！",
		Emotion:        "happy",
		Mood:           "cheer",
		Energy:         0.8,
		Gesture:        "bounce",
		Hand:           "left",
		CreatedAt:      now,
	}
	if err := database.Messages.Create(context.Background(), msg); err != nil {
		t.Fatalf("Create message: %v", err)
	}

	rows, err := database.Messages.ListByConversation(context.Background(), conversation.ID)
	if err != nil {
		t.Fatalf("ListByConversation: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 message, got %d", len(rows))
	}
	got := rows[0]
	if got.Emotion != "happy" || got.Mood != "cheer" || got.Energy != 0.8 || got.Gesture != "bounce" || got.Hand != "left" {
		t.Fatalf("emotion round-trip mismatch: %+v", got)
	}
}
