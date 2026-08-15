package plugin

import (
	"context"
	"testing"
)

type testPlugin struct {
	manifest Manifest
	started  bool
	stopped  bool
}

func (p *testPlugin) Manifest() Manifest { return p.manifest }

func (p *testPlugin) Init(ctx context.Context, host *Host) error {
	if err := host.RegisterAction("hello", func(ctx context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{"greeting": "hi", "name": input["name"]}, nil
	}); err != nil {
		return err
	}
	return host.RegisterAction("fail", func(ctx context.Context, input map[string]any) (map[string]any, error) {
		return nil, errTestAction
	})
}

func (p *testPlugin) Start(ctx context.Context) error { p.started = true; return nil }
func (p *testPlugin) Stop(ctx context.Context) error  { p.stopped = true; return nil }

var errTestAction = &testError{msg: "boom"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestManagerLifecycleAndAction(t *testing.T) {
	m := NewManager(nil, nil)
	p := &testPlugin{manifest: Manifest{Name: "test", Version: "1.0.0", Actions: []Action{{Name: "hello"}}}}

	if err := m.Register(context.Background(), p); err != nil {
		t.Fatalf("register: %v", err)
	}
	if !p.started {
		t.Fatalf("expected plugin started after register")
	}

	statuses := m.List()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(statuses))
	}
	if !statuses[0].Enabled {
		t.Fatalf("expected enabled by default")
	}
	if statuses[0].Manifest.Name != "test" {
		t.Fatalf("unexpected manifest name %q", statuses[0].Manifest.Name)
	}
	if len(statuses[0].Actions) != 2 || statuses[0].Actions[0] != "fail" || statuses[0].Actions[1] != "hello" {
		t.Fatalf("unexpected actions %v", statuses[0].Actions)
	}

	out, err := m.InvokeAction(context.Background(), "test", "hello", map[string]any{"name": "Yuyu"})
	if err != nil {
		t.Fatalf("invoke hello: %v", err)
	}
	if out["greeting"] != "hi" || out["name"] != "Yuyu" {
		t.Fatalf("unexpected result %v", out)
	}

	if _, err := m.InvokeAction(context.Background(), "test", "fail", nil); err == nil {
		t.Fatalf("expected error from failing action")
	}
	if _, err := m.InvokeAction(context.Background(), "test", "missing", nil); err == nil {
		t.Fatalf("expected error from missing action")
	}
	if _, err := m.InvokeAction(context.Background(), "ghost", "hello", nil); err == nil {
		t.Fatalf("expected error from missing plugin")
	}

	if err := m.Disable(context.Background(), "test"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !p.stopped {
		t.Fatalf("expected plugin stopped after disable")
	}
	if _, err := m.InvokeAction(context.Background(), "test", "hello", nil); err == nil {
		t.Fatalf("expected error invoking disabled plugin")
	}

	// 再次启用后应可调用。
	if err := m.Enable(context.Background(), "test"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if _, err := m.InvokeAction(context.Background(), "test", "hello", nil); err != nil {
		t.Fatalf("invoke after enable: %v", err)
	}

	m.StopAll(context.Background())
}

func TestManagerRegisterValidation(t *testing.T) {
	m := NewManager(nil, nil)
	if err := m.Register(context.Background(), &testPlugin{manifest: Manifest{Name: ""}}); err == nil {
		t.Fatalf("expected error for empty plugin name")
	}
}
