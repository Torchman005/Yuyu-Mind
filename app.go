package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	Messages      []Message `json:"messages"`
	Reply         Message   `json:"reply"`
	SpeechText    string    `json:"speechText"`
	Emotion       string    `json:"emotion"`
	AgentStatus   string    `json:"agentStatus"`
	AgentProvider string    `json:"agentProvider"`
	ProviderError string    `json:"providerError"`
}

type AppState struct {
	Messages      []Message `json:"messages"`
	Emotion       string    `json:"emotion"`
	AgentStatus   string    `json:"agentStatus"`
	AgentProvider string    `json:"agentProvider"`
	ProviderError string    `json:"providerError"`
}

type SpeechReply struct {
	AudioBase64 string `json:"audioBase64"`
	ContentType string `json:"contentType"`
	Provider    string `json:"provider"`
}

type agentRequest struct {
	Message  string    `json:"message"`
	History  []Message `json:"history"`
	Memories []string  `json:"memories"`
}

type agentResponse struct {
	Text             string   `json:"text"`
	SpeechText       string   `json:"speechText"`
	Emotion          string   `json:"emotion"`
	MemoryCandidates []string `json:"memoryCandidates"`
	Provider         string   `json:"provider"`
	ProviderError    string   `json:"providerError"`
}

type App struct {
	ctx          context.Context
	db           *sql.DB
	healthClient *http.Client
	chatClient   *http.Client
	agentURL     string
	agentStatus  string
	provider     string
	providerErr  string
}

func NewApp() *App {
	return &App{
		healthClient: &http.Client{Timeout: 3 * time.Second},
		chatClient:   &http.Client{Timeout: 90 * time.Second},
		agentURL:     "http://127.0.0.1:8765",
		agentStatus:  "offline",
		provider:     "unknown",
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	loadDotenv()
	if value := strings.TrimSpace(os.Getenv("MOCHI_AGENT_URL")); value != "" {
		a.agentURL = strings.TrimRight(value, "/")
	}
	if err := a.openDatabase(); err != nil {
		println("database startup error:", err.Error())
	}
	a.refreshAgentStatus()
}

func loadDotenv() {
	candidates := []string{".env", filepath.Join("..", ".env"), filepath.Join("..", "..", ".env")}
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDir, ".env"),
			filepath.Join(executableDir, "..", ".env"),
			filepath.Join(executableDir, "..", "..", ".env"),
		)
	}

	var content []byte
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil {
			content = data
			break
		}
	}
	if len(content) == 0 {
		return
	}

	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func (a *App) shutdown(ctx context.Context) {
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

func (a *App) refreshAgentStatus() {
	if a.pingAgent() {
		a.agentStatus = "online"
		return
	}

	a.agentStatus = "offline"
}

func (a *App) pingAgent() bool {
	request, err := http.NewRequest(http.MethodGet, a.agentURL+"/health", nil)
	if err != nil {
		return false
	}
	response, err := a.healthClient.Do(request)
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
		Messages:      messages,
		Emotion:       emotion,
		AgentStatus:   a.agentStatus,
		AgentProvider: a.provider,
		ProviderError: a.providerErr,
	}, nil
}

func (a *App) ClearChat() (AppState, error) {
	if a.db == nil {
		return AppState{}, errors.New("database is not ready")
	}

	if _, err := a.db.Exec("DELETE FROM messages"); err != nil {
		return AppState{}, err
	}

	return AppState{
		Messages:      []Message{},
		Emotion:       "neutral",
		AgentStatus:   a.agentStatus,
		AgentProvider: a.provider,
		ProviderError: a.providerErr,
	}, nil
}

func (a *App) SynthesizeSpeech(text string) (SpeechReply, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return SpeechReply{}, errors.New("speech text cannot be empty")
	}

	if reply, err := a.askPythonSpeech(text); err == nil {
		return reply, nil
	} else {
		return SpeechReply{}, err
	}
}

func (a *App) askPythonSpeech(text string) (SpeechReply, error) {
	scriptPath, ok := findFishTTSScript()
	if !ok {
		return SpeechReply{}, errors.New("agent/tts_fish.py was not found")
	}

	cmd := exec.CommandContext(a.ctx, findFishPythonExecutable(), scriptPath)
	cmd.Dir = filepath.Dir(filepath.Dir(scriptPath))
	cmd.Stdin = strings.NewReader(text)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return SpeechReply{}, fmt.Errorf("Python Fish Audio TTS failed: %s", detail)
	}

	var reply SpeechReply
	if err := json.Unmarshal(stdout.Bytes(), &reply); err != nil {
		return SpeechReply{}, err
	}
	if strings.TrimSpace(reply.AudioBase64) == "" {
		return SpeechReply{}, errors.New("Python Fish Audio TTS returned empty audio")
	}
	if strings.TrimSpace(reply.ContentType) == "" {
		reply.ContentType = "audio/mpeg"
	}
	if strings.TrimSpace(reply.Provider) == "" {
		reply.Provider = "fish-audio-python-script"
	}
	return reply, nil
}

func findFishPythonExecutable() string {
	if value := strings.TrimSpace(os.Getenv("FISH_AUDIO_PYTHON_PATH")); value != "" {
		if _, err := os.Stat(value); err == nil {
			return value
		}
	}

	candidates := []string{
		filepath.Join(".venv", "Scripts", "python.exe"),
		filepath.Join("..", ".venv", "Scripts", "python.exe"),
		filepath.Join("..", "..", ".venv", "Scripts", "python.exe"),
	}
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDir, ".venv", "Scripts", "python.exe"),
			filepath.Join(executableDir, "..", ".venv", "Scripts", "python.exe"),
			filepath.Join(executableDir, "..", "..", ".venv", "Scripts", "python.exe"),
		)
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absolute); err == nil {
			return absolute
		}
	}

	return "python"
}

func findFishTTSScript() (string, bool) {
	candidates := []string{
		filepath.Join("agent", "tts_fish.py"),
		filepath.Join("..", "agent", "tts_fish.py"),
		filepath.Join("..", "..", "agent", "tts_fish.py"),
	}

	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDir, "agent", "tts_fish.py"),
			filepath.Join(executableDir, "..", "agent", "tts_fish.py"),
			filepath.Join(executableDir, "..", "..", "agent", "tts_fish.py"),
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
		a.provider = "go-fallback"
		a.providerErr = err.Error()
	} else {
		a.agentStatus = "online"
		a.provider = strings.TrimSpace(agentReply.Provider)
		a.providerErr = strings.TrimSpace(agentReply.ProviderError)
		if a.provider == "" {
			a.provider = "agent"
		}
		if expectsDualLanguageSpeech() && strings.TrimSpace(agentReply.SpeechText) == "" {
			a.providerErr = "Agent did not return speechText. Please stop the old python agent/main.py process and restart MochiAI."
		}
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
		Messages:      messages,
		Reply:         reply,
		SpeechText:    chooseSpeechText(agentReply, reply.Content),
		Emotion:       reply.Emotion,
		AgentStatus:   a.agentStatus,
		AgentProvider: a.provider,
		ProviderError: a.providerErr,
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

	response, err := a.chatClient.Post(a.agentURL+"/chat", "application/json", bytes.NewReader(payload))
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
	name := desktopPetName()
	lower := strings.ToLower(content)
	useJapanese := strings.HasPrefix(strings.ToLower(strings.TrimSpace(os.Getenv("MOCHI_REPLY_LANGUAGE"))), "ja")

	switch {
	case strings.Contains(lower, "remember"):
		if useJapanese {
			return agentResponse{Text: "好，我会记住这件事。", SpeechText: "うん、覚えておくね。大事なこととして、ちゃんと残しておくよ。", Emotion: "happy"}
		}
		return agentResponse{Text: "I saved that as a memory candidate. The Python Agent is offline, so this is the Go fallback path.", Emotion: "happy"}
	case strings.Contains(lower, "code"):
		if useJapanese {
			return agentResponse{Text: "这是代码相关问题。当前是本地兜底模式，我会先尽量帮你分析。", SpeechText: "コードのことだね。今はローカルの予備モードだけど、できるところから一緒に見ていくよ。", Emotion: "focused"}
		}
		return agentResponse{Text: "The code tool path will live in the Python Agent. Right now the desktop shell is keeping the chat and memory loop stable.", Emotion: "focused"}
	default:
		if useJapanese {
			return agentResponse{Text: "Python Agent 似乎暂时离线了，请重启应用或手动启动 Agent。", SpeechText: fmt.Sprintf("今はPython Agentが少しお休み中みたい。%sはここにいるから、もう一度起動してみてね。", name), Emotion: "neutral"}
		}
		return agentResponse{Text: "The Python Agent is offline, so I am answering from the Go fallback. Start the agent with python agent/main.py or relaunch the app.", Emotion: "neutral"}
	}
}

func desktopPetName() string {
	name := strings.TrimSpace(os.Getenv("MOCHI_DESKTOP_PET_NAME"))
	if name == "" {
		return "Mochi"
	}
	return name
}

func chooseSpeechText(reply agentResponse, fallback string) string {
	speechText := strings.TrimSpace(reply.SpeechText)
	if speechText != "" {
		return speechText
	}
	if expectsDualLanguageSpeech() {
		return "ごめんね。音声用の日本語テキストがまだ届いていないみたい。画面の返事を確認してね。"
	}
	return strings.TrimSpace(fallback)
}

func expectsDualLanguageSpeech() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MOCHI_REPLY_LANGUAGE"))) {
	case "zh_ja", "zh-ja", "dual", "bilingual":
		return true
	default:
		return false
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
