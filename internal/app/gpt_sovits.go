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

// gptSovitsRequest 对应 GPT-SoVITS api_v2.py 的 TTS 入参。
// 不同版本字段略有差异，这里覆盖常用字段，多余的用 omitempty 省略。
type gptSovitsRequest struct {
	Text            string  `json:"text"`
	TextLang        string  `json:"text_lang"`
	RefAudioPath    string  `json:"ref_audio_path"`
	PromptText      string  `json:"prompt_text"`
	PromptLang      string  `json:"prompt_lang"`
	TopK            int     `json:"top_k,omitempty"`
	TopP            float64 `json:"top_p,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
	SpeedFactor     float64 `json:"speed_factor,omitempty"`
	TextSplitMethod string  `json:"text_split_method,omitempty"`
}

// synthesizeGptSovitsSpeech 调用本地 GPT-SoVITS 合成语音（音色由参考音频决定）。
func (a *App) synthesizeGptSovitsSpeech(text string) (SpeechReply, error) {
	cfg := a.cfg.Speech.GPTSoVITS
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return SpeechReply{}, fmt.Errorf("GPT-SoVITS base url 未配置（config.json → speech.gpt_sovits.base_url）")
	}
	referPath := strings.TrimSpace(cfg.ReferAudioPath)
	if referPath == "" {
		return SpeechReply{}, fmt.Errorf("GPT-SoVITS 参考音频路径未配置（config.json → speech.gpt_sovits.refer_audio_path）")
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = "/tts"
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	body, err := json.Marshal(gptSovitsRequest{
		Text:         text,
		TextLang:     defaultString(cfg.TextLang, "auto"),
		RefAudioPath: referPath,
		PromptText:   cfg.PromptText,
		PromptLang:   defaultString(cfg.PromptLang, "auto"),
		TopK:         5,
		TopP:         1.0,
		Temperature:  1.0,
	})
	if err != nil {
		return SpeechReply{}, fmt.Errorf("marshal GPT-SoVITS request: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return SpeechReply{}, fmt.Errorf("create GPT-SoVITS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return SpeechReply{}, fmt.Errorf("call GPT-SoVITS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return SpeechReply{}, fmt.Errorf("GPT-SoVITS 返回 %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return SpeechReply{}, fmt.Errorf("read GPT-SoVITS response: %w", err)
	}
	audio, err := parseGptSovitsResponse(data, resp.Header.Get("Content-Type"))
	if err != nil {
		return SpeechReply{}, err
	}

	return SpeechReply{
		AudioBase64: base64.StdEncoding.EncodeToString(audio),
		ContentType: "audio/wav",
		Provider:    "GPT-SoVITS",
	}, nil
}

// parseGptSovitsResponse 解析 GPT-SoVITS 响应，返回原始音频字节（纯函数，便于测试）。
// - api_v2.py 返回 JSON：{"type":"audio","data":[{"audio":"<base64>","sr":...}]} 或 {"audio":"<base64>"}。
// - api.py 直接返回原始 WAV 字节。
func parseGptSovitsResponse(body []byte, contentType string) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("GPT-SoVITS 返回空响应")
	}

	isJSON := strings.Contains(strings.ToLower(contentType), "application/json") || trimmed[0] == '{'
	if !isJSON {
		// 原始音频字节（WAV）。
		return body, nil
	}

	var resp struct {
		Data []struct {
			Audio string `json:"audio"`
		} `json:"data"`
		Audio string `json:"audio"`
	}
	if err := json.Unmarshal(trimmed, &resp); err != nil {
		return nil, fmt.Errorf("parse GPT-SoVITS json response: %w", err)
	}

	b64 := resp.Audio
	if b64 == "" && len(resp.Data) > 0 {
		b64 = resp.Data[0].Audio
	}
	if strings.TrimSpace(b64) == "" {
		return nil, fmt.Errorf("GPT-SoVITS 返回空音频")
	}
	audio, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode GPT-SoVITS audio: %w", err)
	}
	return audio, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
