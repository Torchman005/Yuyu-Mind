// Package asr 提供语音识别（Whisper 兼容）能力，让模型「听懂」用户语音。
package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Transcribe 调用 OpenAI 兼容的 /audio/transcriptions 端点，把音频转成文本。
func Transcribe(ctx context.Context, baseURL, apiKey, model string, audio []byte, contentType, language string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("base url is required")
	}
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("asr model is required")
	}
	if len(audio) == 0 {
		return "", fmt.Errorf("audio is empty")
	}

	body, formContentType, err := buildTranscriptionRequest(model, audio, contentType, language)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/audio/transcriptions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create asr request: %w", err)
	}
	req.Header.Set("Content-Type", formContentType)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call asr API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("asr API returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read asr response: %w", err)
	}
	return parseTranscriptionResponse(data)
}

// buildTranscriptionRequest 构造 multipart/form-data 请求体（返回字节与 Content-Type）。
func buildTranscriptionRequest(model string, audio []byte, contentType, language string) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "audio."+extFromContentType(contentType))
	if err != nil {
		return nil, "", fmt.Errorf("create audio part: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return nil, "", fmt.Errorf("write audio part: %w", err)
	}
	if err := writer.WriteField("model", model); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(language) != "" {
		if err := writer.WriteField("language", strings.TrimSpace(language)); err != nil {
			return nil, "", err
		}
	}
	if err := writer.WriteField("response_format", "json"); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}

// parseTranscriptionResponse 解析 Whisper 兼容响应，返回文本（纯函数，便于测试）。
func parseTranscriptionResponse(body []byte) (string, error) {
	var resp struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse asr response: %w", err)
	}
	text := strings.TrimSpace(resp.Text)
	if text == "" {
		return "", fmt.Errorf("asr API returned empty text")
	}
	return text, nil
}

// extFromContentType 由 MIME 类型推导音频文件扩展名（纯函数）。
func extFromContentType(contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "webm"):
		return "webm"
	case strings.Contains(ct, "mp4"), strings.Contains(ct, "m4a"):
		return "m4a"
	case strings.Contains(ct, "mpeg"), strings.Contains(ct, "mp3"):
		return "mp3"
	case strings.Contains(ct, "wav"):
		return "wav"
	case strings.Contains(ct, "ogg"), strings.Contains(ct, "opus"):
		return "ogg"
	default:
		return "webm"
	}
}
