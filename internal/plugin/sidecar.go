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
type sidecarClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID int64
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
	return &sidecarClient{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}, nil
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
	defer c.mu.Unlock()

	c.nextID++
	id := c.nextID
	req, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(append(req, '\n')); err != nil {
		return nil, fmt.Errorf("write sidecar request: %w", err)
	}

	// 逐行读取直到拿到匹配 id 的 JSON 响应（跳过非 JSON 行，如测试框架输出）。
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read sidecar response: %w", err)
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var resp sidecarResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("sidecar error: %s", resp.Error.Message)
		}
		if resp.ID == id {
			if resp.Result == nil {
				resp.Result = map[string]any{}
			}
			return resp.Result, nil
		}
	}
}

func (c *sidecarClient) close() {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
}
