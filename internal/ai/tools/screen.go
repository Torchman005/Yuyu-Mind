package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

var errScreenUnsupported = errors.New("screen capture is not supported on this platform")

// screenCaptureExecutor 是屏幕截图的平台实现接口。
// 具体实例由 screen_windows.go / screen_other.go 通过包级变量 screenCapture 提供。
type screenCaptureExecutor interface {
	Capture() (*image.RGBA, error)
}

// ScreenCaptureInput 是 screen_capture 的参数。
type ScreenCaptureInput struct {
	Path string `json:"path" jsonschema:"description=Relative path under the workspace to save the PNG, e.g. screenshot.png. Empty means screenshot.png."`
}

// ScreenCaptureTool 截屏并保存为 PNG（危险：默认应加入审批清单）。
type ScreenCaptureTool struct {
	ws *Workspace
}

// NewScreenCaptureTool 创建 screen_capture 工具。
func NewScreenCaptureTool(ws *Workspace) *ScreenCaptureTool {
	return &ScreenCaptureTool{ws: ws}
}

// Info 返回工具元数据。
func (t *ScreenCaptureTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "screen_capture",
		Desc: "Capture the current screen and save it as a PNG inside the workspace. Use only with explicit user approval.",
	}, nil
}

// InvokableRun 截屏并保存为工作区内的 PNG 文件。
func (t *ScreenCaptureTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	var in ScreenCaptureInput
	if err := json.Unmarshal([]byte(argumentsJSON), &in); err != nil {
		return "", fmt.Errorf("parse screen_capture arguments: %w", err)
	}
	if screenCapture == nil {
		return "", errScreenUnsupported
	}

	img, err := screenCapture.Capture()
	if err != nil {
		return "", fmt.Errorf("capture screen: %w", err)
	}

	path := strings.TrimSpace(in.Path)
	if path == "" {
		path = "screenshot.png"
	}
	resolved, err := t.ws.Resolve(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", fmt.Errorf("create screenshot dir: %w", err)
	}
	file, err := os.Create(resolved)
	if err != nil {
		return "", fmt.Errorf("create screenshot file: %w", err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		return "", fmt.Errorf("encode screenshot: %w", err)
	}

	raw, err := json.Marshal(map[string]any{
		"path":   relPath(t.ws.Root(), resolved),
		"width":  img.Bounds().Dx(),
		"height": img.Bounds().Dy(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal screen_capture result: %w", err)
	}
	return string(raw), nil
}
