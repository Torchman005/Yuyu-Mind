# AGENT.md — Yuyu Mind 开发日志

> 本文档是 AI 桌宠项目 **Yuyu Mind** 的开发上下文文件，供后续 Agent / 开发者快速对齐现状。
> 维护约定：每次开发完成后同步更新 `Comments`、`Rules`、`Done`，并把思路与难点沉淀到 [`docs/DEVELOPMENT-NOTES.md`](docs/DEVELOPMENT-NOTES.md)。

## 项目概览

- **目标**：让大模型操控 Live2D 桌面宠物，通过情绪化形象与用户对话、操控电脑工具、执行写代码/做 PPT 等任务、参与游戏（如 MC），并以**插件系统**构建生态。
- **技术栈**：Wails v2（Go 后端 + WebView 前端）· Go 1.23+ · CloudWeGo Eino（LLM 编排）· SQLite（modernc 纯 Go）· React 18 + TypeScript + Vite · PixiJS + pixi-live2d-display（Live2D）。
- **模块地图**：
  - `internal/app/` —— Wails 生命周期、前端可调方法、桌宠窗口（鼠标穿透）、Fish Audio TTS。
  - `internal/chat/` —— 聊天编排（Planner/Replyer 两段式）、TurnGate 回复门控、发送服务。
  - `internal/agent/` —— 异步任务系统（Submit/Claim/Run/Cancel/审批/补答）。
  - `internal/ai/` —— Provider 注册表、工具（calculator / web_search / 文件 / 命令）、回调。
  - `internal/memory/` —— 短期记忆窗口 + 长期 Memory Gateway（记忆、候选、摘要、任务上下文快照）。
  - `internal/db/` —— SQLite 仓储与迁移。
  - `internal/usage/` —— Token 用量追踪。
  - `frontend/` —— React 前端：`App.tsx`（聊天/语音/TTS/桌宠/主动发言）、`Live2DStage.tsx`（Live2D 渲染与情绪表现）。

## Comments（观察与结论）

> 当前代码分析结论，按重要程度排序。随开发推进持续修正。

1. **情绪驱动是「关键词启发式」，不是「LLM 结构化输出」**。后端 `companion.go:inferEmotion` 靠文本关键词猜 happy/focused/sad 等；前端 `App.tsx:inferAvatarPerformance` 再靠正则猜 mood/energy/手势。两者都脆弱，且情绪是在「完整回复」之后才确定的，无法在流式 TTS 播放开始前就驱动形象。→ 需要统一的**情绪 Schema**（emotion + intensity + mood + gesture），由 Planner/Replyer 产出，前端消费。
2. **插件系统目前只是空壳**。`companion.go:ListPlugins` 恒返回空列表，`PluginInfo` 类型已定义但无加载/权限/生命周期实现。这是「构建生态」的核心缺口。
3. **Worker 执行器是占位**。`internal/agent/executor.go:DefaultExecutor` 只校验任务包、不执行真实工作。写代码/做 PPT/操控电脑等能力都要挂到它下面。
4. **`frontend/dist` 缺失，导致 `go build .`（含 `//go:embed all:frontend/dist`）无法通过**；`go build ./internal/...` 理论上应通过。需要在 frontend 下 `npm run build` 生成 dist。
5. **Eino `compose` pipeline 已删除**。原 `internal/ai/pipeline` + `internal/ai/template` 是孤儿死代码（聊天服务直接用 Planner/Replyer），已于开发中移除并 `go mod tidy`，聊天走「Planner/Replyer + 内联 prompt」两段式。
6. **`App.tsx` 单文件 2291 行**，把聊天、语音识别、TTS 播放、流式音频拼接、唇同步、桌宠窗口、主动发言/追问全塞在一起，可维护性差，后续需拆分。
7. **桌宠透明窗口 + 鼠标穿透已用 Windows API 实现**（`pet_hit_windows.go`：WS_EX_TRANSPARENT + 轮廓命中检测），这是很有价值的已有资产，扩展屏幕观察/游戏操控时应复用其「非侵入窗口」思路。
8. **前后端事件通道已有基础**：聊天用 `chat:event`（Wails EventsEmit），TTS 流用 `mochi:speech:*` 事件；但异步任务的事件/审批尚未推到前端 UI。
9. **安全边界已有清晰设计**：审批流（`waiting_for_approval` / approve / reject）、任务上下文快照、Worker 不读长期记忆。这是接入「操控电脑」等高风险能力时必须坚守的骨架。
10. **构建环境限制**：本机 Go 的 `GOPATH/GOMODCACHE/GOCACHE` 默认在工作区之外，沙箱下不可写。构建/测试需把 `GOMODCACHE`/`GOCACHE`/`GOTMPDIR` 重定向到工作区内（见 Rules）。

## Rules（开发规则与约束）

### 架构边界（不可违背）

1. **顶层 Agent 是唯一直接接触用户的层**，负责理解意图、读写长期记忆、拆解任务。
2. **Worker Agent 不直接读用户长期记忆，也不直接向用户追问**；它只消费顶层下发的结构化任务包（`TaskSpec`）。
3. Worker 遇到缺信息/错误/需审批时，通过事件和控制消息回到顶层 Agent，而不是自己兜底。
4. **所有任务状态、事件、控制消息、操作日志必须持久化**到 SQLite。
5. `context.Context` 只用于运行中取消和超时传播，**不作为长期任务状态存储**。
6. **派发给 Worker 的记忆投影必须可审计**：每次 `BuildTaskContext` 保存快照（`task_context_snapshots`）。

### 代码与安全

7. 操控电脑的能力（文件写、命令执行、剪贴板、键鼠、屏幕截图）**必须经过审批流**，默认不允许静默执行高危动作。
8. 新增 Go 依赖要克制，优先复用已有库（Eino / Wails / modernc sqlite）。引入前先确认其 API 与 Go 1.23 兼容。
9. 前端保持 React + 纯 JS/TS（无额外 UI 框架），Live2D 表现统一通过 `Param*` 参数与 expression/motion 驱动，不直接 hack 模型内部。
10. 情绪在前后端流转使用**统一的情绪 Schema**，禁止各层各自发明字段名。

### 工程约定

11. 本机构建需重定向 Go 缓存到工作区（否则被沙箱拦截）：
    ```powershell
    $env:GOMODCACHE="<repo>\.gomodcache"; $env:GOCACHE="<repo>\.gocache"; $env:GOTMPDIR="<repo>\.gotmp"; $env:GOTELEMETRY="off"
    ```
    `<repo>\.gomodcache`、`.gocache`、`.gotmp` 已加入 `.gitignore`。
12. 全量构建顺序：先 `cd frontend && npm run build` 生成 `dist`，再 `go build .` 或 `make build`。
13. 每次开发完成必须同步更新本文件的 `Done`，并在 `docs/DEVELOPMENT-NOTES.md` 记录思路与难点解决办法。

## Done（已完成）

### 基础与后端

- [x] Wails 项目骨架、`main.go`/`app.go` 绑定层、窗口参数（1024×768，可切桌宠透明窗口）。
- [x] 配置系统：多 Provider（OpenAI/DeepSeek/Moonshot/Ollama）+ 激活切换 + 本地 `config.json` 持久化。
- [x] SQLite 仓储 + 迁移（会话/消息/设置/token 用量/异步任务/记忆）。
- [x] OpenAI 兼容 Provider 与 Ollama Provider（复用 OpenAI 适配器）。
- [x] Web Search 真实实现（`internal/ai/tools/web_search.go`：`SearchProvider` 接口 + DuckDuckGo 后端（免 Key）+ 结果解析；`web_search_test.go` 用 fake provider + 响应解析测试）。
- [x] Token 用量追踪（`TrackedChatModel` 包装 + 分会话/全局/供应商聚合）。
- [x] 聊天编排：Planner（决策 action）+ Replyer（生成可见回复）+ TurnGate 回复门控。
- [x] Planner 健壮性（`parsePlannerDecision`：JSON 解析失败重试一次 + 空 action 兜底 reply + 情绪归一化；`chat_test.go` 覆盖）。
- [x] 发送服务：回复清洗、长度上限、按句分片持久化。
- [x] 聊天编排单测（`internal/chat/chat_test.go`：JSON 解析/回复清洗/分片/门控/情绪归一化等 9 个测试）。
- [x] 短期记忆：会话历史滑动窗口（`memory/window.go`）。
- [x] 长期 Memory Gateway：记忆 Upsert/Search/Archive、候选记忆 promote/reject、会话摘要、任务上下文投影 + 快照。
- [x] 记忆模块单测（`internal/memory/memory_test.go`：滑动窗口/角色映射/工具调用往返/SQLiteStore 往返 4 个测试）。
- [x] 异步任务系统：Submit/Claim/Run/Cancel、`waiting_for_input`/`waiting_for_approval`、worker slot 并发、事件与控制消息。

### 前端与形象

- [x] Live2D 渲染（pixi-live2d-display，Cubism 5 bridge 可选） + 模型适配/自适应缩放。
- [x] 情绪表达：expression 映射 + 自然临场（视线、眨眼、微动）+ 手势（bounce/tilt/lean/…）。
- [x] 唇同步：Web Audio Analyser 驱动 `ParamMouthOpenY`。
- [x] 桌宠模式：透明窗口、鼠标穿透（Windows WS_EX_TRANSPARENT + 轮廓命中）、滚轮缩放、Ctrl+Shift+M 切换。
- [x] 语音：Fish Audio TTS（buffered + 流式事件）、系统 TTS 兜底、浏览器 ASR、语音门控、打断（barge-in）、连续/自由对话、主动发言与追问。

### 情绪管线（M1）

- [x] 统一情绪 Schema（`internal/chat/emotion.go`：emotion/mood/gesture/hand 白名单 + energy 钳制）。
- [x] Planner 结构化产出情绪（`PlannerDecision` 增加 emotion/mood/energy/gesture/hand + 提示词 + 归一化）。
- [x] 情绪经 `ChatEvent(EventTypeEmotion)` → `collectingEmitter` → `ChatReply.Emotion` 端到端流转，`SendMessage` 优先用 LLM 情绪、回退 `inferEmotion`。
- [x] `ChatReply`/`CompanionMessage` 扩展 mood/energy/gesture/hand 字段，`SendMessage` 填充完整表演参数。
- [x] 前端 `App.tsx` 用 LLM 表演参数（mood/energy/hand）覆盖启发式 `inferAvatarPerformance`，并回退兜底（代码已写，需本地 `npm run build` 验证）。
- [x] 情绪持久化到 messages 表（`emotion/mood/energy/gesture/hand` 列 + 迁移 + `MessageRepo` 读写 + `SendGuidedReply` 回填 + `companionMessages` 读取回退启发式 + 单测）。

### 插件系统（M2 内核）

- [x] 插件接口与契约（`internal/plugin/plugin.go`：`Plugin`/`Manifest`/`Action`/`Host`）。
- [x] 插件管理器（`internal/plugin/manager.go`：注册/启用/停用/列表/动作派发/停用全部）。
- [x] 内置示例插件（`internal/plugin/builtin_system.go`：`system` 插件，`ping`/`version` 动作）。
- [x] 内置工作区插件（`internal/plugin/workspace.go`：`workspace` 插件，`list`/`read`/`write` 动作，复用工作区路径隔离；`workspace_test.go` 验证读写列往返 + 越界拒绝）。
- [x] 插件工具同时进入 Planner 与 Worker 工具集（`app.go` 用 `workerToolReg` 注册表承载 Worker 工具，执行器改为 `toolProvider` 动态读取，插件 `RegisterTool` 双写）。
- [x] 宿主接线（`app.go`）：插件工具注册进工具注册表、挂载内置插件（system + workspace）、Shutdown 时 StopAll。
- [x] Wails 方法：`ListPlugins`（真实数据）/`EnablePlugin`/`DisablePlugin`/`InvokePluginAction`（`internal/app/plugin_service.go`）。
- [x] 单元测试（`internal/plugin/manager_test.go`：生命周期 + 动作派发 + 禁用/启用 + 校验；`workspace_test.go`）。
- [x] 前端插件面板（`App.tsx` 插件列表/启停/调用动作 + JSON 参数输入 + `App.css` 样式；`tsc --noEmit` 通过）。
- [x] wailsjs 绑定补全（`App.js`/`App.d.ts` 新增 `EnablePlugin`/`DisablePlugin`/`InvokePluginAction`，`models.ts` 新增情绪表演字段）。
- [ ] 插件配置持久化（当前 `Context`/`Config` 未接）。
- [ ] 子进程 sidecar 协议（第三方二进制插件，阶段 2）。

### 电脑工具（M3 地基）

- [x] 工作区安全边界（`internal/ai/tools/workspace.go`：路径 containment + 符号链接逃逸拦截）。
- [x] 文件系统工具（`internal/ai/tools/filesystem.go`：`list_files`/`read_file`/`write_file`）。
- [x] 只读工具接入 Planner（`list_files`/`read_file` 已注册；`write_file` 仅 Worker 使用，未注册进同步工具集）。
- [x] 配置新增 `App.WorkspaceRoot`（默认回退用户主目录）。
- [x] 单元测试（`internal/ai/tools/filesystem_test.go`：越界/软链逃逸拒绝 + 读写列往返）。
- [x] Worker 真实执行器（`internal/agent/llm_executor.go`：LLM 工具循环 + `allowed_actions` 白名单过滤 + 事件/操作日志；`internal/app/worker_executor.go` 模型适配；`app.go` 接入 Worker 工具集含 `write_file`；`llm_executor_test.go` 4 个单测通过）。
- [x] Worker 审批流（`Runtime.RequestApproval` + `waiting_for_approval` 状态机 + approve/reject/cancel 消费；`llm_executor.go` 对 `write_file` 等危险工具先审批；`approval_test.go` 验证提交→挂起→批准→完成全链路）。
- [x] 命令执行工具（`internal/ai/tools/command.go`：工作区目录内执行 + 超时 + 输出截断；`execute_command` 加入 Worker 工具集与 `approvalRequiredTools`；`command_test.go` 实测 exec 通过）。
- [ ] 剪贴板 / 屏幕截图工具（Windows 特定，后续 M5 屏幕观察一并做）。

### 任务闭环（M4）

- [x] Planner `task` 动作 + `TaskPlan` 任务包（`agents.go` 增加 `Task` 字段 + 提示词 + `ToTaskSpec` 转换，`task_plan.go`）。
- [x] 聊天→任务提交（`chat.Service` 注入 `TaskSubmitter`，`service.go` 处理 `task` 动作并提交 + 让 Replyer 确认）。
- [x] 宿主适配（`internal/app/task_submitter.go`：填充默认工作区 + 安全默认只读动作；`app.go` 接线）。
- [x] 任务事件回传（`agent.Service` 注入 `Notifier`，`service.go`/`runtime.go` 在事件与状态变更时推送；`internal/app/task_submitter.go` 的 `taskNotifier` 经 Wails `EventsEmit("agent:task:changed")` 推送）。
- [x] 前端任务面板（`App.tsx` 任务列表 + 状态徽标 + 取消/批准/拒绝/补充回答 + 事件订阅刷新 + `App.css` 样式；`tsc --noEmit` 通过）。
- [x] 单元测试（`task_plan_test.go`：TaskPlan→TaskSpec 转换；`notifier_test.go`：任务生命周期触发通知）。

### 文档

- [x] `README.md`（架构说明）。
- [x] `AGENT.md`（本文档，Comments/Rules/Done）。
- [x] `docs/DEVELOPMENT-NOTES.md`（思路与难点解决办法）。
- [x] `docs/PLUGIN-GUIDE.md`（插件开发指南：接口/示例/挂载/约定/路线图）。

## Next（待办 / 路线图）

> ✅ **前端类型已验证**：`tsc --noEmit` 通过（exit 0），前端 TSX/绑定/models 改动类型正确。剩余只需本地 `npm install`（完整）+ `npm run build` 生成 `dist`（沙箱内 esbuild `spawn EPERM` 无法打包）+ `wails dev` 启动。Go 后端已全程 `go build`/`go test` 通过。

- [ ] **M2 插件增强**：插件配置持久化 + sidecar 协议（阶段 2）。
- [ ] **M3 电脑工具（续）**：剪贴板 / 屏幕截图工具（Windows 特定，与 M5 屏幕观察合并做）。
- [ ] **M5 屏幕观察与游戏**：屏幕截图 + 视觉模型观察；键鼠合成（高风险，需审批）。
- [ ] **M6 生态**：插件市场/示例插件/文档。
- [ ] 生成 `frontend/dist`，打通全量 `go build .` / `make build`。
- [ ] 拆分 `App.tsx`。
