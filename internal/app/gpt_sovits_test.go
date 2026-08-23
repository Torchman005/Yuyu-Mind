package app

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/yuyu-mind/backend/internal/config"
)

func TestGetSpeechStreamUrl(t *testing.T) {
	cfg := &config.Config{
		Speech: config.SpeechConfig{
			Provider: "gpt_sovits",
			GPTSoVITS: config.GPTSoVITSConfig{
				BaseURL:        "http://127.0.0.1:9880",
				Endpoint:       "/tts",
				ReferAudioPath: "E:/语音/zh_10s.wav",
				PromptText:     "你看！月亮好漂亮",
				PromptLang:     "zh",
				TextLang:       "zh",
				StreamingMode:  2,
			},
		},
	}
	a := &App{cfg: cfg}

	got, err := a.GetSpeechStreamUrl("你好呀", "")
	if err != nil {
		t.Fatalf("GetSpeechStreamUrl: %v", err)
	}
	if !strings.HasPrefix(got, "http://127.0.0.1:9880/tts?") {
		t.Fatalf("unexpected url prefix: %s", got)
	}
	for _, want := range []string{"text_lang=zh", "prompt_lang=zh", "streaming_mode=2", "ref_audio_path=", "prompt_text=", "text="} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in url: %s", want, got)
		}
	}
	// language 覆盖 text_lang。
	got2, err := a.GetSpeechStreamUrl("你好呀", "ja")
	if err != nil {
		t.Fatalf("GetSpeechStreamUrl ja: %v", err)
	}
	if !strings.Contains(got2, "text_lang=ja") {
		t.Fatalf("expected text_lang=ja: %s", got2)
	}

	// 非 gpt_sovits 引擎报错，前端回退 buffered。
	cfg.Speech.Provider = "fish_audio"
	if _, err := a.GetSpeechStreamUrl("你好呀", ""); err == nil {
		t.Fatalf("expected error for non gpt_sovits provider")
	}
}

func TestParseGptSovitsResponse(t *testing.T) {
	// api_v2：JSON data[0].audio。
	audio := []byte{0x52, 0x49, 0x46, 0x46, 0x01, 0x02, 0x03, 0x04} // 伪 WAV 字节
	b64 := base64.StdEncoding.EncodeToString(audio)
	got, err := parseGptSovitsResponse([]byte(`{"type":"audio","data":[{"audio":"`+b64+`","sr":32000}]}`), "application/json")
	if err != nil {
		t.Fatalf("parse json data: %v", err)
	}
	if string(got) != string(audio) {
		t.Fatalf("decoded audio mismatch: %v", got)
	}

	// 某些 fork：JSON audio 顶层字段。
	got, err = parseGptSovitsResponse([]byte(`{"audio":"`+b64+`"}`), "application/json")
	if err != nil {
		t.Fatalf("parse json audio: %v", err)
	}
	if string(got) != string(audio) {
		t.Fatalf("decoded audio mismatch: %v", got)
	}

	// api.py：直接返回原始 WAV 字节（Content-Type 非 json）。
	got, err = parseGptSovitsResponse(audio, "audio/wav")
	if err != nil {
		t.Fatalf("parse raw wav: %v", err)
	}
	if string(got) != string(audio) {
		t.Fatalf("raw wav mismatch: %v", got)
	}

	// 空响应报错。
	if _, err := parseGptSovitsResponse([]byte("   "), "application/json"); err == nil {
		t.Fatalf("expected error for empty body")
	}
	if _, err := parseGptSovitsResponse([]byte(`{"data":[{}]}`), "application/json"); err == nil {
		t.Fatalf("expected error for empty audio")
	}
}
