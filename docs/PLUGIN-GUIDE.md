# 插件开发指南

> 本文档说明如何为 Yuyu Mind 开发**进程内插件**。插件系统定位：让「可被大模型或用户调用的能力」以插件形式挂载，而不是堆在主流程里。

## 一、插件是什么

一个插件 = **元数据（Manifest）+ 能力（工具/动作）+ 生命周期（Init/Start/Stop）**。

- **工具（Tool）**：注册进宿主的工具注册表，供 **LLM**（Planner 或 Worker）自主调用。危险工具会受白名单 + 工作区 + 审批约束。
- **动作（Action）**：暴露给 **前端插件面板**，由用户手动触发（点击按钮 + 输入 JSON 参数）。

当前为**进程内插件**（编译进二进制，属可信代码）。第三方二进制热插拔（子进程 sidecar）是阶段 2，见文末路线图。

## 二、核心接口

定义在 `internal/plugin/plugin.go`：

```go
type Plugin interface {
    Manifest() Manifest
    Init(ctx context.Context, host *Host) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

### Manifest（元数据 + 权限 + 能力声明）

```go
type Manifest struct {
    SchemaVersion string         // 固定 "1.0"
    Name          string         // 唯一 ID（英文小写+数字），如 "workspace"
    DisplayName   string         // 显示名，如 "工作区文件"
    Description   string         // 一句话说明
    Version       string         // 语义化版本，如 "0.1.0"
    Author        string
    Entry         string         // "builtin"（内置）或未来 sidecar 的入口
    Permissions   []string       // 声明所需能力，如 "workspace.read"、"workspace.write"
    ConfigSchema  map[string]any // 可选：配置 JSON Schema
    Actions       []Action       // 声明可调用动作（供前端发现）
}
```

### Host（Init 期间注入的宿主服务）

```go
type Host struct {
    RegisterTool   func(name string, t tool.BaseTool) error
    RegisterAction func(name string, h ActionHandler) error
    Logf           func(format string, args ...any)
}
```

- `RegisterTool`：把 Eino 工具注册进宿主（同时进入 Planner 与 Worker 工具集）。
- `RegisterAction`：注册一个前端可调用的动作。
- `Logf`：统一日志。

`ActionHandler` 签名：`func(ctx context.Context, input map[string]any) (map[string]any, error)`。

## 三、完整示例

参考 `internal/plugin/builtin_system.go`（最小示例）与 `internal/plugin/workspace.go`（带权限与多动作的示例）。

最小骨架：

```go
package plugin

import (
    "context"
    "github.com/cloudwego/eino/components/tool"
)

type GreetPlugin struct{}

func NewGreetPlugin() *GreetPlugin { return &GreetPlugin{} }

func (p *GreetPlugin) Manifest() Manifest {
    return Manifest{
        SchemaVersion: "1.0",
        Name:          "greet",
        DisplayName:   "问候",
        Description:   "示例：返回问候语。",
        Version:       "0.1.0",
        Author:        "Yuyu Mind",
        Entry:         "builtin",
        Permissions:   []string{},
        Actions:       []Action{{Name: "hello", Description: "返回问候语。"}},
    }
}

func (p *GreetPlugin) Init(ctx context.Context, host *Host) error {
    return host.RegisterAction("hello", func(ctx context.Context, input map[string]any) (map[string]any, error) {
        return map[string]any{"greeting": "你好，我是 Yuyu！"}, nil
    })
}

func (p *GreetPlugin) Start(ctx context.Context) error { return nil }
func (p *GreetPlugin) Stop(ctx context.Context) error  { return nil }
```

## 四、挂载插件

在 `internal/app/app.go` 的 `Startup` 里：

```go
if err := a.pluginMgr.Register(ctx, plugin.NewGreetPlugin()); err != nil {
    slog.Error("failed to register greet plugin", "error", err)
}
```

需要依赖（如工作区）的插件，用构造函数注入：

```go
if ws != nil {
    _ = a.pluginMgr.Register(ctx, plugin.NewWorkspacePlugin(ws))
}
```

## 五、给插件注入「工具」（供 LLM 调用）

若插件要提供**LLM 可自主调用**的能力（如 PPT 生成、搜索），在 `Init` 里注册一个 Eino 工具：

```go
func (p *MyPlugin) Init(ctx context.Context, host *Host) error {
    return host.RegisterTool("generate_ppt", myPPTTool)
}
```

- 工具会被加入 Planner 工具集（同步聊天）与 Worker 工具集（异步任务）。
- Worker 侧：危险工具请把名字加入 `internal/agent/llm_executor.go` 的 `approvalRequiredTools`，这样执行前会走审批流。

## 六、约定与注意

1. **Name 唯一**：插件名是唯一 ID，重复注册会覆盖。
2. **动作是用户触发，工具是 LLM 触发**：动作不经过审批流（用户显式点击），工具若危险需审批。
3. **进程内插件属可信代码**：`Permissions` 目前是声明性元数据（供前端展示），真正的强制隔离留给 sidecar 阶段。
4. **副作用要可逆**：`Stop` 应清理 `Init`/`Start` 创建的 goroutine、定时器、连接等。
5. **前后端类型同步**：动作返回 `map[string]any`，Wails 会 JSON 序列化给前端；字段用 camelCase。

## 七、路线图

- **阶段 1（当前）**：进程内插件，`Plugin` 接口 + `Manager` + 内置插件（system / workspace）。
- **阶段 2**：子进程 sidecar（JSON-RPC over stdio），第三方插件以独立进程运行，崩溃隔离 + 权限强制。
- **阶段 3（可选）**：JS 脚本插件（goja），面向轻量扩展。
