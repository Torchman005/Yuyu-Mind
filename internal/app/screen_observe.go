package app

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuyu-mind/backend/internal/ai/tools"
	"github.com/yuyu-mind/backend/internal/ai/vision"
)

var errWorkspaceUnavailable = errors.New("workspace is not available")

// observeScreenText 截屏并（若配置了视觉模型）描述画面，返回回复内容与情绪。
func (a *App) observeScreenText(prompt string) (string, string, error) {
	path, err := a.captureScreenshot()
	if err != nil {
		return "", "", err
	}

	content := fmt.Sprintf("已截屏保存到工作区 %s。", path)
	emotion := "focused"

	model := strings.TrimSpace(a.cfg.Vision.Model)
	if model == "" {
		content += " 视觉模型尚未配置，暂时无法自动描述画面内容。"
		return content, emotion, nil
	}

	imgPath := filepath.Join(a.workspace.Root(), filepath.FromSlash(path))
	pngBytes, err := os.ReadFile(imgPath)
	if err != nil {
		return content + " 视觉模型读取截图失败。", emotion, nil
	}

	providerCfg, _ := a.cfg.GetActiveProviderConfig()
	query := strings.TrimSpace(prompt)
	if query == "" {
		query = "请用简洁的中文描述当前屏幕内容。"
	}
	description, err := vision.Describe(a.ctx, providerCfg.BaseURL, providerCfg.APIKey, model, query, pngBytes)
	if err != nil {
		return content + fmt.Sprintf(" 视觉模型调用失败：%v。", err), emotion, nil
	}
	return "屏幕观察：" + description, "thinking", nil
}

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
