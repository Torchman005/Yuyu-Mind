package config

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ActiveProvider.ProviderID != "openai" {
		t.Fatalf("default active provider = %q", cfg.ActiveProvider.ProviderID)
	}
	if cfg.Providers["openai"].BaseURL == "" || cfg.Providers["openai"].Model == "" {
		t.Fatalf("openai provider incomplete: %+v", cfg.Providers["openai"])
	}
	if _, ok := cfg.Providers["deepseek"]; !ok {
		t.Fatalf("deepseek provider missing")
	}
	if _, ok := cfg.Providers["ollama"]; !ok {
		t.Fatalf("ollama provider missing")
	}
	if cfg.Chat.MaxReplyChars <= 0 || cfg.Chat.SplitMaxChars <= 0 {
		t.Fatalf("chat split config invalid")
	}
}

func TestSetAndGetActiveProvider(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.SetActiveProvider("deepseek", "deepseek-chat"); err != nil {
		t.Fatalf("SetActiveProvider: %v", err)
	}
	p, err := cfg.GetActiveProviderConfig()
	if err != nil {
		t.Fatalf("GetActiveProviderConfig: %v", err)
	}
	if p.Model != "deepseek-chat" || p.BaseURL == "" {
		t.Fatalf("active provider = %+v", p)
	}

	if err := cfg.SetActiveProvider("ghost", "m"); err == nil {
		t.Fatalf("expected error for unknown provider")
	}
}

func TestUpdateProvider(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.UpdateProvider("openai", Provider{Name: "自定义", BaseURL: "http://x/v1", APIKey: "k", Model: "m"}); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if cfg.Providers["openai"].Name != "自定义" || cfg.Providers["openai"].APIKey != "k" {
		t.Fatalf("update provider failed: %+v", cfg.Providers["openai"])
	}
}
