package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// 本文件实现受限在 Workspace 内的文件系统工具。
// list_files / read_file 为只读，可注册进 Planner；write_file 为写入，
// 默认只应暴露给 Worker（需审批流），不要注册进同步 Planner 工具集。

const (
	defaultMaxEntries = 200
	defaultMaxBytes   = 64 * 1024
)

// ---- list_files ----

// ListFilesInput 是 list_files 的参数。
type ListFilesInput struct {
	Path       string `json:"path" jsonschema:"description=Relative directory path under the workspace. Empty means workspace root."`
	Recursive  bool   `json:"recursive" jsonschema:"description=Whether to list recursively."`
	MaxEntries int    `json:"max_entries" jsonschema:"description=Maximum number of entries to return, default 200."`
}

// ListFilesTool 列出工作区内的文件与目录。
type ListFilesTool struct{ ws *Workspace }

// NewListFilesTool 创建 list_files 工具。
func NewListFilesTool(ws *Workspace) *ListFilesTool { return &ListFilesTool{ws: ws} }

// Info 返回工具元数据。
func (t *ListFilesTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_files",
		Desc: "List files and directories inside the workspace. Use this to explore the project structure before reading or writing files.",
	}, nil
}

// InvokableRun 执行目录列举。
func (t *ListFilesTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	var in ListFilesInput
	if err := json.Unmarshal([]byte(argumentsJSON), &in); err != nil {
		return "", fmt.Errorf("parse list_files arguments: %w", err)
	}
	// 空路径表示列出工作区根目录。
	dir := t.ws.Root()
	if strings.TrimSpace(in.Path) != "" {
		resolved, err := t.ws.Resolve(in.Path)
		if err != nil {
			return "", err
		}
		dir = resolved
	}
	if in.MaxEntries <= 0 {
		in.MaxEntries = defaultMaxEntries
	}

	type entry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
		Size  int64  `json:"size,omitempty"`
	}

	entries := make([]entry, 0, in.MaxEntries)
	if in.Recursive {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if p == dir {
				return nil
			}
			if len(entries) >= in.MaxEntries {
				return fs.SkipAll
			}
			info, infoErr := d.Info()
			size := int64(0)
			if infoErr == nil {
				size = info.Size()
			}
			entries = append(entries, entry{
				Name:  d.Name(),
				Path:  relPath(t.ws.Root(), p),
				IsDir: d.IsDir(),
				Size:  size,
			})
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("walk directory: %w", err)
		}
	} else {
		items, readErr := os.ReadDir(dir)
		if readErr != nil {
			return "", fmt.Errorf("read directory: %w", readErr)
		}
		for _, item := range items {
			if len(entries) >= in.MaxEntries {
				break
			}
			info, infoErr := item.Info()
			size := int64(0)
			if infoErr == nil {
				size = info.Size()
			}
			entries = append(entries, entry{
				Name:  item.Name(),
				Path:  relPath(t.ws.Root(), filepath.Join(dir, item.Name())),
				IsDir: item.IsDir(),
				Size:  size,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal list_files result: %w", err)
	}
	return string(data), nil
}

// ---- read_file ----

// ReadFileInput 是 read_file 的参数。
type ReadFileInput struct {
	Path     string `json:"path" jsonschema:"description=Relative file path under the workspace."`
	MaxBytes int    `json:"max_bytes" jsonschema:"description=Maximum bytes to read, default 65536."`
}

// ReadFileTool 读取工作区内的文件内容。
type ReadFileTool struct{ ws *Workspace }

// NewReadFileTool 创建 read_file 工具。
func NewReadFileTool(ws *Workspace) *ReadFileTool { return &ReadFileTool{ws: ws} }

// Info 返回工具元数据。
func (t *ReadFileTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_file",
		Desc: "Read the text content of a file inside the workspace. Returns the file content, truncated if it exceeds max_bytes.",
	}, nil
}

// InvokableRun 执行文件读取。
func (t *ReadFileTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	var in ReadFileInput
	if err := json.Unmarshal([]byte(argumentsJSON), &in); err != nil {
		return "", fmt.Errorf("parse read_file arguments: %w", err)
	}
	path, err := t.ws.Resolve(in.Path)
	if err != nil {
		return "", err
	}
	if in.MaxBytes <= 0 {
		in.MaxBytes = defaultMaxBytes
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path %q is a directory, not a file", in.Path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	truncated := false
	if len(data) > in.MaxBytes {
		data = data[:in.MaxBytes]
		truncated = true
	}

	result := struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		Truncated bool   `json:"truncated,omitempty"`
		Size      int64  `json:"size"`
	}{
		Path:      relPath(t.ws.Root(), path),
		Content:   string(data),
		Truncated: truncated,
		Size:      info.Size(),
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal read_file result: %w", err)
	}
	return string(raw), nil
}

// ---- write_file ----

// WriteFileInput 是 write_file 的参数。
type WriteFileInput struct {
	Path    string `json:"path" jsonschema:"description=Relative file path under the workspace."`
	Content string `json:"content" jsonschema:"description=The complete text content to write."`
}

// WriteFileTool 写入工作区内的文件（危险：默认只供 Worker 使用）。
type WriteFileTool struct{ ws *Workspace }

// NewWriteFileTool 创建 write_file 工具。
func NewWriteFileTool(ws *Workspace) *WriteFileTool { return &WriteFileTool{ws: ws} }

// Info 返回工具元数据。
func (t *WriteFileTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "write_file",
		Desc: "Write text content to a file inside the workspace, creating parent directories as needed. Overwrites existing files. Use only with explicit user approval.",
	}, nil
}

// InvokableRun 执行文件写入。
func (t *WriteFileTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	var in WriteFileInput
	if err := json.Unmarshal([]byte(argumentsJSON), &in); err != nil {
		return "", fmt.Errorf("parse write_file arguments: %w", err)
	}
	path, err := t.ws.Resolve(in.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create parent directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	result := struct {
		Path    string `json:"path"`
		Bytes   int    `json:"bytes"`
		Written bool   `json:"written"`
	}{
		Path:    relPath(t.ws.Root(), path),
		Bytes:   len(in.Content),
		Written: true,
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal write_file result: %w", err)
	}
	return string(raw), nil
}

// relPath 把绝对路径转换为相对 root 的路径。
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(strings.TrimPrefix(path, root))
	}
	return filepath.ToSlash(rel)
}
