package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Message struct {
	ID        int64  `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Emotion   string `json:"emotion"`
	CreatedAt string `json:"createdAt"`
}

type ChatReply struct {
	Messages    []Message `json:"messages"`
	Reply       Message   `json:"reply"`
	Emotion     string    `json:"emotion"`
	AgentStatus string    `json:"agentStatus"`
}

type AppState struct {
	Messages    []Message `json:"messages"`
	Emotion     string    `json:"emotion"`
	AgentStatus string    `json:"agentStatus"`
}

type agentRequest struct {
	Message  string    `json:"message"`
	History  []Message `json:"history"`
	Memories []string  `json:"memories"`
}

type agentResponse struct {
	Text             string   `json:"text"`
	Emotion          string   `json:"emotion"`
	MemoryCandidates []string `json:"memoryCandidates"`
}

type App struct {
	ctx         context.Context
	db          *sql.DB
	httpClient  *http.Client
	agentURL    string
	agentStatus string
	agentCmd    *exec.Cmd
}

func NewApp() *App {
	return &App{
		httpClient:  &http.Client{Timeout: 3 * time.Second},
		agentURL:    "http://127.0.0.1:8765",
		agentStatus: "offline",
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if value := strings.TrimSpace(os.Getenv("MOCHI_AGENT_URL")); value != "" {
		a.agentURL = strings.TrimRight(value, "/")
	}
	if err := a.openDatabase(); err != nil {
		println("database startup error:", err.Error())
	}
	a.startAgentIfAvailable()
}

func (a *App) shutdown(ctx context.Context) {
	if a.agentCmd != nil && a.agentCmd.Process != nil {
		_ = a.agentCmd.Process.Kill()
	}
	if a.db != nil {
		_ = a.db.Close()
	}
}

func (a *App) openDatabase() error {
	if err := os.MkdirAll("data", 0755); err != nil {
		return err
	}

	db, err := sql.Open("sqlite", filepath.Join("data", "mochi.db"))
	if err != nil {
		return err
	}

	schema := `
CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	emotion TEXT NOT NULL DEFAULT 'neutral',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS memories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	kind TEXT NOT NULL,
	content TEXT NOT NULL,
	source_message_id INTEGER,
	created_at TEXT NOT NULL
);`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return err
	}

	a.db = db
	return nil
}

func (a *App) startAgentIfAvailable() {
	if a.pingAgent() {
		a.agentStatus = "online"
		return
	}

	scriptPath, ok := findAgentScript()
	if !ok {
		a.agentStatus = "offline"
		return
	}

	cmd := exec.CommandContext(a.ctx, "python", scriptPath)
	cmd.Dir = filepath.Dir(filepath.Dir(scriptPath))
	if err := cmd.Start(); err != nil {
		a.agentStatus = "offline"
		return
	}

	a.agentCmd = cmd
	a.agentStatus = "starting"
	go func() {
		_ = cmd.Wait()
		if a.agentStatus == "online" {
			a.agentStatus = "offline"
		}
	}()

	for i := 0; i < 20; i++ {
		if a.pingAgent() {
			a.agentStatus = "online"
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func findAgentScript() (string, bool) {
	candidates := []string{
		filepath.Join("agent", "main.py"),
		filepath.Join("..", "agent", "main.py"),
		filepath.Join("..", "..", "agent", "main.py"),
	}

	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDir, "agent", "main.py"),
			filepath.Join(executableDir, "..", "agent", "main.py"),
			filepath.Join(executableDir, "..", "..", "agent", "main.py"),
		)
	}

	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absolute); err == nil {
			return absolute, true
		}
	}

	return "", false
}

func (a *App) pingAgent() bool {
	request, err := http.NewRequest(http.MethodGet, a.agentURL+"/health", nil)
	if err != nil {
		return false
	}
	response, err := a.httpClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300
}

func (a *App) GetState() (AppState, error) {
	if a.db == nil {
		return AppState{}, errors.New("database is not ready")
	}

	messages, err := a.fetchMessages()
	if err != nil {
		return AppState{}, err
	}

	emotion := "neutral"
	if len(messages) > 0 {
		emotion = messages[len(messages)-1].Emotion
	}

	if a.pingAgent() {
		a.agentStatus = "online"
	}

	return AppState{
		Messages:    messages,
		Emotion:     emotion,
		AgentStatus: a.agentStatus,
	}, nil
}

func (a *App) SendMessage(content string) (ChatReply, error) {
	if a.db == nil {
		return ChatReply{}, errors.New("database is not ready")
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return ChatReply{}, errors.New("message cannot be empty")
	}

	userMessage, err := a.saveMessage("user", content, "focused")
	if err != nil {
		return ChatReply{}, err
	}

	history, err := a.fetchMessages()
	if err != nil {
		return ChatReply{}, err
	}
	memories, err := a.fetchMemories()
	if err != nil {
		return ChatReply{}, err
	}

	agentReply, err := a.askAgent(content, history, memories)
	if err != nil {
		agentReply = a.generateFallbackReply(content)
		a.agentStatus = "offline"
	} else {
		a.agentStatus = "online"
	}

	reply, err := a.saveMessage("assistant", agentReply.Text, normalizedEmotion(agentReply.Emotion))
	if err != nil {
		return ChatReply{}, err
	}

	a.extractSimpleMemory(userMessage)
	a.saveMemoryCandidates(agentReply.MemoryCandidates, userMessage.ID)

	messages, err := a.fetchMessages()
	if err != nil {
		return ChatReply{}, err
	}

	return ChatReply{
		Messages:    messages,
		Reply:       reply,
		Emotion:     reply.Emotion,
		AgentStatus: a.agentStatus,
	}, nil
}

func (a *App) askAgent(content string, history []Message, memories []string) (agentResponse, error) {
	payload, err := json.Marshal(agentRequest{
		Message:  content,
		History:  history,
		Memories: memories,
	})
	if err != nil {
		return agentResponse{}, err
	}

	response, err := a.httpClient.Post(a.agentURL+"/chat", "application/json", bytes.NewReader(payload))
	if err != nil {
		return agentResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return agentResponse{}, errors.New("agent returned a non-success status")
	}

	var reply agentResponse
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		return agentResponse{}, err
	}
	if strings.TrimSpace(reply.Text) == "" {
		return agentResponse{}, errors.New("agent returned an empty reply")
	}
	return reply, nil
}

func (a *App) saveMessage(role string, content string, emotion string) (Message, error) {
	createdAt := time.Now().Format(time.RFC3339)
	result, err := a.db.Exec(
		"INSERT INTO messages(role, content, emotion, created_at) VALUES(?, ?, ?, ?)",
		role,
		content,
		emotion,
		createdAt,
	)
	if err != nil {
		return Message{}, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Message{}, err
	}

	return Message{
		ID:        id,
		Role:      role,
		Content:   content,
		Emotion:   emotion,
		CreatedAt: createdAt,
	}, nil
}

func (a *App) fetchMessages() ([]Message, error) {
	rows, err := a.db.Query(`
SELECT id, role, content, emotion, created_at
FROM messages
ORDER BY id ASC
LIMIT 80`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := []Message{}
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.ID, &message.Role, &message.Content, &message.Emotion, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}

	return messages, rows.Err()
}

func (a *App) fetchMemories() ([]string, error) {
	rows, err := a.db.Query(`
SELECT content
FROM memories
ORDER BY id DESC
LIMIT 12`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memories := []string{}
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		memories = append(memories, content)
	}

	return memories, rows.Err()
}

func (a *App) generateFallbackReply(content string) agentResponse {
	lower := strings.ToLower(content)

	switch {
	case strings.Contains(lower, "remember"):
		return agentResponse{Text: "I saved that as a memory candidate. The Python Agent is offline, so this is the Go fallback path.", Emotion: "happy"}
	case strings.Contains(lower, "code"):
		return agentResponse{Text: "The code tool path will live in the Python Agent. Right now the desktop shell is keeping the chat and memory loop stable.", Emotion: "focused"}
	default:
		return agentResponse{Text: "The Python Agent is offline, so I am answering from the Go fallback. Start the agent with python agent/main.py or relaunch the app.", Emotion: "neutral"}
	}
}

func (a *App) extractSimpleMemory(message Message) {
	content := strings.TrimSpace(message.Content)
	lower := strings.ToLower(content)
	if !(strings.Contains(content, "记住") || strings.Contains(content, "喜欢") || strings.Contains(lower, "remember") || strings.Contains(lower, "like")) {
		return
	}

	_, _ = a.db.Exec(
		"INSERT INTO memories(kind, content, source_message_id, created_at) VALUES(?, ?, ?, ?)",
		"preference",
		content,
		message.ID,
		time.Now().Format(time.RFC3339),
	)
}

func (a *App) saveMemoryCandidates(candidates []string, sourceMessageID int64) {
	for _, candidate := range candidates {
		content := strings.TrimSpace(candidate)
		if content == "" {
			continue
		}
		_, _ = a.db.Exec(
			"INSERT INTO memories(kind, content, source_message_id, created_at) VALUES(?, ?, ?, ?)",
			"agent",
			content,
			sourceMessageID,
			time.Now().Format(time.RFC3339),
		)
	}
}

func normalizedEmotion(emotion string) string {
	switch strings.ToLower(strings.TrimSpace(emotion)) {
	case "happy", "focused", "thinking", "sad", "surprised":
		return strings.ToLower(strings.TrimSpace(emotion))
	default:
		return "neutral"
	}
}
