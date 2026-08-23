package config

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ActiveProvider.ProviderID != "deepseek" {
		t.Fatalf("default active provider = %q", cfg.ActiveProvider.ProviderID)
	}
	if cfg.Providers["deepseek"].BaseURL == "" || cfg.Providers["deepseek"].Model == "" {
		t.Fatalf("deepseek provider incomplete: %+v", cfg.Providers["deepseek"])
	}
	if _, ok := cfg.Providers["bailian"]; !ok {
		t.Fatalf("bailian provider missing")
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

func TestLoadSaveRoundTrip(t *testing.T) {
	t.Setenv("YUYU_CONFIG_DIR", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ActiveProvider.ProviderID != "deepseek" {
		t.Fatalf("default active provider = %q", cfg.ActiveProvider.ProviderID)
	}

	if err := cfg.SetActiveProvider("bailian", "qwen-plus"); err != nil {
		t.Fatalf("SetActiveProvider: %v", err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load again: %v", err)
	}
	if cfg2.ActiveProvider.ProviderID != "bailian" || cfg2.ActiveProvider.Model != "qwen-plus" {
		t.Fatalf("round-trip failed: %+v", cfg2.ActiveProvider)
	}
}
