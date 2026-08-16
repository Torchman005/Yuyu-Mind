package plugin

import (
	"context"
	"testing"
)

type memConfigStore struct {
	data map[string]map[string]any
}

func (s *memConfigStore) Get(ctx context.Context, pluginID string) (map[string]any, error) {
	if s.data == nil {
		return map[string]any{}, nil
	}
	if cfg, ok := s.data[pluginID]; ok {
		return cfg, nil
	}
	return map[string]any{}, nil
}

func (s *memConfigStore) Set(ctx context.Context, pluginID string, config map[string]any) error {
	if s.data == nil {
		s.data = make(map[string]map[string]any)
	}
	s.data[pluginID] = config
	return nil
}

func TestPluginConfigRoundTrip(t *testing.T) {
	m := NewManager(nil, nil)
	m.SetConfigStore(&memConfigStore{})

	cfg, err := m.GetConfig(context.Background(), "system")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if len(cfg) != 0 {
		t.Fatalf("expected empty config, got %v", cfg)
	}

	if err := m.SetConfig(context.Background(), "system", map[string]any{"greeting": "hi"}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	cfg, err = m.GetConfig(context.Background(), "system")
	if err != nil {
		t.Fatalf("GetConfig after set: %v", err)
	}
	if cfg["greeting"] != "hi" {
		t.Fatalf("config mismatch: %v", cfg)
	}
}

func TestPluginConfigWithoutStore(t *testing.T) {
	m := NewManager(nil, nil)

	cfg, err := m.GetConfig(context.Background(), "system")
	if err != nil || len(cfg) != 0 {
		t.Fatalf("get without store: %v %v", cfg, err)
	}
	if err := m.SetConfig(context.Background(), "system", map[string]any{"a": 1}); err == nil {
		t.Fatalf("expected error setting without store")
	}
}
