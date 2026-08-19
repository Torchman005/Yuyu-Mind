package asr

import (
	"strings"
	"testing"
)

func TestParseTranscriptionResponse(t *testing.T) {
	text, err := parseTranscriptionResponse([]byte(`{"text":"  你好，Yuyu。  "}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if text != "你好，Yuyu。" {
		t.Fatalf("unexpected text: %q", text)
	}

	if _, err := parseTranscriptionResponse([]byte(`{"text":"   "}`)); err == nil {
		t.Fatalf("expected error for empty text")
	}
	if _, err := parseTranscriptionResponse([]byte(`not-json`)); err == nil {
		t.Fatalf("expected error for invalid json")
	}
}

func TestExtFromContentType(t *testing.T) {
	cases := map[string]string{
		"audio/webm;codecs=opus": "webm",
		"audio/webm":             "webm",
		"audio/mp4":              "m4a",
		"audio/m4a":              "m4a",
		"audio/mpeg":             "mp3",
		"audio/mp3":              "mp3",
		"audio/wav":              "wav",
		"audio/ogg":              "ogg",
		"":                       "webm",
	}
	for contentType, want := range cases {
		if got := extFromContentType(contentType); got != want {
			t.Fatalf("extFromContentType(%q) = %q, want %q", contentType, got, want)
		}
	}
}

func TestBuildTranscriptionRequest(t *testing.T) {
	body, contentType, err := buildTranscriptionRequest("whisper-1", []byte("audio-bytes"), "audio/webm;codecs=opus", "zh")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	s := string(body)
	for _, want := range []string{"audio.webm", "whisper-1", "response_format", "json", "language", "zh"} {
		if !strings.Contains(s, want) {
			t.Fatalf("body missing %q", want)
		}
	}
}
