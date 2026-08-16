package vision

import (
	"strings"
	"testing"
)

func TestBuildVisionRequest(t *testing.T) {
	body, err := buildVisionRequest("gpt-4o", "描述这张图", []byte{0x89, 0x50, 0x4E, 0x47})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, `"model":"gpt-4o"`) {
		t.Fatalf("missing model: %s", s)
	}
	if !strings.Contains(s, "data:image/png;base64,iVBORw") {
		t.Fatalf("missing image data URL: %s", s)
	}
	if !strings.Contains(s, "image_url") || !strings.Contains(s, "描述这张图") {
		t.Fatalf("missing multimodal content: %s", s)
	}
}

func TestParseVisionResponse(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"屏幕上有一个编辑器窗口。"}}]}`)
	text, err := parseVisionResponse(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if text != "屏幕上有一个编辑器窗口。" {
		t.Fatalf("unexpected text %q", text)
	}

	if _, err := parseVisionResponse([]byte(`{"choices":[]}`)); err == nil {
		t.Fatalf("expected error for empty choices")
	}
	if _, err := parseVisionResponse([]byte(`not json`)); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}
