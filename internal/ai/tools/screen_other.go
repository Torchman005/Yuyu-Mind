//go:build !windows

package tools

import "image"

// screenCapture 在非 Windows 平台为 no-op（不支持截图）。
var screenCapture screenCaptureExecutor = &noopScreenCapture{}

type noopScreenCapture struct{}

func (n *noopScreenCapture) Capture() (*image.RGBA, error) { return nil, errScreenUnsupported }
