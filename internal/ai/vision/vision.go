// Package vision 提供多模态视觉描述能力（让模型「看懂」图片/截图）。
package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Describe 调用 OpenAI 兼容的多模态 API，让模型描述一张 PNG 图片。
func Describe(ctx context.Context, baseURL, apiKey, model, prompt string, imagePNG []byte) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("base url is required")
	}
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("vision model is required")
	}

	body, err := buildVisionRequest(model, prompt, imagePNG)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create vision request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call vision API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("vision API returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read vision response: %w", err)
	}
	return parseVisionResponse(data)
}

// buildVisionRequest 构造多模态请求体（纯函数，便于测试）。
func buildVisionRequest(model, prompt string, imagePNG []byte) ([]byte, error) {
	b64 := base64.StdEncoding.EncodeToString(imagePNG)
	payload := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": prompt},
					{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + b64}},
				},
			},
		},
		"max_tokens": 512,
	}
	return json.Marshal(payload)
}

// parseVisionResponse 解析多模态响应，返回文本内容（纯函数，便于测试）。
func parseVisionResponse(body []byte) (string, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("vision API returned no choices")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
