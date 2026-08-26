package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// SidecarSpec 描述一个外部插件进程。
type SidecarSpec struct {
	Name    string   // 插件 ID（唯一）
	Command string   // 可执行文件路径
	Args    []string // 参数
	Env     []string // 额外环境变量（形如 KEY=VALUE）
}

// SidecarPlugin 通过 stdio 上的 newline-delimited JSON-RPC 驱动外部插件进程，
// 从而让第三方插件无需重新编译宿主即可挂载。它实现了 Plugin 接口。
type SidecarPlugin struct {
	spec     SidecarSpec
	client   *sidecarClient
	manifest Manifest
}

// NewSidecarPlugin 创建 sidecar 插件（进程在 Register 时启动）。
func NewSidecarPlugin(spec SidecarSpec) *SidecarPlugin {
	return &SidecarPlugin{spec: spec}
}

// Manifest 返回从 sidecar 进程协商得到的元数据；协商前返回仅含 Name 的桩。
func (p *SidecarPlugin) Manifest() Manifest {
	if p.manifest.Name == "" {
		return Manifest{SchemaVersion: "1.0", Name: p.spec.Name}
	}
	return p.manifest
}

// Init 启动 sidecar 进程、协商 manifest 并注册动作。
func (p *SidecarPlugin) Init(ctx context.Context, host *Host) error {
	client, err := newSidecarClient(p.spec)
	if err != nil {
		return fmt.Errorf("start sidecar %q: %w", p.spec.Name, err)
	}
	p.client = client

	raw, err := client.call("manifest", nil)
	if err != nil {
		client.close()
		return fmt.Errorf("sidecar %q manifest: %w", p.spec.Name, err)
	}
	manifest, err := decodeManifest(raw)
	if err != nil {
		client.close()
		return fmt.Errorf("sidecar %q manifest decode: %w", p.spec.Name, err)
	}
	manifest.Name = p.spec.Name
	p.manifest = manifest

	for _, action := range manifest.Actions {
		name := action.Name
		if err := host.RegisterAction(name, func(ctx context.Context, input map[string]any) (map[string]any, error) {
			return client.call("invoke_action", map[string]any{"action": name, "input": input})
		}); err != nil {
			return err
		}
	}
	return nil
}

// Start 通知 sidecar 进程启动。
func (p *SidecarPlugin) Start(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("sidecar %q is not initialized", p.spec.Name)
	}
	_, err := p.client.call("start", nil)
	return err
}

// Stop 通知 sidecar 进程停止并终止其进程。
func (p *SidecarPlugin) Stop(ctx context.Context) error {
	if p.client == nil {
		return nil
	}
	_, _ = p.client.call("stop", nil)
	p.client.close()
	p.client = nil
	return nil
}

func decodeManifest(raw map[string]any) (Manifest, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// sidecarClient 是与 sidecar 进程通信的 JSON-RPC 客户端。
// 由一个常驻读协程统一消费 stdout：带 id 的行作为响应派发给等待者，
// 带 "event" 字段的行作为进度事件交给 onEvent 回调（用于中途播报/实时 diff）。
type sidecarClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan sidecarResponse
	onEvent func(event string, data map[string]any)
	done    chan struct{}
	once    sync.Once
}

func newSidecarClient(spec SidecarSpec) (*sidecarClient, error) {
	cmd := exec.Command(spec.Command, spec.Args...)
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &sidecarClient{
		cmd:     cmd,
		stdin:   stdin,
		pending: make(map[int64]chan sidecarResponse),
		done:    make(chan struct{}),
	}
	go c.readLoop(bufio.NewReader(stdout))
	return c, nil
}

// readLoop 持续读取 sidecar 的 stdout，路由响应与事件。
func (c *sidecarClient) readLoop(r *bufio.Reader) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			c.failPending(fmt.Sprintf("sidecar stdout closed: %v", err))
			return
		}
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		// 事件行：{"event": "...", "data": {...}}（无 id）
		var raw map[string]any
		if json.Unmarshal([]byte(line), &raw) == nil {
			if ev, ok := raw["event"].(string); ok {
				if c.onEvent != nil {
					data, _ := raw["data"].(map[string]any)
					if data == nil {
						data = map[string]any{}
					}
					c.onEvent(ev, data)
				}
				continue
			}
		}
		var resp sidecarResponse
		if json.Unmarshal([]byte(line), &resp) != nil {
			continue
		}
		c.mu.Lock()
		if ch, ok := c.pending[resp.ID]; ok {
			delete(c.pending, resp.ID)
			ch <- resp
		}
		c.mu.Unlock()
	}
}

// failPending 在 sidecar 进程异常退出时，让所有等待中的调用返回错误。
func (c *sidecarClient) failPending(msg string) {
	c.mu.Lock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		select {
		case ch <- sidecarResponse{Error: &struct{ Message string `json:"message"` }{Message: msg}}:
		default:
		}
	}
	c.mu.Unlock()
	c.once.Do(func() { close(c.done) })
}

type sidecarResponse struct {
	ID     int64          `json:"id"`
	Result map[string]any `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *sidecarClient) call(method string, params map[string]any) (map[string]any, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan sidecarResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(append(req, '\n')); err != nil {
		return nil, fmt.Errorf("write sidecar request: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("sidecar error: %s", resp.Error.Message)
		}
		if resp.Result == nil {
			resp.Result = map[string]any{}
		}
		return resp.Result, nil
	case <-c.done:
		return nil, fmt.Errorf("sidecar process closed")
	}
}

func (c *sidecarClient) close() {
	c.once.Do(func() { close(c.done) })
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
}
