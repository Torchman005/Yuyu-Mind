package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

var errInputUnsupported = errors.New("keyboard input synthesis is not supported on this platform")

// platformInputExecutor 是键盘输入的平台实现接口（Windows 用 SendInput，其它平台 no-op）。
// 具体实例由 input_windows.go / input_other.go 通过包级变量 platformInput 提供。
type platformInputExecutor interface {
	KeyPress(vk uint16) error
	TypeText(text string) error
}

// KeyVK 把按键名映射为 Windows 虚拟键码（纯函数，便于测试）。
func KeyVK(name string) (uint16, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if len(key) == 1 {
		c := key[0]
		if c >= 'a' && c <= 'z' {
			return uint16(c-'a') + 0x41, true // 'A'..'Z' 的 VK 码
		}
		if c >= '0' && c <= '9' {
			return uint16(c), true // 数字键 VK 码即其 ASCII
		}
	}
	specials := map[string]uint16{
		"space": 0x20, "enter": 0x0D, "return": 0x0D, "escape": 0x1B, "esc": 0x1B,
		"tab": 0x09, "up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
		"shift": 0x10, "ctrl": 0x11, "control": 0x11, "alt": 0x12,
		"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74,
	}
	vk, ok := specials[key]
	return vk, ok
}

// InputInput 是 send_input 的参数。
type InputInput struct {
	Type string `json:"type" jsonschema:"description=key_press or type_text"`
	Key  string `json:"key" jsonschema:"description=Key name for key_press, e.g. w, space, enter, escape."`
	Text string `json:"text" jsonschema:"description=Text to type for type_text."`
}

// InputTool 合成键盘输入（危险：默认应加入审批清单）。
type InputTool struct{}

// NewInputTool 创建 send_input 工具。
func NewInputTool() *InputTool { return &InputTool{} }

// Info 返回工具元数据。
func (t *InputTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "send_input",
		Desc: "Synthesize keyboard input: press a key (key_press) or type text (type_text). Use only with explicit user approval.",
	}, nil
}

// InvokableRun 执行键盘输入合成。
func (t *InputTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	var in InputInput
	if err := json.Unmarshal([]byte(argumentsJSON), &in); err != nil {
		return "", fmt.Errorf("parse send_input arguments: %w", err)
	}
	if platformInput == nil {
		return "", errInputUnsupported
	}

	switch in.Type {
	case "key_press":
		vk, ok := KeyVK(in.Key)
		if !ok {
			return "", fmt.Errorf("unknown key %q", in.Key)
		}
		if err := platformInput.KeyPress(vk); err != nil {
			return "", fmt.Errorf("key press: %w", err)
		}
		raw, _ := json.Marshal(map[string]any{"type": "key_press", "key": in.Key, "ok": true})
		return string(raw), nil
	case "type_text":
		if strings.TrimSpace(in.Text) == "" {
			return "", fmt.Errorf("text is required for type_text")
		}
		if err := platformInput.TypeText(in.Text); err != nil {
			return "", fmt.Errorf("type text: %w", err)
		}
		raw, _ := json.Marshal(map[string]any{"type": "type_text", "ok": true})
		return string(raw), nil
	default:
		return "", fmt.Errorf("unsupported input type %q", in.Type)
	}
}
