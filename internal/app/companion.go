package app

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yuyu-mind/backend/internal/ai/asr"
	"github.com/yuyu-mind/backend/internal/chat"
	"github.com/yuyu-mind/backend/internal/db"
)

const defaultCompanionConversationID = "desktop-companion"

type CompanionMessage struct {
	ID        string  `json:"id"`
	Role      string  `json:"role"`
	Content   string  `json:"content"`
	Emotion   string  `json:"emotion"`
	Mood      string  `json:"mood"`
	Energy    float64 `json:"energy"`
	Valence   float64 `json:"valence"`
	Dominance float64 `json:"dominance"`
	Gesture   string  `json:"gesture"`
	Hand      string  `json:"hand"`
	CreatedAt string  `json:"createdAt"`
}

type ChatReply struct {
	Messages      []CompanionMessage `json:"messages"`
	Reply         CompanionMessage   `json:"reply"`
	SpeechText    string             `json:"speechText"`
	Emotion       string             `json:"emotion"`
	Mood          string             `json:"mood"`
	Energy        float64            `json:"energy"`
	Valence       float64            `json:"valence"`
	Dominance     float64            `json:"dominance"`
	Gesture       string             `json:"gesture"`
	Hand          string             `json:"hand"`
	AgentStatus   string             `json:"agentStatus"`
	AgentProvider string             `json:"agentProvider"`
	ProviderError string             `json:"providerError"`
}

type AppState struct {
	Messages      []CompanionMessage `json:"messages"`
	Emotion       string             `json:"emotion"`
	AgentStatus   string             `json:"agentStatus"`
	AgentProvider string             `json:"agentProvider"`
	ProviderError string             `json:"providerError"`
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

type collectingEmitter struct {
	mu        sync.Mutex
	tokens    strings.Builder
	err       string
	emotion   string
	mood      string
	energy    float64
	valence   float64
	dominance float64
	gesture   string
	hand      string
}

func (e *collectingEmitter) Emit(event chat.ChatEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch event.Type {
	case chat.EventTypeToken:
		e.tokens.WriteString(event.Content)
	case chat.EventTypeEmotion:
		e.emotion = event.Emotion
		e.mood = event.Mood
		e.energy = event.Energy
		e.valence = event.Valence
		e.dominance = event.Dominance
		e.gesture = event.Gesture
		e.hand = event.Hand
	case chat.EventTypeError:
		e.err = event.Content
	}
}

func (e *collectingEmitter) content() (string, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.TrimSpace(e.tokens.String()), strings.TrimSpace(e.err)
}

// collectedEmotion 返回 Planner 产出的结构化表情；空串表示 LLM 未产出，调用方回退启发式。
func (e *collectingEmitter) collectedEmotion() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.emotion
}

// collectedPerformance 返回 Planner 产出的完整表演参数（mood/energy/valence/dominance/gesture/hand）。
func (e *collectingEmitter) collectedPerformance() (string, float64, float64, float64, string, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mood, e.energy, e.valence, e.dominance, e.gesture, e.hand
}

func (a *App) GetState() (AppState, error) {
	if err := a.ensureCompanionReady(); err != nil {
		return AppState{}, err
	}
	messages, err := a.companionMessages()
	if err != nil {
		return AppState{}, err
	}
	return AppState{
		Messages:      messages,
		Emotion:       latestEmotion(messages),
		AgentStatus:   "online",
		AgentProvider: a.activeProviderName(),
	}, nil
}

func (a *App) ClearChat() (AppState, error) {
	if err := a.ensureCompanionReady(); err != nil {
		return AppState{}, err
	}
	if err := a.db.Messages.DeleteByConversation(a.ctx, defaultCompanionConversationID); err != nil {
		return AppState{}, err
	}
	return AppState{
		Messages:      []CompanionMessage{},
		Emotion:       "neutral",
		AgentStatus:   "online",
		AgentProvider: a.activeProviderName(),
	}, nil
}

func (a *App) SendMessage(content string) (ChatReply, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return ChatReply{}, errors.New("message cannot be empty")
	}
	if err := a.ensureCompanionReady(); err != nil {
		return ChatReply{}, err
	}

	emitter := &collectingEmitter{}
	err := a.chatSvc.StreamChat(a.ctx, chat.ChatRequest{
		ConversationID: defaultCompanionConversationID,
		Content:        content,
		UseTools:       false,
	}, emitter)
	replyText, providerErr := emitter.content()
	if err != nil {
		providerErr = err.Error()
	}
	if replyText == "" && providerErr != "" {
		replyText = "模型暂时没有返回内容，请检查供应商配置后再试。"
	}
	if replyText == "" {
		replyText = "我还在，但刚才没有组织出有效回复。"
	}

	messages, listErr := a.companionMessages()
	if listErr != nil {
		return ChatReply{}, listErr
	}
	reply := latestAssistant(messages)
	emotion := emitter.collectedEmotion()
	mood, energy, valence, dominance, gesture, hand := emitter.collectedPerformance()
	if emotion == "" {
		emotion = inferEmotion(replyText)
		valence = inferValence(emotion)
		dominance = inferDominance(emotion)
	}
	if mood == "" {
		mood = chat.MoodCalm
	}
	if reply.ID == "" || reply.Content != replyText {
		reply = CompanionMessage{
			ID:        uuid.New().String(),
			Role:      "assistant",
			Content:   replyText,
			Emotion:   emotion,
			Mood:      mood,
			Energy:    energy,
			Valence:   valence,
			Dominance: dominance,
			Gesture:   gesture,
			Hand:      hand,
			CreatedAt: time.Now().Format(time.RFC3339),
		}
	} else {
		reply.Emotion = emotion
		reply.Mood = mood
		reply.Energy = energy
		reply.Valence = valence
		reply.Dominance = dominance
		reply.Gesture = gesture
		reply.Hand = hand
	}

	return ChatReply{
		Messages:      messages,
		Reply:         reply,
		SpeechText:    replyText,
		Emotion:       reply.Emotion,
		Mood:          mood,
		Energy:        energy,
		Valence:       valence,
		Dominance:     dominance,
		Gesture:       gesture,
		Hand:          hand,
		AgentStatus:   statusFromError(providerErr),
		AgentProvider: a.activeProviderName(),
		ProviderError: providerErr,
	}, err
}

func (a *App) GenerateProactiveMessage(trigger string) (ChatReply, error) {
	if err := a.ensureCompanionReady(); err != nil {
		return ChatReply{}, err
	}
	messages, err := a.companionMessages()
	if err != nil {
		return ChatReply{}, err
	}
	line := buildProactiveLine(trigger, messages)
	reply := CompanionMessage{
		ID:        uuid.New().String(),
		Role:      "assistant",
		Content:   line,
		Emotion:   inferEmotion(line),
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	if err := a.db.Messages.Create(a.ctx, &db.Message{
		ID:             reply.ID,
		ConversationID: defaultCompanionConversationID,
		Role:           reply.Role,
		Content:        reply.Content,
		CreatedAt:      time.Now(),
	}); err != nil {
		return ChatReply{}, err
	}
	messages, err = a.companionMessages()
	if err != nil {
		return ChatReply{}, err
	}
	return ChatReply{
		Messages:      messages,
		Reply:         reply,
		SpeechText:    line,
		Emotion:       reply.Emotion,
		AgentStatus:   "online",
		AgentProvider: a.activeProviderName(),
	}, nil
}

func (a *App) ObserveScreen(prompt string) (ChatReply, error) {
	content, emotion, err := a.observeScreenText(prompt)
	if err != nil {
		return a.GenerateProactiveMessage("screen-observe-unavailable")
	}
	reply := CompanionMessage{
		ID:        uuid.New().String(),
		Role:      "assistant",
		Content:   content,
		Emotion:   emotion,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	return ChatReply{
		Messages:      []CompanionMessage{reply},
		Reply:         reply,
		SpeechText:    content,
		Emotion:       emotion,
		AgentStatus:   "online",
		AgentProvider: a.activeProviderName(),
	}, nil
}

func (a *App) SynthesizeSpeech(text string) (SpeechReply, error) {
	if strings.TrimSpace(text) == "" {
		return SpeechReply{}, errors.New("speech text cannot be empty")
	}
	return a.synthesizeFishSpeech(text)
}

func (a *App) SynthesizeSpeechStream(text string) (SpeechStreamStart, error) {
	if strings.TrimSpace(text) == "" {
		return SpeechStreamStart{}, errors.New("speech text cannot be empty")
	}
	return SpeechStreamStart{}, errors.New("streaming TTS is not configured in this build")
}

func (a *App) StartRealtimeSpeech(text string) (SpeechStreamStart, error) {
	if strings.TrimSpace(text) == "" {
		return SpeechStreamStart{}, errors.New("speech text cannot be empty")
	}
	return SpeechStreamStart{}, errors.New("realtime speech is not configured in this build")
}

func (a *App) TranscribeAudio(audioBase64 string, contentType string, language string) (ASRReply, error) {
	if strings.TrimSpace(audioBase64) == "" {
		return ASRReply{}, errors.New("audio payload is empty")
	}
	model := strings.TrimSpace(a.cfg.ASR.Model)
	if model == "" {
		return ASRReply{}, errors.New("ASR 模型未配置：请在 config.json 的 asr.model 填入模型名（如 whisper-1）")
	}

	audio, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return ASRReply{}, fmt.Errorf("decode audio: %w", err)
	}

	providerCfg, err := a.cfg.GetActiveProviderConfig()
	if err != nil {
		return ASRReply{}, err
	}

	text, err := asr.Transcribe(a.ctx, providerCfg.BaseURL, providerCfg.APIKey, model, audio, contentType, language)
	if err != nil {
		return ASRReply{}, err
	}
	return ASRReply{
		Text:     text,
		Provider: model,
		Language: language,
	}, nil
}

func (a *App) ProbeFishLive() (FishLiveProbeResult, error) {
	return a.probeFishSpeech()
}

func (a *App) ensureCompanionReady() error {
	if a.ctx == nil || a.db == nil || a.chatSvc == nil || a.cfg == nil {
		return errors.New("application is not ready")
	}
	existing, err := a.db.Conversations.GetByID(a.ctx, defaultCompanionConversationID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	providerCfg, _ := a.cfg.GetActiveProviderConfig()
	now := time.Now()
	return a.db.Conversations.Create(a.ctx, &db.Conversation{
		ID:        defaultCompanionConversationID,
		Title:     "Yuyu Desktop Companion",
		Provider:  a.cfg.ActiveProvider.ProviderID,
		Model:     providerCfg.Model,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (a *App) companionMessages() ([]CompanionMessage, error) {
	rows, err := a.db.Messages.ListByConversation(a.ctx, defaultCompanionConversationID)
	if err != nil {
		return nil, err
	}
	messages := make([]CompanionMessage, 0, len(rows))
	for _, row := range rows {
		emotion := row.Emotion
		valence := row.Valence
		dominance := row.Dominance
		if emotion == "" {
			emotion = inferEmotion(row.Content)
			valence = inferValence(emotion)
			dominance = inferDominance(emotion)
		}
		messages = append(messages, CompanionMessage{
			ID:        row.ID,
			Role:      row.Role,
			Content:   row.Content,
			Emotion:   emotion,
			Mood:      row.Mood,
			Energy:    row.Energy,
			Valence:   valence,
			Dominance: dominance,
			Gesture:   row.Gesture,
			Hand:      row.Hand,
			CreatedAt: row.CreatedAt.Format(time.RFC3339),
		})
	}
	return messages, nil
}

func (a *App) activeProviderName() string {
	if a.cfg == nil {
		return "unknown"
	}
	providerCfg, err := a.cfg.GetActiveProviderConfig()
	if err != nil {
		return a.cfg.ActiveProvider.ProviderID
	}
	if strings.TrimSpace(providerCfg.Name) != "" {
		return providerCfg.Name
	}
	return a.cfg.ActiveProvider.ProviderID
}

func latestEmotion(messages []CompanionMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return messages[i].Emotion
		}
	}
	return "neutral"
}

func latestAssistant(messages []CompanionMessage) CompanionMessage {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return messages[i]
		}
	}
	return CompanionMessage{}
}

func statusFromError(value string) string {
	if strings.TrimSpace(value) != "" {
		return "offline"
	}
	return "online"
}

func inferEmotion(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.ContainsAny(text, "开心喜欢太好了谢谢") || strings.Contains(lower, "great"):
		return "happy"
	case strings.ContainsAny(text, "错误失败抱歉") || strings.Contains(lower, "error"):
		return "thinking"
	case strings.ContainsAny(text, "代码项目配置测试") || strings.Contains(lower, "code"):
		return "focused"
	case strings.ContainsAny(text, "难过低落"):
		return "sad"
	default:
		return "neutral"
	}
}

// inferValence 由离散表情推断效价（-1..1），作为 LLM 未产出连续情绪时的回退。
func inferValence(emotion string) float64 {
	switch emotion {
	case chat.EmotionHappy:
		return 0.7
	case chat.EmotionSurprised:
		return 0.3
	case chat.EmotionFocused:
		return 0.2
	case chat.EmotionSad:
		return -0.6
	case chat.EmotionThinking:
		return 0
	default:
		return 0
	}
}

// inferDominance 由离散表情推断支配度（-1..1），作为 LLM 未产出连续情绪时的回退。
func inferDominance(emotion string) float64 {
	switch emotion {
	case chat.EmotionFocused:
		return 0.4
	case chat.EmotionHappy:
		return 0.2
	case chat.EmotionSurprised:
		return -0.1
	case chat.EmotionSad:
		return -0.4
	default:
		return 0
	}
}

func buildProactiveLine(trigger string, messages []CompanionMessage) string {
	last := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			last = strings.TrimSpace(messages[i].Content)
			break
		}
	}
	if strings.Contains(trigger, "screen-observe-unavailable") {
		return "当前版本还没有接入屏幕观察能力；我可以先根据聊天上下文继续帮你。"
	}
	if last != "" {
		return fmt.Sprintf("我还记着你刚才说的「%s」，要不要继续从这里往下处理？", truncateRunes(last, 28))
	}
	return "我在这边，随时可以继续。"
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}
