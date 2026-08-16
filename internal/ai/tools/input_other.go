//go:build !windows

package tools

// platformInput 在非 Windows 平台为 no-op（不支持键鼠合成）。
var platformInput platformInputExecutor = &noopInput{}

type noopInput struct{}

func (n *noopInput) KeyPress(vk uint16) error   { return errInputUnsupported }
func (n *noopInput) TypeText(text string) error { return errInputUnsupported }
