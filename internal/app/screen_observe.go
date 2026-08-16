package app

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/yuyu-mind/backend/internal/ai/tools"
)

var errWorkspaceUnavailable = errors.New("workspace is not available")

// captureScreenshot 截屏并保存到工作区 screenshots/ 目录，返回相对路径。
func (a *App) captureScreenshot() (string, error) {
	if a.workspace == nil {
		return "", errWorkspaceUnavailable
	}
	img, err := tools.CaptureScreen()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(a.workspace.Root(), "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := time.Now().Format("20060102-150405") + ".png"
	path := filepath.Join(dir, name)

	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		return "", err
	}

	rel, _ := filepath.Rel(a.workspace.Root(), path)
	return filepath.ToSlash(rel), nil
}
