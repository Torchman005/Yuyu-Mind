package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v2/pkg/runtime"
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

type ASRReply struct {
	Text     string  `json:"text"`
	Provider string  `json:"provider"`
	Language string  `json:"language"`
	Duration float64 `json:"duration"`
	Error    string  `json:"error,omitempty"`
}

type SpeechStreamStart struct {
	SessionID   string `json:"sessionId"`
	ContentType string `json:"contentType"`
	Provider    string `json:"provider"`
}

type SpeechStreamEvent struct {
	SessionID   string `json:"sessionId"`
	AudioBase64 string `json:"audioBase64,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Error       string `json:"error,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Phase       string `json:"phase,omitempty"`
	ElapsedMs   int64  `json:"elapsedMs,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

type FishLiveProbeResult struct {
	OK        bool     `json:"ok"`
	Error     string   `json:"error,omitempty"`
	Events    []string `json:"events"`
	ElapsedMs int64    `json:"elapsedMs"`
	AudioSize int      `json:"audioSize"`
}

type PetHitTestState struct {
	Enabled      bool    `json:"enabled"`
	ControlsOpen bool    `json:"controlsOpen"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
}

type agentRequest struct {
	Message  string    `json:"message"`
	History  []Message `json:"history"`
	Memories []string  `json:"memories"`
	Mode     string    `json:"mode,omitempty"`
}

type agentResponse struct {
	Text             string   `json:"text"`
	SpeechText       string   `json:"speechText"`
	Emotion          string   `json:"emotion"`
	MemoryCandidates []string `json:"memoryCandidates"`
	Provider         string   `json:"provider"`
	ProviderError    string   `json:"providerError"`
}

type PluginInfo struct {
	SchemaVersion string           `json:"schemaVersion"`
	Name          string           `json:"name"`
	DisplayName   string           `json:"displayName"`
	Description   string           `json:"description"`
	Version       string           `json:"version"`
	Author        string           `json:"author"`
	Enabled       bool             `json:"enabled"`
	Entry         string           `json:"entry"`
	Permissions   []string         `json:"permissions"`
	Context       map[string]any   `json:"context"`
	Config        map[string]any   `json:"config"`
	ConfigSchema  map[string]any   `json:"configSchema"`
	Actions       []map[string]any `json:"actions"`
	LoadedActions []string         `json:"loadedActions"`
}

type PluginListReply struct {
	OK      bool         `json:"ok"`
	Plugins []PluginInfo `json:"plugins"`
}

type PluginInvokeResult struct {
	OK          bool           `json:"ok"`
	Plugin      string         `json:"plugin"`
	Action      string         `json:"action"`
	Error       string         `json:"error"`
	Summary     string         `json:"summary"`
	Metadata    map[string]any `json:"metadata"`
	Vision      map[string]any `json:"vision"`
	ImageBase64 string         `json:"imageBase64"`
	ContentType string         `json:"contentType"`
}

type PluginContextCandidate struct {
	Plugin       string
	Action       string
	Mode         string
	Priority     int
	Prompt       string
	Result       PluginInvokeResult
	ProviderName string
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
	if value := envFirst("YUYU_AGENT_URL", "MOCHI_AGENT_URL"); value != "" {
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

	dbPath := filepath.Join("data", "yuyu-mind.db")
	legacyPath := filepath.Join("data", "mochi.db")
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		if _, legacyErr := os.Stat(legacyPath); legacyErr == nil {
			dbPath = legacyPath
		}
	}
	db, err := sql.Open("sqlite", dbPath)
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

func (a *App) ListPlugins() (PluginListReply, error) {
	request, err := http.NewRequest(http.MethodGet, a.agentURL+"/plugins", nil)
	if err != nil {
		return PluginListReply{}, err
	}
	response, err := a.chatClient.Do(request)
	if err != nil {
		return PluginListReply{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return PluginListReply{}, errors.New("agent plugin registry returned a non-success status")
	}
	var reply PluginListReply
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		return PluginListReply{}, err
	}
	return reply, nil
}

func (a *App) SynthesizeSpeech(text string) (SpeechReply, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return SpeechReply{}, errors.New("speech text cannot be empty")
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv("TTS_PROVIDER"))) {
	case "fish":
		if reply, err := a.askFishSpeechDirect(text); err == nil {
			return reply, nil
		} else if !envEnabled("FISH_AUDIO_ENABLE_PYTHON_FALLBACK") {
			return SpeechReply{}, err
		}
		return a.askPythonSpeech(text)
	case "gpt-sovits", "gptsovits", "sovits":
		return a.askGPTSoVITSSpeech(text)
	case "elevenlabs", "eleven":
		return a.askElevenLabsSpeech(text)
	case "cartesia":
		return a.askCartesiaSpeech(text)
	case "", "system":
		return SpeechReply{}, errors.New("cloud TTS provider is not configured")
	default:
		return SpeechReply{}, fmt.Errorf("unsupported TTS_PROVIDER: %s", os.Getenv("TTS_PROVIDER"))
	}
}

func (a *App) SynthesizeSpeechStream(text string) (SpeechStreamStart, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return SpeechStreamStart{}, errors.New("speech text cannot be empty")
	}
	if strings.ToLower(strings.TrimSpace(os.Getenv("TTS_PROVIDER"))) != "fish" {
		return SpeechStreamStart{}, errors.New("streaming TTS is currently available for TTS_PROVIDER=fish")
	}
	if strings.TrimSpace(os.Getenv("FISH_AUDIO_API_KEY")) == "" {
		return SpeechStreamStart{}, errors.New("FISH_AUDIO_API_KEY is not configured")
	}
	if strings.TrimSpace(os.Getenv("FISH_AUDIO_REFERENCE_ID")) == "" {
		return SpeechStreamStart{}, errors.New("FISH_AUDIO_REFERENCE_ID is not configured")
	}

	sessionID := fmt.Sprintf("fish-%d", time.Now().UnixNano())
	start := SpeechStreamStart{
		SessionID:   sessionID,
		ContentType: "audio/mpeg",
		Provider:    "fish-audio-websocket",
	}
	go a.streamFishSpeech(sessionID, text)
	return start, nil
}

func (a *App) TranscribeAudio(audioBase64 string, contentType string, language string) (ASRReply, error) {
	audioBase64 = strings.TrimSpace(audioBase64)
	contentType = strings.TrimSpace(contentType)
	language = strings.TrimSpace(language)
	if audioBase64 == "" {
		return ASRReply{}, errors.New("audio payload is empty")
	}
	if contentType == "" {
		contentType = "audio/webm"
	}

	payload, err := json.Marshal(map[string]string{
		"audioBase64": audioBase64,
		"contentType": contentType,
		"language":    language,
	})
	if err != nil {
		return ASRReply{}, err
	}

	response, err := a.chatClient.Post(a.agentURL+"/asr", "application/json", bytes.NewReader(payload))
	if err != nil {
		return ASRReply{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return ASRReply{}, fmt.Errorf("agent ASR returned status %d: %s", response.StatusCode, strings.TrimSpace(string(detail)))
	}

	var reply ASRReply
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		return ASRReply{}, err
	}
	reply.Text = strings.TrimSpace(reply.Text)
	reply.Error = strings.TrimSpace(reply.Error)
	if reply.Error != "" {
		return ASRReply{}, errors.New(reply.Error)
	}
	if reply.Text == "" {
		return ASRReply{}, errors.New("ASR returned empty text")
	}
	if strings.TrimSpace(reply.Provider) == "" {
		reply.Provider = "agent-asr"
	}
	return reply, nil
}

func (a *App) StartRealtimeSpeech(text string) (SpeechStreamStart, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return SpeechStreamStart{}, errors.New("speech text cannot be empty")
	}
	if strings.ToLower(strings.TrimSpace(os.Getenv("TTS_PROVIDER"))) != "fish" {
		return SpeechStreamStart{}, errors.New("realtime speech is currently available for TTS_PROVIDER=fish")
	}
	if strings.TrimSpace(os.Getenv("FISH_AUDIO_API_KEY")) == "" {
		return SpeechStreamStart{}, errors.New("FISH_AUDIO_API_KEY is not configured")
	}
	if strings.TrimSpace(os.Getenv("FISH_AUDIO_REFERENCE_ID")) == "" {
		return SpeechStreamStart{}, errors.New("FISH_AUDIO_REFERENCE_ID is not configured")
	}

	history, _ := a.fetchMessages()
	memories, _ := a.fetchMemories()
	sessionID := fmt.Sprintf("fish-realtime-%d", time.Now().UnixNano())
	start := SpeechStreamStart{
		SessionID:   sessionID,
		ContentType: "audio/mpeg",
		Provider:    "fish-audio-realtime",
	}
	go a.streamRealtimeSpeech(sessionID, text, history, memories)
	return start, nil
}

func (a *App) ProbeFishLive() (FishLiveProbeResult, error) {
	startedAt := time.Now()
	result := FishLiveProbeResult{Events: []string{}}
	connection, err := a.openFishSpeechWebSocket()
	if err != nil {
		result.Error = err.Error()
		result.ElapsedMs = time.Since(startedAt).Milliseconds()
		return result, nil
	}
	defer connection.Close()

	startPayload := fishLiveStartPayload()
	result.Events = append(result.Events, "connected")
	if err := connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(startPayload)); err != nil {
		result.Error = err.Error()
		result.ElapsedMs = time.Since(startedAt).Milliseconds()
		return result, nil
	}
	result.Events = append(result.Events, "start-sent")

	probeText := strings.TrimSpace(os.Getenv("FISH_AUDIO_LIVE_PROBE_TEXT"))
	if probeText == "" {
		probeText = "こんにちは。これはFish Audio Live TTSの接続テストです。音声が返ってくるか確認しています。"
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(map[string]any{
		"event": "text",
		"text":  probeText,
	})); err != nil {
		result.Error = err.Error()
		result.ElapsedMs = time.Since(startedAt).Milliseconds()
		return result, nil
	}
	result.Events = append(result.Events, "text-sent")
	if err := connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(map[string]any{"event": "flush"})); err != nil {
		result.Error = err.Error()
		result.ElapsedMs = time.Since(startedAt).Milliseconds()
		return result, nil
	}
	result.Events = append(result.Events, "flush-sent")

	deadline := time.Now().Add(firstFishAudioTimeout())
	for time.Now().Before(deadline) {
		_ = connection.SetReadDeadline(deadline)
		messageType, data, err := connection.ReadMessage()
		if err != nil {
			result.Error = err.Error()
			result.ElapsedMs = time.Since(startedAt).Milliseconds()
			return result, nil
		}
		if messageType != websocket.BinaryMessage {
			result.Events = append(result.Events, "non-binary:"+trimMetricDetail(string(data)))
			continue
		}
		message, err := decodeMsgpackMap(data)
		if err != nil {
			result.Events = append(result.Events, "decode-error:"+err.Error())
			continue
		}
		result.Events = append(result.Events, describeFishMessage(message))
		if errorText := fishMessageError(message); errorText != "" {
			result.Error = errorText
			result.ElapsedMs = time.Since(startedAt).Milliseconds()
			return result, nil
		}
		if audio := fishMessageAudio(message); len(audio) > 0 {
			result.OK = true
			result.AudioSize = len(audio)
			result.ElapsedMs = time.Since(startedAt).Milliseconds()
			_ = connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(map[string]any{"event": "stop"}))
			return result, nil
		}
	}

	result.Error = "probe did not receive audio"
	result.ElapsedMs = time.Since(startedAt).Milliseconds()
	_ = connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(map[string]any{"event": "stop"}))
	return result, nil
}

func (a *App) streamFishSpeech(sessionID string, text string) {
	emit := func(event string, payload SpeechStreamEvent) {
		runtime.EventsEmit(a.ctx, event, payload)
	}
	emit("mochi:speech:start", SpeechStreamEvent{
		SessionID:   sessionID,
		ContentType: "audio/mpeg",
		Provider:    "fish-audio-websocket",
	})

	attempts := fishAudioStreamRetries()
	for attempt := 0; attempt < attempts; attempt++ {
		receivedAudio, err := a.streamFishSpeechOnce(sessionID, text, emit)
		if err == nil {
			emit("mochi:speech:done", SpeechStreamEvent{SessionID: sessionID, Provider: "fish-audio-websocket"})
			return
		}
		if receivedAudio && errors.Is(err, io.EOF) {
			emit("mochi:speech:done", SpeechStreamEvent{SessionID: sessionID, Provider: "fish-audio-websocket"})
			return
		}
		if !receivedAudio && errors.Is(err, io.EOF) && attempt+1 < attempts {
			emit("mochi:speech:metric", SpeechStreamEvent{
				SessionID: sessionID,
				Provider:  "fish-audio-websocket",
				Phase:     "fish-eof-retry",
				Detail:    fmt.Sprintf("retry %d/%d after EOF before audio", attempt+1, attempts),
			})
			time.Sleep(time.Duration(180+attempt*220) * time.Millisecond)
			continue
		}
		emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: err.Error()})
		return
	}
}

func (a *App) streamFishSpeechOnce(sessionID string, text string, emit func(string, SpeechStreamEvent)) (bool, error) {
	connection, err := a.openFishSpeechWebSocket()
	if err != nil {
		return false, err
	}
	defer connection.Close()

	startPayload := fishLiveStartPayload()
	if err := connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(startPayload)); err != nil {
		return false, err
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(map[string]any{
		"event": "text",
		"text":  text,
	})); err != nil {
		return false, err
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(map[string]any{"event": "flush"})); err != nil {
		return false, err
	}
	readTimeout := fishAudioStreamTimeout()
	receivedAudio := false
	for {
		_ = connection.SetReadDeadline(time.Now().Add(readTimeout))
		messageType, data, err := connection.ReadMessage()
		if err != nil {
			return receivedAudio, err
		}
		if messageType != websocket.BinaryMessage {
			emit("mochi:speech:metric", SpeechStreamEvent{
				SessionID: sessionID,
				Provider:  "fish-audio-websocket",
				Phase:     "fish-non-binary",
				Detail:    trimMetricDetail(string(data)),
			})
			continue
		}
		message, err := decodeMsgpackMap(data)
		if err != nil {
			continue
		}
		event, _ := message["event"].(string)
		if audio, ok := message["audio"].([]byte); ok && len(audio) > 0 {
			receivedAudio = true
			_ = connection.SetReadDeadline(time.Now().Add(readTimeout))
			emit("mochi:speech:chunk", SpeechStreamEvent{
				SessionID:   sessionID,
				AudioBase64: base64.StdEncoding.EncodeToString(audio),
				ContentType: "audio/mpeg",
				Provider:    "fish-audio-websocket",
			})
		}
		if event == "finish" || event == "done" || event == "end" {
			return receivedAudio, nil
		}
	}
}

func (a *App) streamRealtimeSpeech(sessionID string, text string, history []Message, memories []string) {
	startedAt := time.Now()
	emit := func(event string, payload SpeechStreamEvent) {
		runtime.EventsEmit(a.ctx, event, payload)
	}
	emitMetric := func(phase string, detail string) {
		elapsed := time.Since(startedAt).Milliseconds()
		runtime.EventsEmit(a.ctx, "mochi:speech:metric", SpeechStreamEvent{
			SessionID: sessionID,
			Provider:  "fish-audio-realtime",
			Phase:     phase,
			ElapsedMs: elapsed,
			Detail:    detail,
		})
		fmt.Printf("[speech:%s] %s %dms %s\n", sessionID, phase, elapsed, detail)
	}
	emit("mochi:speech:start", SpeechStreamEvent{
		SessionID:   sessionID,
		ContentType: "audio/mpeg",
		Provider:    "fish-audio-realtime",
	})
	emitMetric("start", "realtime speech requested")

	segmentCh := make(chan string, 8)
	llmDone := make(chan error, 1)
	go func() {
		pending := ""
		firstDelta := true
		firstSegment := true
		err := a.streamLLMSpeechText(text, history, memories, func(delta string) error {
			if firstDelta {
				firstDelta = false
				emitMetric("llm-first-delta", "")
			}
			pending += delta
			for {
				part, rest, ok := cutSpeechSegment(pending)
				if !ok {
					break
				}
				if firstSegment {
					firstSegment = false
					emitMetric("llm-first-segment", trimMetricDetail(part))
				}
				segmentCh <- part
				pending = rest
			}
			return nil
		})
		if err == nil && strings.TrimSpace(pending) != "" {
			if firstSegment {
				emitMetric("llm-first-segment", trimMetricDetail(pending))
			}
			segmentCh <- strings.TrimSpace(pending)
		}
		close(segmentCh)
		llmDone <- err
	}()

	emitMetric("fish-connect-start", "")
	connection, err := a.openFishSpeechWebSocket()
	if err != nil {
		emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: err.Error()})
		emitMetric("fish-connect-error", err.Error())
		return
	}
	defer connection.Close()
	emitMetric("fish-connected", "")

	startPayload := fishLiveStartPayload()
	if err := connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(startPayload)); err != nil {
		emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: err.Error()})
		emitMetric("fish-start-error", err.Error())
		return
	}
	emitMetric("fish-start-sent", "")

	done := make(chan struct{})
	errCh := make(chan error, 1)
	firstAudio := true
	firstAudioCh := make(chan struct{})
	go a.readFishSpeechStream(connection, sessionID, "fish-audio-realtime", done, errCh, func(phase string, detail string) {
		emitMetric(phase, detail)
	}, func() {
		if firstAudio {
			firstAudio = false
			close(firstAudioCh)
			emitMetric("fish-first-audio", "")
		}
	})

	sendText := func(part string) error {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil
		}
		if err := connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(map[string]any{
			"event": "text",
			"text":  part,
		})); err != nil {
			return err
		}
		emitMetric("fish-text-sent", trimMetricDetail(part))
		return nil
	}
	flushFish := func() error {
		if err := connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(map[string]any{"event": "flush"})); err != nil {
			return err
		}
		emitMetric("fish-flush-sent", "")
		return nil
	}

	if realtimeAckEnabled() {
		ack := realtimeAckText()
		if err := sendText(ack); err != nil {
			emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: err.Error()})
			emitMetric("ack-error", err.Error())
			return
		}
		emitMetric("ack-sent", trimMetricDetail(ack))
	}

	sentSegments := 0
	for part := range segmentCh {
		if err := sendText(part); err != nil {
			emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: err.Error()})
			emitMetric("fish-text-error", err.Error())
			return
		}
		sentSegments++
		if err := flushFish(); err != nil {
			emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: err.Error()})
			emitMetric("fish-flush-error", err.Error())
			return
		}
	}
	if streamErr := <-llmDone; streamErr != nil {
		emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: streamErr.Error()})
		emitMetric("llm-error", streamErr.Error())
		return
	}
	if sentSegments == 0 {
		emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: "LLM returned empty realtime speech text"})
		emitMetric("llm-empty", "")
		return
	}

	select {
	case <-firstAudioCh:
	case <-done:
		emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: "Fish Audio did not return audio chunks"})
		emitMetric("fish-done-without-audio", "")
		return
	case err := <-errCh:
		emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: err.Error()})
		emitMetric("fish-read-error", err.Error())
		return
	}

	select {
	case <-done:
		emit("mochi:speech:done", SpeechStreamEvent{SessionID: sessionID, Provider: "fish-audio-realtime"})
		emitMetric("done", "")
	case err := <-errCh:
		emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: err.Error()})
		emitMetric("fish-read-error", err.Error())
	case <-time.After(fishAudioStreamTimeout()):
		emit("mochi:speech:error", SpeechStreamEvent{SessionID: sessionID, Error: "Fish Audio realtime speech timed out"})
		emitMetric("timeout", "")
	}

	_ = connection.WriteMessage(websocket.BinaryMessage, encodeMsgpack(map[string]any{"event": "stop"}))
	emitMetric("fish-stop-sent", "")
}

func (a *App) readFishSpeechStream(connection *websocket.Conn, sessionID string, provider string, done chan<- struct{}, errCh chan<- error, onMetric func(string, string), onAudio func()) {
	readTimeout := fishAudioStreamTimeout()
	receivedAudio := false
	for {
		timeout := readTimeout
		if receivedAudio {
			timeout = fishAudioEndSilenceTimeout()
		}
		_ = connection.SetReadDeadline(time.Now().Add(timeout))
		messageType, data, err := connection.ReadMessage()
		if err != nil {
			if receivedAudio {
				if onMetric != nil {
					onMetric("fish-audio-silence-done", "")
				}
				close(done)
				return
			}
			select {
			case errCh <- err:
			default:
			}
			return
		}
		if messageType != websocket.BinaryMessage {
			if onMetric != nil {
				onMetric("fish-non-binary", trimMetricDetail(string(data)))
			}
			continue
		}
		message, err := decodeMsgpackMap(data)
		if err != nil {
			continue
		}
		event, _ := message["event"].(string)
		if onMetric != nil {
			onMetric("fish-event", trimMetricDetail(describeFishMessage(message)))
		}
		if errorText := fishMessageError(message); errorText != "" {
			select {
			case errCh <- errors.New(errorText):
			default:
			}
			return
		}
		if audio := fishMessageAudio(message); len(audio) > 0 {
			receivedAudio = true
			if onAudio != nil {
				onAudio()
			}
			runtime.EventsEmit(a.ctx, "mochi:speech:chunk", SpeechStreamEvent{
				SessionID:   sessionID,
				AudioBase64: base64.StdEncoding.EncodeToString(audio),
				ContentType: "audio/mpeg",
				Provider:    provider,
			})
		}
		if event == "finish" || event == "done" || event == "end" {
			close(done)
			return
		}
	}
}

func (a *App) openFishSpeechWebSocket() (*websocket.Conn, error) {
	endpoint := strings.TrimSpace(os.Getenv("FISH_AUDIO_WS_URL"))
	if endpoint == "" {
		endpoint = "wss://api.fish.audio/v1/tts/live"
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+strings.TrimSpace(os.Getenv("FISH_AUDIO_API_KEY")))
	header.Set("model", defaultString(os.Getenv("FISH_AUDIO_MODEL"), "s2-pro"))
	header.Set("Content-Type", "application/msgpack")
	header.Set("Accept", "application/msgpack")

	dialer := websocket.Dialer{
		HandshakeTimeout: cloudTTSTimeout(),
		Proxy:            fishAudioProxy,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}
	connection, _, err := dialer.Dial(endpoint, header)
	return connection, err
}

func fishLiveStartPayload() map[string]any {
	return map[string]any{
		"event": "start",
		"request": map[string]any{
			"text":                         "",
			"format":                       "mp3",
			"reference_id":                 strings.TrimSpace(os.Getenv("FISH_AUDIO_REFERENCE_ID")),
			"normalize":                    true,
			"latency":                      strings.TrimSpace(defaultString(os.Getenv("FISH_AUDIO_STREAM_LATENCY"), "balanced")),
			"chunk_length":                 int(readFloatEnv("FISH_AUDIO_STREAM_CHUNK_LENGTH", 300)),
			"min_chunk_length":             int(readFloatEnv("FISH_AUDIO_STREAM_MIN_CHUNK_LENGTH", 50)),
			"temperature":                  readFloatEnv("FISH_AUDIO_STREAM_TEMPERATURE", 0.7),
			"top_p":                        readFloatEnv("FISH_AUDIO_STREAM_TOP_P", 0.7),
			"repetition_penalty":           readFloatEnv("FISH_AUDIO_STREAM_REPETITION_PENALTY", 1.2),
			"max_new_tokens":               int(readFloatEnv("FISH_AUDIO_STREAM_MAX_NEW_TOKENS", 1024)),
			"condition_on_previous_chunks": envEnabledDefault("FISH_AUDIO_STREAM_CONDITION_ON_PREVIOUS_CHUNKS", true),
		},
	}
}

func (a *App) askGPTSoVITSSpeech(text string) (SpeechReply, error) {
	endpoint := strings.TrimSpace(os.Getenv("GPT_SOVITS_URL"))
	if endpoint == "" {
		endpoint = "http://127.0.0.1:9880/tts"
	}

	style := strings.ToLower(strings.TrimSpace(os.Getenv("GPT_SOVITS_API_STYLE")))
	if style == "legacy" {
		return a.askGPTSoVITSLegacySpeech(endpoint, text)
	}
	return a.askGPTSoVITSV2Speech(endpoint, text)
}

func (a *App) askGPTSoVITSV2Speech(endpoint string, text string) (SpeechReply, error) {
	mediaType := strings.TrimSpace(os.Getenv("GPT_SOVITS_MEDIA_TYPE"))
	if mediaType == "" {
		mediaType = "wav"
	}

	payload := map[string]any{
		"text":               text,
		"text_lang":          defaultString(os.Getenv("GPT_SOVITS_TEXT_LANG"), "zh"),
		"ref_audio_path":     strings.TrimSpace(os.Getenv("GPT_SOVITS_REF_AUDIO_PATH")),
		"prompt_text":        strings.TrimSpace(os.Getenv("GPT_SOVITS_PROMPT_TEXT")),
		"prompt_lang":        defaultString(os.Getenv("GPT_SOVITS_PROMPT_LANG"), "zh"),
		"text_split_method":  defaultString(os.Getenv("GPT_SOVITS_TEXT_SPLIT_METHOD"), "cut5"),
		"batch_size":         int(readFloatEnv("GPT_SOVITS_BATCH_SIZE", 1)),
		"batch_threshold":    readFloatEnv("GPT_SOVITS_BATCH_THRESHOLD", 0.75),
		"split_bucket":       envEnabledDefault("GPT_SOVITS_SPLIT_BUCKET", true),
		"speed_factor":       readFloatEnv("GPT_SOVITS_SPEED_FACTOR", 1.0),
		"fragment_interval":  readFloatEnv("GPT_SOVITS_FRAGMENT_INTERVAL", 0.3),
		"streaming_mode":     boolOrIntEnv("GPT_SOVITS_STREAMING_MODE", false),
		"parallel_infer":     envEnabledDefault("GPT_SOVITS_PARALLEL_INFER", true),
		"repetition_penalty": readFloatEnv("GPT_SOVITS_REPETITION_PENALTY", 1.35),
		"sample_steps":       int(readFloatEnv("GPT_SOVITS_SAMPLE_STEPS", 32)),
		"super_sampling":     envEnabled("GPT_SOVITS_SUPER_SAMPLING"),
		"overlap_length":     int(readFloatEnv("GPT_SOVITS_OVERLAP_LENGTH", 2)),
		"min_chunk_length":   int(readFloatEnv("GPT_SOVITS_MIN_CHUNK_LENGTH", 16)),
		"media_type":         mediaType,
	}
	if auxRefAudioPaths := csvEnv("GPT_SOVITS_AUX_REF_AUDIO_PATHS"); len(auxRefAudioPaths) > 0 {
		payload["aux_ref_audio_paths"] = auxRefAudioPaths
	}
	if seed := strings.TrimSpace(os.Getenv("GPT_SOVITS_SEED")); seed != "" {
		payload["seed"] = int(readFloatEnv("GPT_SOVITS_SEED", -1))
	}
	if topK := strings.TrimSpace(os.Getenv("GPT_SOVITS_TOP_K")); topK != "" {
		payload["top_k"] = int(readFloatEnv("GPT_SOVITS_TOP_K", 5))
	}
	if topP := strings.TrimSpace(os.Getenv("GPT_SOVITS_TOP_P")); topP != "" {
		payload["top_p"] = readFloatEnv("GPT_SOVITS_TOP_P", 1.0)
	}
	if temperature := strings.TrimSpace(os.Getenv("GPT_SOVITS_TEMPERATURE")); temperature != "" {
		payload["temperature"] = readFloatEnv("GPT_SOVITS_TEMPERATURE", 1.0)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return SpeechReply{}, err
	}

	request, err := http.NewRequestWithContext(a.ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return SpeechReply{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", ttsAcceptHeader(mediaType))

	return a.readTTSHTTPResponse("GPT-SoVITS", "gpt-sovits-v2", request, mediaType)
}

func (a *App) askGPTSoVITSLegacySpeech(endpoint string, text string) (SpeechReply, error) {
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return SpeechReply{}, err
	}
	query := requestURL.Query()
	query.Set("text", text)
	query.Set("text_language", defaultString(os.Getenv("GPT_SOVITS_TEXT_LANG"), "zh"))
	query.Set("refer_wav_path", strings.TrimSpace(os.Getenv("GPT_SOVITS_REF_AUDIO_PATH")))
	query.Set("prompt_text", strings.TrimSpace(os.Getenv("GPT_SOVITS_PROMPT_TEXT")))
	query.Set("prompt_language", defaultString(os.Getenv("GPT_SOVITS_PROMPT_LANG"), "zh"))
	requestURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(a.ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return SpeechReply{}, err
	}

	return a.readTTSHTTPResponse("GPT-SoVITS", "gpt-sovits-legacy", request, "wav")
}

func (a *App) askCartesiaSpeech(text string) (SpeechReply, error) {
	apiKey := strings.TrimSpace(os.Getenv("CARTESIA_API_KEY"))
	voiceID := strings.TrimSpace(os.Getenv("CARTESIA_VOICE_ID"))
	if apiKey == "" {
		return SpeechReply{}, errors.New("CARTESIA_API_KEY is not configured")
	}
	if voiceID == "" {
		return SpeechReply{}, errors.New("CARTESIA_VOICE_ID is not configured")
	}

	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("CARTESIA_BASE_URL")), "/")
	if endpoint == "" {
		endpoint = "https://api.cartesia.ai"
	}
	model := strings.TrimSpace(os.Getenv("CARTESIA_MODEL"))
	if model == "" {
		model = "sonic-3.5"
	}
	language := strings.TrimSpace(os.Getenv("CARTESIA_LANGUAGE"))
	if language == "" {
		language = "zh"
	}
	emotion := strings.TrimSpace(os.Getenv("CARTESIA_EMOTION"))
	if emotion == "" {
		emotion = "positivity:high"
	}

	payload, err := json.Marshal(map[string]any{
		"model_id":   model,
		"transcript": text,
		"voice": map[string]any{
			"mode": "id",
			"id":   voiceID,
			"__experimental_controls": map[string]any{
				"speed":   readFloatEnv("CARTESIA_SPEED", 0),
				"emotion": []string{emotion},
			},
		},
		"output_format": map[string]any{
			"container":   "mp3",
			"sample_rate": 44100,
			"bit_rate":    128000,
		},
		"language": language,
	})
	if err != nil {
		return SpeechReply{}, err
	}

	request, err := http.NewRequestWithContext(a.ctx, http.MethodPost, endpoint+"/tts/bytes", bytes.NewReader(payload))
	if err != nil {
		return SpeechReply{}, err
	}
	request.Header.Set("X-API-Key", apiKey)
	request.Header.Set("Cartesia-Version", "2024-11-13")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "audio/mpeg")

	return a.readTTSHTTPResponse("Cartesia", "cartesia-sonic-3.5", request, "mp3")
}

func (a *App) askElevenLabsSpeech(text string) (SpeechReply, error) {
	apiKey := strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY"))
	voiceID := strings.TrimSpace(os.Getenv("ELEVENLABS_VOICE_ID"))
	if apiKey == "" {
		return SpeechReply{}, errors.New("ELEVENLABS_API_KEY is not configured")
	}
	if voiceID == "" {
		return SpeechReply{}, errors.New("ELEVENLABS_VOICE_ID is not configured")
	}

	model := strings.TrimSpace(os.Getenv("ELEVENLABS_MODEL"))
	if model == "" {
		model = "eleven_flash_v2_5"
	}
	outputFormat := strings.TrimSpace(os.Getenv("ELEVENLABS_OUTPUT_FORMAT"))
	if outputFormat == "" {
		outputFormat = "mp3_44100_128"
	}
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("ELEVENLABS_BASE_URL")), "/")
	if endpoint == "" {
		endpoint = "https://api.elevenlabs.io"
	}
	endpoint = fmt.Sprintf("%s/v1/text-to-speech/%s/stream?output_format=%s", endpoint, url.PathEscape(voiceID), url.QueryEscape(outputFormat))

	payload, err := json.Marshal(map[string]any{
		"text":     text,
		"model_id": model,
		"voice_settings": map[string]any{
			"stability":         readFloatEnv("ELEVENLABS_STABILITY", 0.45),
			"similarity_boost":  readFloatEnv("ELEVENLABS_SIMILARITY_BOOST", 0.75),
			"style":             readFloatEnv("ELEVENLABS_STYLE", 0.0),
			"use_speaker_boost": envEnabledDefault("ELEVENLABS_SPEAKER_BOOST", true),
		},
	})
	if err != nil {
		return SpeechReply{}, err
	}

	request, err := http.NewRequestWithContext(a.ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return SpeechReply{}, err
	}
	request.Header.Set("xi-api-key", apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "audio/mpeg")

	return a.readTTSHTTPResponse("ElevenLabs", "elevenlabs-flash", request, "mp3")
}

func (a *App) streamLLMSpeechText(text string, history []Message, memories []string, onDelta func(string) error) error {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	if provider == "" {
		provider = "deepseek"
	}
	if provider == "openai" {
		return errors.New("realtime speech currently supports OpenAI-compatible chat providers; set LLM_PROVIDER=deepseek/openrouter/ollama")
	}

	apiKey := strings.TrimSpace(os.Getenv(strings.ToUpper(provider) + "_API_KEY"))
	model := strings.TrimSpace(os.Getenv(strings.ToUpper(provider) + "_MODEL"))
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(strings.ToUpper(provider)+"_BASE_URL")), "/")
	switch provider {
	case "deepseek":
		if model == "" {
			model = "deepseek-v4-flash"
		}
		if baseURL == "" {
			baseURL = "https://api.deepseek.com"
		}
	case "openrouter":
		if model == "" {
			model = "deepseek/deepseek-chat"
		}
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
	case "ollama":
		if model == "" {
			model = "qwen2.5:7b"
		}
		if baseURL == "" {
			baseURL = "http://127.0.0.1:11434/v1"
		}
	default:
		return fmt.Errorf("realtime speech does not support LLM_PROVIDER=%s yet", provider)
	}
	if provider != "ollama" && apiKey == "" {
		return fmt.Errorf("%s_API_KEY is not configured", strings.ToUpper(provider))
	}

	messages := []map[string]string{
		{"role": "system", "content": buildRealtimeSpeechPrompt()},
		{"role": "user", "content": buildRealtimeSpeechContext(text, history, memories)},
	}
	payload, err := json.Marshal(map[string]any{
		"model":       model,
		"messages":    messages,
		"stream":      true,
		"temperature": 0.6,
		"max_tokens":  80,
	})
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(a.ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	response, err := a.chatClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(response.Body)
		detail := strings.TrimSpace(string(body))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return fmt.Errorf("%s realtime speech LLM failed: HTTP %d %s", provider, response.StatusCode, detail)
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	sawDelta := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		delta := extractChatStreamDelta(data)
		if delta == "" {
			continue
		}
		sawDelta = true
		if err := onDelta(delta); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !sawDelta {
		fallback, err := a.completeLLMSpeechText(provider, apiKey, model, baseURL, messages)
		if err != nil {
			return fmt.Errorf("LLM returned empty stream; non-stream fallback failed: %w", err)
		}
		if strings.TrimSpace(fallback) == "" {
			return errors.New("LLM returned empty stream")
		}
		return onDelta(fallback)
	}
	return nil
}

func (a *App) completeLLMSpeechText(provider string, apiKey string, model string, baseURL string, messages []map[string]string) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"model":       model,
		"messages":    messages,
		"stream":      false,
		"temperature": 0.6,
		"max_tokens":  80,
	})
	if err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(a.ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	response, err := a.chatClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := strings.TrimSpace(string(body))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return "", fmt.Errorf("%s realtime speech LLM fallback failed: HTTP %d %s", provider, response.StatusCode, detail)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("non-stream fallback returned no choices")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func buildRealtimeSpeechPrompt() string {
	name := envFirst("YUYU_DESKTOP_PET_NAME", "MOCHI_DESKTOP_PET_NAME")
	if name == "" {
		name = "Yuyu"
	}
	language := strings.ToLower(envFirst("YUYU_REPLY_LANGUAGE", "MOCHI_REPLY_LANGUAGE"))
	if language == "" {
		language = "ja"
	}
	if strings.HasPrefix(language, "ja") || strings.Contains(language, "ja") {
		return fmt.Sprintf(`You are %s, a warm anime-style desktop companion.
Write ONLY the spoken reply for TTS, in natural Japanese.
Even if the user writes in Chinese, translate the meaning internally and answer only in Japanese.
Do not include Chinese words unless the user explicitly asks for a translation or a quoted phrase.
No JSON, no markdown, no labels, no quotes.
Keep it conversational and concise, usually 1-3 short sentences.
Start directly with the answer.`, name)
	}
	return fmt.Sprintf(`You are %s, a warm desktop companion.
Write ONLY the spoken reply for TTS, in natural Chinese.
No JSON, no markdown, no labels, no quotes.
Keep it conversational and concise, usually 1-3 short sentences.
Start directly with the answer.`, name)
}

func buildRealtimeSpeechContext(text string, history []Message, memories []string) string {
	var builder strings.Builder
	if len(memories) > 0 {
		builder.WriteString("Memories:\n")
		for _, memory := range memories {
			memory = strings.TrimSpace(memory)
			if memory != "" {
				builder.WriteString("- ")
				builder.WriteString(memory)
				builder.WriteString("\n")
			}
		}
	}
	if len(history) > 0 {
		builder.WriteString("Recent chat:\n")
		start := len(history) - 6
		if start < 0 {
			start = 0
		}
		for _, message := range history[start:] {
			content := strings.TrimSpace(message.Content)
			if content == "" {
				continue
			}
			builder.WriteString(message.Role)
			builder.WriteString(": ")
			builder.WriteString(content)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("User just said:\n")
	builder.WriteString(text)
	return builder.String()
}

func extractChatStreamDelta(data string) string {
	var event struct {
		Choices []struct {
			Delta struct {
				Content any `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return ""
	}
	if len(event.Choices) == 0 {
		return ""
	}
	switch content := event.Choices[0].Delta.Content.(type) {
	case string:
		return content
	case []any:
		var builder strings.Builder
		for _, item := range content {
			if part, ok := item.(map[string]any); ok {
				if text, ok := part["text"].(string); ok {
					builder.WriteString(text)
				}
			}
		}
		return builder.String()
	default:
		return ""
	}
}

func cutSpeechSegment(text string) (string, string, bool) {
	text = strings.TrimLeft(text, " \t\r\n")
	if text == "" {
		return "", "", false
	}
	for index, char := range text {
		if strings.ContainsRune("。！？!?…\n、，,；;", char) {
			end := index + len(string(char))
			return strings.TrimSpace(text[:end]), text[end:], true
		}
	}
	maxChars := int(readFloatEnv("REALTIME_SPEECH_MAX_CHARS", 14))
	if maxChars < 8 {
		maxChars = 8
	}
	if len([]rune(text)) >= maxChars {
		runes := []rune(text)
		return strings.TrimSpace(string(runes[:maxChars])), string(runes[maxChars:]), true
	}
	return "", text, false
}

func realtimeAckEnabled() bool {
	return envEnabledDefault("REALTIME_SPEECH_ACK_ENABLED", false)
}

func realtimeAckText() string {
	value := strings.TrimSpace(os.Getenv("REALTIME_SPEECH_ACK_TEXT"))
	if value != "" {
		return value
	}
	language := strings.ToLower(envFirst("YUYU_REPLY_LANGUAGE", "MOCHI_REPLY_LANGUAGE"))
	if strings.Contains(language, "ja") {
		return "うん、ちょっと待ってね。"
	}
	return "嗯，我想一下。"
}

func trimMetricDetail(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 36 {
		return value
	}
	return string(runes[:36]) + "..."
}

func describeFishMessage(message map[string]any) string {
	event, _ := message["event"].(string)
	keys := make([]string, 0, len(message))
	for key := range message {
		keys = append(keys, key)
	}
	return fmt.Sprintf("event=%s keys=%s", event, strings.Join(keys, ","))
}

func fishMessageError(message map[string]any) string {
	for _, key := range []string{"error", "message", "detail"} {
		if value, ok := message[key].(string); ok && strings.TrimSpace(value) != "" {
			event, _ := message["event"].(string)
			if strings.EqualFold(event, "error") || key == "error" {
				return strings.TrimSpace(value)
			}
		}
	}
	if event, _ := message["event"].(string); strings.EqualFold(event, "error") {
		return describeFishMessage(message)
	}
	return ""
}

func fishMessageAudio(message map[string]any) []byte {
	for _, key := range []string{"audio", "data", "chunk", "bytes"} {
		value, ok := message[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case []byte:
			return typed
		case string:
			if decoded, err := base64.StdEncoding.DecodeString(typed); err == nil && len(decoded) > 0 {
				return decoded
			}
			return []byte(typed)
		}
	}
	return nil
}

func (a *App) askFishSpeechDirect(text string) (SpeechReply, error) {
	apiKey := strings.TrimSpace(os.Getenv("FISH_AUDIO_API_KEY"))
	referenceID := strings.TrimSpace(os.Getenv("FISH_AUDIO_REFERENCE_ID"))
	if apiKey == "" {
		return SpeechReply{}, errors.New("FISH_AUDIO_API_KEY is not configured")
	}
	if referenceID == "" {
		return SpeechReply{}, errors.New("FISH_AUDIO_REFERENCE_ID is not configured")
	}

	endpoint := strings.TrimSpace(os.Getenv("FISH_AUDIO_TTS_URL"))
	if endpoint == "" {
		endpoint = "https://api.fish.audio/v1/tts"
	}
	model := strings.TrimSpace(os.Getenv("FISH_AUDIO_MODEL"))
	if model == "" {
		model = "s2-pro"
	}

	payload, err := json.Marshal(map[string]string{
		"text":         text,
		"reference_id": referenceID,
		"format":       "mp3",
	})
	if err != nil {
		return SpeechReply{}, err
	}

	request, err := http.NewRequestWithContext(a.ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return SpeechReply{}, err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("model", model)

	client := &http.Client{
		Timeout: cloudTTSTimeout(),
		Transport: &http.Transport{
			Proxy:           fishAudioProxy,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	return readTTSHTTPResponseWithClient(client, "Fish Audio", "fish-audio-go-direct", request, "mp3")
}

func (a *App) readTTSHTTPResponse(service string, provider string, request *http.Request, fallbackMediaType string) (SpeechReply, error) {
	return readTTSHTTPResponseWithClient(&http.Client{Timeout: cloudTTSTimeout()}, service, provider, request, fallbackMediaType)
}

func readTTSHTTPResponseWithClient(client *http.Client, service string, provider string, request *http.Request, fallbackMediaType string) (SpeechReply, error) {
	response, err := client.Do(request)
	if err != nil {
		return SpeechReply{}, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return SpeechReply{}, err
	}
	if response.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(body))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return SpeechReply{}, fmt.Errorf("%s TTS failed: HTTP %d %s", service, response.StatusCode, detail)
	}
	if len(body) == 0 {
		return SpeechReply{}, fmt.Errorf("%s TTS returned empty audio", service)
	}

	contentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	if contentType == "" || strings.HasPrefix(contentType, "application/octet-stream") {
		contentType = ttsContentType(fallbackMediaType)
	}
	return SpeechReply{
		AudioBase64: base64.StdEncoding.EncodeToString(body),
		ContentType: contentType,
		Provider:    provider,
	}, nil
}

func ttsAcceptHeader(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "mp3", "mpeg":
		return "audio/mpeg"
	case "ogg":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	default:
		return "audio/wav"
	}
}

func ttsContentType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "mp3", "mpeg":
		return "audio/mpeg"
	case "ogg":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	default:
		return "audio/wav"
	}
}

func fishAudioProxy(request *http.Request) (*url.URL, error) {
	value := strings.TrimSpace(os.Getenv("FISH_AUDIO_PROXY"))
	switch strings.ToLower(value) {
	case "direct", "none", "off", "false", "0":
		return nil, nil
	case "":
		return http.ProxyFromEnvironment(request)
	default:
		return url.Parse(value)
	}
}

func cloudTTSTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("TTS_TIMEOUT_SECONDS"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("FISH_AUDIO_TIMEOUT_SECONDS"))
	}
	if value == "" {
		return 12 * time.Second
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds <= 0 {
		return 12 * time.Second
	}
	return time.Duration(seconds * float64(time.Second))
}

func fishAudioStreamTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("FISH_AUDIO_STREAM_TIMEOUT_SECONDS"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("TTS_TIMEOUT_SECONDS"))
	}
	if value == "" {
		return 30 * time.Second
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(seconds * float64(time.Second))
}

func fishAudioStreamRetries() int {
	value := strings.TrimSpace(os.Getenv("FISH_AUDIO_STREAM_RETRIES"))
	if value == "" {
		return 3
	}
	retries, err := strconv.Atoi(value)
	if err != nil || retries < 1 {
		return 3
	}
	if retries > 5 {
		return 5
	}
	return retries
}

func firstFishAudioTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("FISH_AUDIO_FIRST_AUDIO_TIMEOUT_SECONDS"))
	if value == "" {
		return 6 * time.Second
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds <= 0 {
		return 6 * time.Second
	}
	return time.Duration(seconds * float64(time.Second))
}

func fishAudioEndSilenceTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("FISH_AUDIO_END_SILENCE_SECONDS"))
	if value == "" {
		return 2 * time.Second
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds <= 0 {
		return 2 * time.Second
	}
	return time.Duration(seconds * float64(time.Second))
}

func readFloatEnv(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func envEnabled(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envEnabledDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envEnabledDefaultCompat(primary string, legacy string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(primary))
	if value == "" {
		return envEnabledDefault(legacy, fallback)
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func intEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func intValue(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return fallback
		}
		return int(parsed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(stringValue(item))
			if text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func boolOrIntEnv(key string, fallback bool) any {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func csvEnv(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func encodeMsgpack(value any) []byte {
	var out []byte
	appendMsgpackValue(&out, value)
	return out
}

func appendMsgpackValue(out *[]byte, value any) {
	switch typed := value.(type) {
	case map[string]any:
		appendMsgpackMap(out, typed)
	case string:
		appendMsgpackString(out, typed)
	case bool:
		if typed {
			*out = append(*out, 0xc3)
		} else {
			*out = append(*out, 0xc2)
		}
	case int:
		appendMsgpackInt(out, int64(typed))
	case int64:
		appendMsgpackInt(out, typed)
	case float64:
		appendMsgpackFloat64(out, typed)
	case []string:
		appendMsgpackArrayHeader(out, len(typed))
		for _, item := range typed {
			appendMsgpackString(out, item)
		}
	default:
		*out = append(*out, 0xc0)
	}
}

func appendMsgpackMap(out *[]byte, value map[string]any) {
	if len(value) < 16 {
		*out = append(*out, byte(0x80|len(value)))
	} else {
		*out = append(*out, 0xde, byte(len(value)>>8), byte(len(value)))
	}
	for key, item := range value {
		appendMsgpackString(out, key)
		appendMsgpackValue(out, item)
	}
}

func appendMsgpackArrayHeader(out *[]byte, length int) {
	if length < 16 {
		*out = append(*out, byte(0x90|length))
		return
	}
	*out = append(*out, 0xdc, byte(length>>8), byte(length))
}

func appendMsgpackString(out *[]byte, value string) {
	bytes := []byte(value)
	length := len(bytes)
	switch {
	case length < 32:
		*out = append(*out, byte(0xa0|length))
	case length <= 0xff:
		*out = append(*out, 0xd9, byte(length))
	default:
		*out = append(*out, 0xda, byte(length>>8), byte(length))
	}
	*out = append(*out, bytes...)
}

func appendMsgpackInt(out *[]byte, value int64) {
	if value >= 0 && value <= 0x7f {
		*out = append(*out, byte(value))
		return
	}
	if value >= 0 && value <= 0xff {
		*out = append(*out, 0xcc, byte(value))
		return
	}
	*out = append(*out, 0xcd, byte(value>>8), byte(value))
}

func appendMsgpackFloat64(out *[]byte, value float64) {
	var bytes [8]byte
	binary.BigEndian.PutUint64(bytes[:], math.Float64bits(value))
	*out = append(*out, 0xcb)
	*out = append(*out, bytes[:]...)
}

func decodeMsgpackMap(data []byte) (map[string]any, error) {
	decoder := msgpackDecoder{data: data}
	value, err := decoder.readValue()
	if err != nil {
		return nil, err
	}
	message, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("message is not a map")
	}
	return message, nil
}

type msgpackDecoder struct {
	data []byte
	pos  int
}

func (d *msgpackDecoder) readValue() (any, error) {
	if d.pos >= len(d.data) {
		return nil, io.ErrUnexpectedEOF
	}
	code := d.readByte()
	switch {
	case code <= 0x7f:
		return int(code), nil
	case code >= 0xa0 && code <= 0xbf:
		return d.readString(int(code & 0x1f))
	case code >= 0x80 && code <= 0x8f:
		return d.readMap(int(code & 0x0f))
	case code >= 0x90 && code <= 0x9f:
		return d.readArray(int(code & 0x0f))
	}

	switch code {
	case 0xc0:
		return nil, nil
	case 0xc2:
		return false, nil
	case 0xc3:
		return true, nil
	case 0xc4:
		length, err := d.readUint8()
		if err != nil {
			return nil, err
		}
		return d.readBytes(int(length))
	case 0xc5:
		length, err := d.readUint16()
		if err != nil {
			return nil, err
		}
		return d.readBytes(int(length))
	case 0xcc:
		value, err := d.readUint8()
		return int(value), err
	case 0xcd:
		value, err := d.readUint16()
		return int(value), err
	case 0xca:
		value, err := d.readUint32()
		if err != nil {
			return nil, err
		}
		return math.Float32frombits(value), nil
	case 0xcb:
		value, err := d.readUint64()
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(value), nil
	case 0xd9:
		length, err := d.readUint8()
		if err != nil {
			return nil, err
		}
		return d.readString(int(length))
	case 0xda:
		length, err := d.readUint16()
		if err != nil {
			return nil, err
		}
		return d.readString(int(length))
	case 0xde:
		length, err := d.readUint16()
		if err != nil {
			return nil, err
		}
		return d.readMap(int(length))
	default:
		return nil, fmt.Errorf("unsupported msgpack code: 0x%x", code)
	}
}

func (d *msgpackDecoder) readMap(length int) (map[string]any, error) {
	result := make(map[string]any, length)
	for i := 0; i < length; i++ {
		keyValue, err := d.readValue()
		if err != nil {
			return nil, err
		}
		key, ok := keyValue.(string)
		if !ok {
			return nil, errors.New("msgpack map key is not a string")
		}
		value, err := d.readValue()
		if err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

func (d *msgpackDecoder) readArray(length int) ([]any, error) {
	result := make([]any, 0, length)
	for i := 0; i < length; i++ {
		value, err := d.readValue()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (d *msgpackDecoder) readByte() byte {
	value := d.data[d.pos]
	d.pos++
	return value
}

func (d *msgpackDecoder) readUint8() (uint8, error) {
	if d.pos >= len(d.data) {
		return 0, io.ErrUnexpectedEOF
	}
	return d.readByte(), nil
}

func (d *msgpackDecoder) readUint16() (uint16, error) {
	if d.pos+2 > len(d.data) {
		return 0, io.ErrUnexpectedEOF
	}
	value := uint16(d.data[d.pos])<<8 | uint16(d.data[d.pos+1])
	d.pos += 2
	return value, nil
}

func (d *msgpackDecoder) readUint32() (uint32, error) {
	if d.pos+4 > len(d.data) {
		return 0, io.ErrUnexpectedEOF
	}
	value := binary.BigEndian.Uint32(d.data[d.pos : d.pos+4])
	d.pos += 4
	return value, nil
}

func (d *msgpackDecoder) readUint64() (uint64, error) {
	if d.pos+8 > len(d.data) {
		return 0, io.ErrUnexpectedEOF
	}
	value := binary.BigEndian.Uint64(d.data[d.pos : d.pos+8])
	d.pos += 8
	return value, nil
}

func (d *msgpackDecoder) readString(length int) (string, error) {
	bytes, err := d.readBytes(length)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (d *msgpackDecoder) readBytes(length int) ([]byte, error) {
	if d.pos+length > len(d.data) {
		return nil, io.ErrUnexpectedEOF
	}
	value := d.data[d.pos : d.pos+length]
	d.pos += length
	return value, nil
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

	agentContent := content
	pluginContexts, pluginContextErrs := a.collectPluginContexts("chat", content)
	if len(pluginContexts) > 0 {
		agentContent = buildPluginAwareUserPrompt(content, pluginContexts)
	}

	agentReply, err := a.askAgent(agentContent, history, memories)
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
			a.providerErr = "Agent did not return speechText. Please stop the old python agent/main.py process and restart Yuyu-Mind."
		}
	}
	if len(pluginContextErrs) > 0 {
		if strings.TrimSpace(a.providerErr) != "" {
			a.providerErr += " | "
		}
		a.providerErr += "plugin context failed: " + strings.Join(pluginContextErrs, "; ")
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

func (a *App) ObserveScreen(prompt string) (ChatReply, error) {
	if a.db == nil {
		return ChatReply{}, errors.New("database is not ready")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "看一下当前屏幕，告诉我你注意到了什么。"
	}

	observation, err := a.invokeAgentPlugin("screen", "observe", map[string]any{
		"prompt":       prompt,
		"includeImage": false,
	})
	if err != nil {
		return ChatReply{}, err
	}
	if !observation.OK {
		if strings.TrimSpace(observation.Error) != "" {
			return ChatReply{}, errors.New(observation.Error)
		}
		return ChatReply{}, errors.New("screen plugin failed")
	}

	userMessage, err := a.saveMessage("user", prompt, "focused")
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

	screenContext := buildPluginContextPrompt(
		"The user explicitly asked you to look at the current screen.",
		prompt,
		[]PluginContextCandidate{{
			Plugin:       "screen",
			Action:       "observe",
			Mode:         "chat",
			Prompt:       prompt,
			Result:       observation,
			ProviderName: "屏幕观察",
		}},
	)
	agentReply, err := a.askAgentWithMode(screenContext, history, memories, "chat")
	if err != nil {
		summary := strings.TrimSpace(observation.Summary)
		if summary == "" {
			summary = "屏幕插件已经返回结果，但没有生成摘要。"
		}
		agentReply = agentResponse{
			Text:       fmt.Sprintf("我看到了当前屏幕：%s", summary),
			SpeechText: "画面は見えたよ。内容は画面の要約として確認できているみたい。",
			Emotion:    "thinking",
		}
		a.agentStatus = "offline"
		a.provider = "go-fallback"
		a.providerErr = "screen plugin succeeded; agent reply failed: " + err.Error()
	} else {
		a.agentStatus = "online"
		a.provider = strings.TrimSpace(agentReply.Provider)
		a.providerErr = strings.TrimSpace(agentReply.ProviderError)
		if a.provider == "" {
			a.provider = "agent"
		}
		if strings.TrimSpace(agentReply.Text) == "" {
			agentReply.Text = fmt.Sprintf("我看到了当前屏幕：%s", strings.TrimSpace(observation.Summary))
			agentReply.Emotion = "thinking"
		}
	}

	reply, err := a.saveMessage("assistant", agentReply.Text, normalizedEmotion(agentReply.Emotion))
	if err != nil {
		return ChatReply{}, err
	}
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

func (a *App) invokeAgentPlugin(plugin string, action string, payload map[string]any) (PluginInvokeResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return PluginInvokeResult{}, err
	}
	response, err := a.chatClient.Post(
		fmt.Sprintf("%s/plugins/%s/%s", a.agentURL, url.PathEscape(plugin), url.PathEscape(action)),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return PluginInvokeResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return PluginInvokeResult{}, fmt.Errorf("agent plugin returned status %d: %s", response.StatusCode, strings.TrimSpace(string(detail)))
	}
	var result PluginInvokeResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return PluginInvokeResult{}, err
	}
	return result, nil
}

func (a *App) collectPluginContexts(mode string, message string) ([]PluginContextCandidate, []string) {
	if !envEnabledDefaultCompat("YUYU_PLUGIN_CONTEXT_ENABLED", "MOCHI_PLUGIN_CONTEXT_ENABLED", true) {
		return nil, nil
	}
	if mode == "chat" && !envEnabledDefaultCompat("YUYU_PLUGIN_CONTEXT_CHAT_ENABLED", "MOCHI_PLUGIN_CONTEXT_CHAT_ENABLED", envEnabledDefault("MOCHI_SCREEN_CONTEXT_AUTO", true)) {
		return nil, nil
	}
	if mode == "proactive" && !envEnabledDefaultCompat("YUYU_PLUGIN_CONTEXT_PROACTIVE_ENABLED", "MOCHI_PLUGIN_CONTEXT_PROACTIVE_ENABLED", envEnabledDefault("MOCHI_SCREEN_PROACTIVE_ENABLED", true)) {
		return nil, nil
	}
	reply, err := a.ListPlugins()
	if err != nil {
		return nil, []string{err.Error()}
	}
	candidates := []PluginContextCandidate{}
	errs := []string{}
	for _, plugin := range reply.Plugins {
		candidate, ok, err := a.maybeInvokePluginContext(plugin, mode, message)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", plugin.Name, err.Error()))
			continue
		}
		if ok {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, errs
}

func (a *App) maybeInvokePluginContext(plugin PluginInfo, mode string, message string) (PluginContextCandidate, bool, error) {
	if !plugin.Enabled {
		return PluginContextCandidate{}, false, nil
	}
	contextConfig, ok := plugin.Context["modes"].(map[string]any)
	if !ok {
		return PluginContextCandidate{}, false, nil
	}
	rawMode, ok := contextConfig[mode].(map[string]any)
	if !ok {
		return PluginContextCandidate{}, false, nil
	}
	if enabled, ok := rawMode["enabled"].(bool); ok && !enabled {
		return PluginContextCandidate{}, false, nil
	}
	action := strings.TrimSpace(stringValue(rawMode["action"]))
	if action == "" {
		return PluginContextCandidate{}, false, nil
	}
	if !pluginHasLoadedAction(plugin, action) {
		return PluginContextCandidate{}, false, nil
	}
	if mode == "chat" && !pluginContextMatchesMessage(rawMode, message) {
		return PluginContextCandidate{}, false, nil
	}
	if mode == "proactive" && !pluginContextAllowsProactive(rawMode) {
		return PluginContextCandidate{}, false, nil
	}

	result, err := a.invokeAgentPlugin(plugin.Name, action, map[string]any{
		"prompt":       pluginContextPrompt(rawMode, message),
		"includeImage": false,
	})
	if err != nil {
		return PluginContextCandidate{}, false, err
	}
	if !result.OK {
		if strings.TrimSpace(result.Error) != "" {
			return PluginContextCandidate{}, false, errors.New(result.Error)
		}
		return PluginContextCandidate{}, false, errors.New("plugin returned unsuccessful context")
	}
	return PluginContextCandidate{
		Plugin:       plugin.Name,
		Action:       action,
		Mode:         mode,
		Priority:     intValue(rawMode["priority"], 50),
		Prompt:       pluginContextPrompt(rawMode, message),
		Result:       result,
		ProviderName: plugin.DisplayName,
	}, true, nil
}

func pluginHasLoadedAction(plugin PluginInfo, action string) bool {
	for _, loaded := range plugin.LoadedActions {
		if loaded == action {
			return true
		}
	}
	return false
}

func pluginContextMatchesMessage(config map[string]any, message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return false
	}
	for _, marker := range stringSlice(config["triggers"]) {
		if marker != "" && strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	demonstratives := stringSlice(config["demonstratives"])
	targets := stringSlice(config["targets"])
	for _, demonstrative := range demonstratives {
		if demonstrative == "" || !strings.Contains(text, strings.ToLower(demonstrative)) {
			continue
		}
		for _, target := range targets {
			if target != "" && strings.Contains(text, strings.ToLower(target)) {
				return true
			}
		}
	}
	return false
}

func pluginContextAllowsProactive(config map[string]any) bool {
	chance := intValue(config["chancePercent"], 0)
	if chance <= 0 {
		return false
	}
	if chance >= 100 {
		return true
	}
	return int(time.Now().UnixNano()%100) < chance
}

func pluginContextPrompt(config map[string]any, fallback string) string {
	prompt := strings.TrimSpace(stringValue(config["prompt"]))
	if prompt != "" {
		return prompt
	}
	return fallback
}

func buildPluginAwareUserPrompt(userMessage string, contexts []PluginContextCandidate) string {
	return buildPluginContextPrompt(
		"The user message appears to need live plugin context. Use the context below, then answer the user's original message directly.",
		userMessage,
		contexts,
	)
}

func buildPluginContextPrompt(instruction string, userMessage string, contexts []PluginContextCandidate) string {
	var details strings.Builder
	for _, context := range contexts {
		metadata, _ := json.Marshal(context.Result.Metadata)
		vision, _ := json.Marshal(context.Result.Vision)
		summary := strings.TrimSpace(context.Result.Summary)
		if summary == "" {
			summary = "The plugin returned no summary."
		}
		details.WriteString(fmt.Sprintf(`Provider: %s
Plugin: %s.%s
Summary:
%s
Metadata JSON:
%s
Details JSON:
%s

`, context.ProviderName, context.Plugin, context.Action, summary, string(metadata), string(vision)))
	}
	return fmt.Sprintf(`%s
User request: %s

Live plugin context:
%s
Answer naturally as the desktop companion. Do not mention internal plugin names unless the user asks.`, instruction, userMessage, strings.TrimSpace(details.String()))
}

func (a *App) GenerateProactiveMessage(trigger string) (ChatReply, error) {
	if a.db == nil {
		return ChatReply{}, errors.New("database is not ready")
	}

	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		trigger = "idle"
	}

	history, err := a.fetchMessages()
	if err != nil {
		return ChatReply{}, err
	}
	memories, err := a.fetchMemories()
	if err != nil {
		return ChatReply{}, err
	}

	content := buildProactivePrompt(trigger, history)
	pluginContexts, pluginContextErrs := a.collectPluginContexts("proactive", trigger)
	if len(pluginContexts) > 0 {
		content = buildProactivePluginPrompt(trigger, history, pluginContexts)
	}
	agentReply, err := a.askAgentWithMode(content, history, memories, "proactive")
	if err != nil {
		agentReply = agentResponse{
			Text:       "我还在这边。刚才有点没连上模型，所以先不打扰你太久。",
			SpeechText: "まだここにいるよ。さっき少しモデルにつながりにくかったみたいだから、長くは邪魔しないね。",
			Emotion:    "thinking",
		}
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
	}
	if len(pluginContextErrs) > 0 {
		if strings.TrimSpace(a.providerErr) != "" {
			a.providerErr += " | "
		}
		a.providerErr += "proactive plugin context failed: " + strings.Join(pluginContextErrs, "; ")
	}

	reply, err := a.saveMessage("assistant", agentReply.Text, normalizedEmotion(agentReply.Emotion))
	if err != nil {
		return ChatReply{}, err
	}
	a.saveMemoryCandidates(agentReply.MemoryCandidates, reply.ID)

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
	return a.askAgentWithMode(content, history, memories, "chat")
}

func (a *App) askAgentWithMode(content string, history []Message, memories []string, mode string) (agentResponse, error) {
	payload, err := json.Marshal(agentRequest{
		Message:  content,
		History:  history,
		Memories: memories,
		Mode:     mode,
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

func buildProactivePrompt(trigger string, history []Message) string {
	var recent strings.Builder
	var recentAssistant strings.Builder
	start := len(history) - 6
	if start < 0 {
		start = 0
	}
	for _, message := range history[start:] {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		recent.WriteString(message.Role)
		recent.WriteString(": ")
		recent.WriteString(content)
		recent.WriteString("\n")
		if message.Role == "assistant" {
			recentAssistant.WriteString("- ")
			recentAssistant.WriteString(content)
			recentAssistant.WriteString("\n")
		}
	}
	return fmt.Sprintf(`Generate one short proactive desktop companion line.
Trigger: %s
Trigger meaning: %s
Recent context:
%s
Recent assistant lines to avoid repeating:
%s
Rules:
- One sentence, or two very short sentences at most.
- Low interruption. Do not demand an answer.
- Pick one intent: notice, encourage, lightly tease, or suggest one tiny next step.
- If recent context is technical, gently refer to it without summarizing the whole task.
- If this is a reply-follow-up, add a fresh small afterthought instead of answering the user again.
- If the trigger includes screen-context, use live context only when it gives a specific natural comment.
- If there is no useful context, simply check in warmly.
- Do not repeat recent assistant wording.
- Do not mention timers, idle detection, triggers, screenshots, plugins, or internal state.`, trigger, describeProactiveTrigger(trigger), strings.TrimSpace(recent.String()), strings.TrimSpace(recentAssistant.String()))
}

func buildProactivePluginPrompt(trigger string, history []Message, contexts []PluginContextCandidate) string {
	base := buildProactivePrompt(trigger, history)
	contextText := buildPluginContextPrompt(
		"Use this live plugin context only if it helps a low-interruption proactive desktop companion line.",
		trigger,
		contexts,
	)
	return fmt.Sprintf(`%s

Live context:
%s

Extra rules for this proactive line:
- You may gently tease, notice, or encourage based on the live context.
- Keep it natural, specific, and low-interruption.
- Do not say "I used a plugin" or name internal context systems.`, base, contextText)
}

func describeProactiveTrigger(trigger string) string {
	parts := strings.Split(strings.TrimSpace(trigger), ":")
	base := ""
	if len(parts) > 0 {
		base = strings.TrimSpace(parts[0])
	}
	var description string
	switch base {
	case "pet-idle":
		description = "The desktop pet has been quiet for a while and may make one small companion comment."
	case "free-idle":
		description = "Full mode free conversation is enabled and the user has been idle for a while."
	case "reply-follow-up":
		description = "The companion may add one natural short follow-up after its last answer, like a real conversation."
	default:
		description = "The companion may make one low-interruption proactive comment."
	}
	if strings.Contains(trigger, "screen-context") {
		description += " Live screen context may be available; use it only if it makes the comment more specific."
	}
	return description
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
FROM (
	SELECT id, role, content, emotion, created_at
	FROM messages
	ORDER BY id DESC
	LIMIT 80
)
ORDER BY id ASC`)
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
	_ = content
	currentName := desktopPetName()
	if expectsDualLanguageSpeech() {
		return agentResponse{
			Text:       "现在不是我只会固定回复，而是 Python Agent 或上游 LLM 暂时不可用。请检查前端下方的 Provider fallback 错误，或重启 agent/main.py。",
			SpeechText: fmt.Sprintf("今は%sが固定返答しかできないわけじゃなくて、Python AgentかLLMが一時的に使えないみたい。エラー表示を確認してね。", currentName),
			Emotion:    "thinking",
		}
	}
	if strings.HasPrefix(strings.ToLower(envFirst("YUYU_REPLY_LANGUAGE", "MOCHI_REPLY_LANGUAGE")), "ja") {
		return agentResponse{
			Text:    fmt.Sprintf("今は%sが固定返答しかできないわけじゃなくて、Python AgentかLLMが一時的に使えないみたい。エラー表示を確認してね。", currentName),
			Emotion: "thinking",
		}
	}
	return agentResponse{
		Text:    "现在不是我只会固定回复，而是 Python Agent 或上游 LLM 暂时不可用。请检查前端下方的 Provider fallback 错误，或重启 agent/main.py。",
		Emotion: "thinking",
	}

	name := desktopPetName()
	lower := strings.ToLower(content)
	useJapanese := strings.HasPrefix(strings.ToLower(envFirst("YUYU_REPLY_LANGUAGE", "MOCHI_REPLY_LANGUAGE")), "ja")

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
	name := envFirst("YUYU_DESKTOP_PET_NAME", "MOCHI_DESKTOP_PET_NAME")
	if name == "" {
		return "Yuyu"
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
	switch strings.ToLower(envFirst("YUYU_REPLY_LANGUAGE", "MOCHI_REPLY_LANGUAGE")) {
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
