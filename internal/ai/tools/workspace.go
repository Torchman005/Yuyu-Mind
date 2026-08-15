package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace 是文件工具的安全边界：所有路径都被约束在 root 之内。
// 这是后续「写代码 / 做 PPT / 操控文件」等能力的通用安全原语。
type Workspace struct {
	root string // 绝对路径、已 Clean
}

// NewWorkspace 创建文件工具工作区。root 为空时使用当前工作目录。
func NewWorkspace(root string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve workspace root: %w", err)
		}
		root = wd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs workspace root: %w", err)
	}
	abs = filepath.Clean(abs)
	// 若根目录已存在，解析其符号链接，保证后续 containment 判断基于真实路径。
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return &Workspace{root: abs}, nil
}

// Root 返回工作区根路径。
func (w *Workspace) Root() string { return w.root }

// Resolve 把相对或绝对路径约束到工作区内，返回规范化的绝对路径。
// 越界（含 `..` 逃逸、绝对路径逃逸、已存在符号链接逃逸）一律拒绝。
func (w *Workspace) Resolve(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is empty")
	}

	p := path
	if !filepath.IsAbs(p) {
		p = filepath.Join(w.root, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("abs path: %w", err)
	}
	clean := filepath.Clean(abs)

	// 第一道：词法 containment，拦截 `..` 与绝对路径逃逸。
	if !withinRoot(w.root, clean) {
		return "", fmt.Errorf("path %q escapes workspace root", path)
	}

	// 第二道：若路径已存在，解析符号链接后再做一次 containment，拦截软链逃逸。
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		if !withinRoot(w.root, resolved) {
			return "", fmt.Errorf("path %q escapes workspace root via symlink", path)
		}
		return resolved, nil
	}

	// 目标不存在（例如写新文件）时，对最深已存在祖先解析符号链接。
	resolved, err := resolveExistingAncestor(clean)
	if err != nil {
		return "", err
	}
	if !withinRoot(w.root, resolved) {
		return "", fmt.Errorf("path %q escapes workspace root via parent symlink", path)
	}
	return resolved, nil
}

// withinRoot 判断 p 是否在 root 内（词法）。
func withinRoot(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveExistingAncestor 向上找到第一个存在的祖先目录，解析其符号链接，
// 再把剩余不存在的部分拼接回去，得到「真实祖先 + 新路径」的近似解析。
func resolveExistingAncestor(path string) (string, error) {
	clean := filepath.Clean(path)
	existing := clean
	var suffix []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return clean, nil
		}
		suffix = append([]string{filepath.Base(existing)}, suffix...)
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return clean, nil
	}
	for _, part := range suffix {
		resolved = filepath.Join(resolved, part)
	}
	return filepath.Clean(resolved), nil
}
