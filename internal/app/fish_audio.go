package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultFishAudioBaseURL = "https://api.fish.audio"

type fishTTSRequest struct {
	Text        string `json:"text"`
	ReferenceID string `json:"reference_id,omitempty"`
	Format      string `json:"format,omitempty"`
	Normalize   bool   `json:"normalize"`
	Latency     string `json:"latency,omitempty"`
}

func (a *App) synthesizeFishSpeech(text string) (SpeechReply, error) {
	if a.cfg == nil {
		return SpeechReply{}, fmt.Errorf("configuration is not loaded")
	}

	cfg := a.cfg.Speech.FishAudio
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return SpeechReply{}, fmt.Errorf("Fish Audio API key is not configured")
	}

	format := strings.TrimSpace(cfg.Format)
	if format == "" {
		format = "mp3"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultFishAudioBaseURL
	}

	body, err := json.Marshal(fishTTSRequest{
		Text:        text,
		ReferenceID: strings.TrimSpace(cfg.ReferenceID),
		Format:      format,
		Normalize:   true,
		Latency:     "balanced",
	})
	if err != nil {
		return SpeechReply{}, fmt.Errorf("marshal Fish Audio request: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, baseURL+"/v1/tts", bytes.NewReader(body))
	if err != nil {
		return SpeechReply{}, fmt.Errorf("create Fish Audio request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", contentTypeForAudioFormat(format))

	client := &http.Client{
		Timeout:   75 * time.Second,
		Transport: &http.Transport{Proxy: fishAudioProxy},
	}
	resp, err := client.Do(req)
	if err != nil {
		return SpeechReply{}, fmt.Errorf("call Fish Audio TTS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return SpeechReply{}, fmt.Errorf("Fish Audio TTS failed: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return SpeechReply{}, fmt.Errorf("read Fish Audio response: %w", err)
	}
	if len(audio) == 0 {
		return SpeechReply{}, fmt.Errorf("Fish Audio returned empty audio")
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || strings.HasPrefix(strings.ToLower(contentType), "application/octet-stream") {
		contentType = contentTypeForAudioFormat(format)
	}

	return SpeechReply{
		AudioBase64: base64.StdEncoding.EncodeToString(audio),
		ContentType: contentType,
		Provider:    "Fish Audio",
	}, nil
}

func (a *App) probeFishSpeech() (FishLiveProbeResult, error) {
	started := time.Now()
	reply, err := a.synthesizeFishSpeech("测试")
	if err != nil {
		return FishLiveProbeResult{
			OK:        false,
			Error:     err.Error(),
			Events:    []string{"tts-failed"},
			ElapsedMs: time.Since(started).Milliseconds(),
		}, nil
	}
	return FishLiveProbeResult{
		OK:        true,
		Events:    []string{"tts-ok"},
		ElapsedMs: time.Since(started).Milliseconds(),
		AudioSize: len(reply.AudioBase64) * 3 / 4,
	}, nil
}

func contentTypeForAudioFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	case "opus":
		return "audio/ogg"
	default:
		return "audio/mpeg"
	}
}
