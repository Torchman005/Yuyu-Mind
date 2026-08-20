package app

import (
	"encoding/base64"
	"testing"
)

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
