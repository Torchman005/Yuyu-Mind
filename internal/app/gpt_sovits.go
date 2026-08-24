package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ttsSplitMethod 是发送给 GPT-SoVITS 的 text_split_method。
// 默认 cut5 会在逗号等处分句并常把「带尾随逗号的短片段」哼成轻哼；cut1 对整句朗读更稳，
// 能避免短片段被哼声/丢读。
const ttsSplitMethod = "cut1"

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

// GetSpeechStreamUrl 返回一个可直接用于 <audio src> 的 GPT-SoVITS 流式合成 URL。
// 前端用它做渐进式播放：在上一句播放时提前用 audio.load() 预载，下一句播放时近零停顿。
// 仅 speech.provider 为 gpt_sovits 时可用；其余引擎返回错误，前端回退到 buffered 合成。
func (a *App) GetSpeechStreamUrl(text string, language string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("speech text cannot be empty")
	}
	if !strings.EqualFold(strings.TrimSpace(a.cfg.Speech.Provider), "gpt_sovits") {
		return "", fmt.Errorf("streaming TTS 仅支持 gpt_sovits，当前引擎 %q", a.cfg.Speech.Provider)
	}
	cfg := a.cfg.Speech.GPTSoVITS
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("GPT-SoVITS base url 未配置（config.json → speech.gpt_sovits.base_url）")
	}
	referPath := strings.TrimSpace(cfg.ReferAudioPath)
	if referPath == "" {
		return "", fmt.Errorf("GPT-SoVITS 参考音频路径未配置（config.json → speech.gpt_sovits.refer_audio_path）")
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = "/tts"
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	textLang := strings.TrimSpace(language)
	if textLang == "" {
		textLang = defaultString(cfg.TextLang, "auto")
	}
	// 文本语言须与文本脚本一致，避免中文文本被日语 G2P 乱读/截断。
	if detected := DetectTextLang(text); detected != "" {
		textLang = detected
	}

	streamingMode := cfg.StreamingMode
	if streamingMode <= 0 {
		streamingMode = 1
	}

	q := url.Values{}
	q.Set("text", text)
	q.Set("text_lang", textLang)
	q.Set("ref_audio_path", referPath)
	q.Set("prompt_text", cfg.PromptText)
	q.Set("prompt_lang", defaultString(cfg.PromptLang, "auto"))
	q.Set("top_k", "5")
	q.Set("top_p", "1.0")
	q.Set("temperature", "1.0")
	q.Set("text_split_method", ttsSplitMethod)
	q.Set("streaming_mode", strconv.Itoa(streamingMode))

	return baseURL + endpoint + "?" + q.Encode(), nil
}

// synthesizeGptSovitsSpeech 调用本地 GPT-SoVITS 合成语音（音色由参考音频决定）。
// language 覆盖 text_lang（用于中日语音切换）；为空则回退配置默认。
func (a *App) synthesizeGptSovitsSpeech(text string, language string) (SpeechReply, error) {
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

	textLang := strings.TrimSpace(language)
	if textLang == "" {
		textLang = defaultString(cfg.TextLang, "auto")
	}
	// 文本语言必须与实际文本脚本一致：中文文本不能用日语 G2P（否则会乱码/截断）。
	// 若传入语言与文本脚本不符，按脚本自动纠正（例如前端默认曾误传 ja，中文回复被当日语读）。
	if detected := DetectTextLang(text); detected != "" {
		textLang = detected
	}

	body, err := json.Marshal(gptSovitsRequest{
		Text:            text,
		TextLang:        textLang,
		RefAudioPath:    referPath,
		PromptText:      cfg.PromptText,
		PromptLang:      defaultString(cfg.PromptLang, "auto"),
		TopK:            5,
		TopP:            1.0,
		Temperature:     1.0,
		TextSplitMethod: ttsSplitMethod,
	})
	if err != nil {
		return SpeechReply{}, fmt.Errorf("marshal GPT-SoVITS request: %w", err)
	}
	slog.Info("[tts] gpt-sovits request", "text", text, "text_lang", textLang)

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
		slog.Error("[tts] gpt-sovits http error", "text", text, "text_lang", textLang, "status", resp.StatusCode, "body", strings.TrimSpace(string(msg)))
		return SpeechReply{}, fmt.Errorf("GPT-SoVITS 返回 %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return SpeechReply{}, fmt.Errorf("read GPT-SoVITS response: %w", err)
	}
	audio, err := parseGptSovitsResponse(data, resp.Header.Get("Content-Type"))
	if err != nil {
		slog.Error("[tts] gpt-sovits parse error", "text", text, "text_lang", textLang, "err", err)
		return SpeechReply{}, err
	}
	slog.Info("[tts] gpt-sovits ok", "text", text, "text_lang", textLang, "audio_bytes", len(audio))

	return SpeechReply{
		AudioBase64: base64.StdEncoding.EncodeToString(audio),
		ContentType: "audio/wav",
		Provider:    "GPT-SoVITS",
	}, nil
}

// parseGptSovitsResponse 解析 GPT-SoVITS 响应，返回原始音频字节（纯函数，便于测试）。
// 不同版本响应格式不同：主流 api_v2.py 直接返回 WAV 字节流（audio/wav）；部分 fork 返回
// JSON base64（{"data":[{"audio":"<base64>"}]} 或 {"audio":"<base64>"}）。这里兼容两者。
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

// DetectTextLang 粗略判断文本主体脚本：含日文假名 → "ja"；否则若含汉字 → "zh"；
// 其余返回空串（交由调用方用配置/默认 language）。
// 用于让 GPT-SoVITS 的 text_lang 始终匹配文本真实语言，避免中文文本被日语 G2P 乱读/截断。
func DetectTextLang(text string) string {
	hasKana := false
	hasHan := false
	for _, r := range text {
		switch {
		case r >= '\u3040' && r <= '\u30ff': // 平假名/片假名
			hasKana = true
		case unicode.Is(unicode.Han, r):
			hasHan = true
		}
	}
	if hasKana {
		return "ja"
	}
	if hasHan {
		return "zh"
	}
	return ""
}
