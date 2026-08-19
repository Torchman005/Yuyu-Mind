# 开发思路与难点解决办法

> 本文档记录 Yuyu Mind 从 0 到 1 的**整体思路**、遇到的**难点**以及对应的**解决办法**。
> 与 [`AGENT.md`](../AGENT.md) 配套：`AGENT.md` 记录「现状/规则/完成」，本文件记录「为什么这么做 + 怎么解决」。
> 迭代约定：每解决或新遇一个难点，就在对应条目更新状态与方案。

## 一、整体思路

### 1.1 分层 Agent（顶层 / Worker）

- **顶层 Agent** 是唯一接触用户的层：理解意图、读写长期记忆、拆解任务、下发生成任务包。
- **Worker Agent** 只消费结构化任务包（`TaskSpec`），不读长期记忆、不直接追问用户；缺信息/需审批时通过事件与控制消息回传。

> 为什么：把「懂用户」和「干活的」分离，既保护记忆隐私（Worker 只看投影），又让任务可审计、可取消、可审批。这是后续接入「操控电脑」「写代码」等高风险能力的安全骨架。

### 1.2 情绪 → 表现管线

目标链路：`用户输入 → Planner(决策+情绪) → Replyer(文本+情绪) → 前端 Live2D(表情/参数/手势) + TTS`。

- 情绪必须在**流式 TTS 开始前**确定，因此由 Planner/Replyer 结构化产出，而不是等完整回复后再猜。
- 前端 `Live2DStage` 只负责「把情绪 Schema 翻译成 `Param*` 参数 / expression / motion」。

### 1.3 插件生态

先定义**稳定接口 + 权限模型 + 生命周期**，再谈分发。分阶段落地（见难点 2）。

### 1.4 安全边界

所有副作用（文件写、命令、剪贴板、键鼠、截图）都必须可审批、可审计、可撤销。审批复用异步任务已有的 `waiting_for_approval` 状态机。

---

## 二、难点与解决办法

> 状态标记：✅ 已解决 · 🔶 进行中 · ⬜ 待解决

### 难点 1：LLM 情绪如何可靠地驱动 Live2D —— ✅ 已解决（M1 离散 + v2 连续 VAD/FACS-AU）

**难点**：现有实现是「关键词启发式」——后端 `inferEmotion` 猜 happy/focused/sad，前端 `inferAvatarPerformance` 再用正则猜 mood/energy/手势。问题：① 覆盖不全、误判多；② 情绪在完整回复后才确定，无法在 TTS 播放开始前就驱动形象；③ 前后端各有一套字段，无统一契约。

**解决办法**：

1. 定义统一情绪 Schema（后端与前端共享同一份契约）：
   ```jsonc
   {
     "emotion": "happy|focused|thinking|sad|surprised|neutral",
     "mood": "calm|cheer|curious|confident|comfort|surprised|playful",
     "energy": 0.0,             // 0~1
     "gesture": "none|bounce|tilt|lean|playfulSway|surprisePop|comfortNod",
     "hand": "none|left|right|both"
   }
   ```
2. **Planner** 在决策时一并产出情绪（它已看到 gate/pending/history，是最早能判断情绪的阶段）；**Replyer** 只产出可见文本，避免污染 TTS 文本。
3. 情绪经事件通道端到端流转：`PlannerDecision` → `ChatEvent(EventTypeEmotion)` → `collectingEmitter` → `ChatReply.Emotion`；`SendMessage` 优先 LLM 情绪、回退 `inferEmotion`。
4. 前端 `Live2DStage` 消费该 Schema，映射到已有的 `applyPixiEmotion` + `applyPerformancePresence` 参数管线（M1.5）。

> ✅ **已落地（M1 + M1.5 + 持久化）**：`internal/chat/emotion.go` 定义白名单与归一化；`PlannerDecision` 增加 emotion/mood/energy/gesture/hand；`ChatEvent` 增加 `EventTypeEmotion`；`companion.go` 收集并回填到 `ChatReply`/`CompanionMessage`；messages 表新增 emotion/mood/energy/gesture/hand 列并随 `SendGuidedReply` 持久化、`companionMessages` 读取（空则回退 `inferEmotion`）；前端 `App.tsx` 用 LLM mood/energy/hand 覆盖 `inferAvatarPerformance`（兜底保留）。`go build`/`go test ./internal/db` 通过；前端改动需本地 `npm run build` 验证。
> ⬜ **待办**：gesture 字段当前仅随 DTO 传递、尚未覆盖 `Live2DStage` 内部由 mood 推导的手势（可选优化）。

> 🎯 **参考方向（情绪系统 v2）：[soullink-emotion-sdk](https://github.com/nanlingyin/soullink-emotion-sdk)**（framework-agnostic 实时 Live2D 表情/动作 SDK）。其四大支柱与我们的现状对照：
>
> | SDK 支柱 | 含义 | 我们现状 | 差距 → 演进 |
> | --- | --- | --- | --- |
> | **Continuous VAD emotion** | 连续效价(valence)/唤醒(arousal)/支配(dominance) 情绪，实时推断 | 离散 `emotion` 字符串 + 单一 `energy`(≈arousal) | 后端 Schema 增加连续 `valence`/`arousal`，来源从「关键词/LLM 离散」走向「文本+语音连续」 |
> | **FACS/AU synthesis** | 动作单元 AU1(内眉上扬)/AU4(皱眉)/AU6(脸颊提升)/AU12(嘴角上拉)/AU25(双唇分开)… 合成表情 | `performanceTargets` 把离散 mood → `ParamBrowY/ParamEyeSmile/ParamMouthForm` 手调权重 | 改为 **AU 表驱动**：AU 权重 → Live2D 参数，原则化、可复用 |
> | **Layered animation** | idle + 情绪 + 手势分层、权重混合 | 已有雏形：`applyPresence`(idle/眨眼/呼吸) + `applyPerformancePresence`(情绪) + gesture + `applyLipSync` | 基本对齐，主要缺「连续权重的平滑混合」 |
> | **Automatic model adaptation** | 参数注册表自动适配任意 Live2D 模型的参数/表情命名 | 参数名硬编码当前模型（`ParamEyeSmile`、`ParamBrowY`…） | 引入 manifest/参数注册表，运行时按模型名自动匹配 |
>
> 结论：我们已踩中「layered + 参数映射」的前半程，v2 的实质是**离散 → 连续（VAD）**、**手调 → AU 表**、**硬编码 → 注册表适配**三个方向。

> ✅ **v2 已落地（连续 VAD + FACS/AU + 参数注册表）**：
> - **后端 VAD**：`emotion.go` 新增 `ClampValence`/`ClampDominance`（`energy`≡arousal 0..1）；`PlannerDecision`/`EmotionInfo`/`ChatEvent` 增加 `valence`(-1..1)/`dominance`(-1..1)；Planner 提示词要求连续输出；`parsePlannerDecision` 钳制归一化；端到端流转（`service.go` → `collectingEmitter` → `ChatReply`/`CompanionMessage` → `SendMessage` 回填，`inferValence`/`inferDominance` 兜底）；messages 表新增 `valence`/`dominance` 列并持久化。
> - **前端 AU 合成**：新增 `frontend/src/components/emotionEngine.ts`——`computeAUWeights`（离散 emotion/mood 基线 + 连续 VAD 调制 → AU1/AU4/AU6/AU12/AU15… 权重）+ `computeExpressionTargets`（AU → 逻辑参数净目标：eyeSmile/browRaise/browForm/mouthCorner）+ `applyExpressionTargets`；`Live2DStage.tsx` 用 AU 驱动微笑/眉/嘴型，替换原 `moodBoost` 手调权重，且不触碰眨眼/唇同步/expression 层（避免参数竞争）。
> - **参数注册表（自动适配）**：`PARAM_REGISTRY` 逻辑参数 → 候选 Live2D 参数 ID（当前模型在前、常见替代名在后），运行时按 `coreModel.getParameterIndex` 匹配第一个存在的 ID。
> - **验证**：`go build ./...` + `go test ./internal/...`（10 包全绿）+ `tsc --noEmit`（exit 0）。前端渲染效果需本地 `npm run build` 验证。
> ⬜ **待办（v2.1）**：语音 VAD 实时推断（音频连续情绪）；直接接入 `@soullink-emotion/live2d-pixi` 替换自研 `computeExpressionTargets`。

### 难点 2：插件系统的实现路线 —— 🔶 进行中（内核 + 前端已完成，权限/sidecar 待做）

**难点**：Go 官方 `plugin` 包在 Windows 上几乎不可用（且要求编译期类型一致）；「构建生态」又需要第三方能扩展能力；同时要解决沙箱、权限、生命周期、热更新、崩溃隔离。

**解决办法（分阶段，先内后外）**：

- **阶段 1（内置插件，优先）**：定义进程内接口 `Plugin`（`Manifest()/Init(ctx,host)/Start()/Stop()`），用 `Manager` 注册并管理生命周期。权限用 `Permissions []string` 声明，`Manifest.Actions` 声明可调用动作，`Init` 里通过 `Host.RegisterAction/RegisterTool` 注入能力。这先打通 `ListPlugins`/启停/动作派发/工具注入，风险最低。
- **阶段 2（外部 sidecar）**：第三方插件以独立进程运行，通过 **JSON-RPC over stdio** 与宿主通信，崩溃不拖垮宿主。宿主持有「能力白名单」（暴露哪些工具/事件给插件），插件持权最小化。
- **阶段 3（可选，脚本生态）**：引入 JS 引擎（goja）让轻量插件用 JS 写，适合快速生态增长，但沙箱能力弱，需谨慎。

> 关键取舍：先牺牲「第三方二进制热插拔」，换取「稳定接口 + 权限模型 + 生命周期」的正确性；sidecar 协议在阶段 2 再补齐。
>
> ✅ **已落地（M2 内核 + 前端）**：`internal/plugin` 包（`Plugin`/`Manifest`/`Action`/`Host` 接口 + `Manager` 注册/启停/列表/动作派发 + `system` 内置插件）；`app.go` 接线（工具注册进宿主工具表 + Shutdown StopAll）；Wails 方法 `ListPlugins`/`EnablePlugin`/`DisablePlugin`/`InvokePluginAction`；前端 `App.tsx` 插件面板（列表/启停/调用动作）+ `App.css` 样式 + wailsjs 绑定补全；`manager_test.go` 单元测试通过。`go build`/`go vet`/`go test ./internal/plugin` 均通过；前端改动需本地 `npm run build` 验证。
> ⬜ **待办**：插件配置持久化、动作级权限强制、sidecar 协议。

### 难点 3：操控电脑的安全边界 —— 🔶 进行中（M3 地基已落地）

**难点**：写文件/执行命令/键鼠/截图都是高风险副作用，误操作不可逆；Windows 下 API 多样；需要用户可感知、可中断。

**解决办法**：

1. 复用异步任务已有的审批状态机：高危工具先 `waiting_for_approval`，顶层 Agent 弹给用户 approve/reject，`allowed_actions` 白名单限制每个任务能调用的工具。
2. 每个副作用写 `agent_operation_logs`（kind/target/summary/status），保证可审计。
3. 工具按「危险等级」分级：只读（list/read/search）默认放行；写入/命令执行默认审批；键鼠/截图默认审批且带超时。
4. 工作区（workspace）默认限制在用户指定目录，越界路径一律拒绝。

> ✅ **已落地（M3 地基）**：`internal/ai/tools/workspace.go` 实现工作区 containment（词法 `..`/绝对路径逃逸 + 已存在/父目录符号链接逃逸拦截）；`filesystem.go` 实现 `list_files`/`read_file`/`write_file`；只读工具注册进 Planner，`write_file` 保留给 Worker（审批流）；`App.WorkspaceRoot` 可配置（默认用户主目录）。`filesystem_test.go` 验证越界/软链逃逸拒绝与读写列往返。`go build`/`go test` 通过。
> ⬜ **待办**：命令执行 / 剪贴板 / 截图工具；Worker 真实执行器接入写工具 + 审批流。

### 难点 4：流式对话 + 情绪 + 唇同步的时序 —— 🔶 部分解决

**难点**：LLM token 流、完整情绪、TTS 播放三者时序不一致；情绪若等完整回复才出，就会「嘴先动、表情后到」。

**解决办法**：

- 采用**两段式**（Planner 先定行为与情绪方向，Replyer 再定文本），情绪在 `Generate` 一次性返回，早于前端播放。
- 唇同步不依赖 LLM，而是用 Web Audio `AnalyserNode` 实时算 `mouthLevel`（已实现），与情绪解耦。
- 打断/换轮用 `playbackId` 世代计数（已实现），旧音频回调直接作废，避免串台。

### 难点 5：桌宠透明窗口（原生透明 + 鼠标穿透）—— ✅ 已解决

**难点**：WebView 整窗是矩形，但桌宠只有形象轮廓可点击，其余区域要「点穿」到桌面；同时桌宠模式要求「只显示 Live2D 形象、其余全透明」，但窗口背景却是黑色。

**解决办法（两层，缺一不可）**：

1. **原生窗口透明（消除黑底）**：Wails v2 在 Windows 下默认 `Windows.WebviewIsTransparent=false` 且不启用窗口透传。前端 CSS 透明 + `WindowSetBackgroundColour(0,0,0,0)` 虽让 WebView2 的 `DefaultBackgroundColor.A=0`（内容透明），但 `win32.SetBackgroundColour(hwnd, 0,0,0)` 会把原生 HWND 的背景刷子刷成**黑色**，透出透明 WebView。修复：`main.go` 设置 `Windows: &windows.Options{WebviewIsTransparent: true, WindowIsTranslucent: true, BackdropType: windows.None}`——Win11 22621+ 走 `DWMSBT_NONE`（无材质、全透明），Win10 回退 `ACCENT_ENABLE_BLURBEHIND`（毛玻璃，非全透明）。
2. **鼠标穿透（轮廓命中）**：Windows 下用 `WS_EX_LAYERED + WS_EX_TRANSPARENT` 切换穿透，`GetCursorPos`/`GetWindowRect` + 轮廓命中函数 `petContourBand` 判断鼠标是否落在形象内（已实现，见 `internal/app/pet_hit_windows.go`）。非 Windows 用 no-op 兜底（`pet_hit_other.go`）。

### 难点 6：Eino `compose` API 适配 —— ✅ 已解决（删除孤儿）

**难点**：`internal/ai/pipeline/*.go` 使用 `compose.NewChain/AppendGraph/AppendChatModel`，与依赖的 Eino v0.9.9 API 可能不一致；且聊天服务已绕过 pipeline（直接 Planner/Replyer），pipeline 成为孤儿代码。

**解决办法**：采纳方案 ②——**删除孤儿 `internal/ai/pipeline` 与 `internal/ai/template` 包**（它们只互相引用，无任何业务代码导入），并 `go mod tidy` 清理依赖。聊天保持「Planner/Replyer + 内联 prompt」两段式，真实 Worker 执行器已用 `model.ToolCallingChatModel` 直接实现工具循环（不经 compose）。`go build`/`go test ./internal/...` 通过。

### 难点 7：构建环境（frontend/dist + Go 缓存）—— 🔶 部分解决

**难点**：`main.go` 用 `//go:embed all:frontend/dist`，而 `frontend/dist` 缺失 → 全量构建失败；本机 Go 默认 `GOMODCACHE/GOCACHE` 在工作区外，沙箱不可写 → 无法编译。

**解决办法**：

- 生成 dist：`cd frontend && npm run build`（tsc + vite build）。
- Go 缓存重定向到工作区：`GOMODCACHE/GOCACHE/GOTMPDIR` 指向 `<repo>\.gomodcache` 等（已加入 `.gitignore`）。已验证 `go build ./internal/...` 与 `go vet` 通过。

> ⚠️ **沙箱限制（已实测）**：
> - `npm install --ignore-scripts` 可成功（176 包，含 typescript 与所有类型），因此 `node node_modules/typescript/bin/tsc --noEmit` **能跑**，且**已通过（exit 0）**——证明前端 TSX/绑定/models 改动类型正确。
> - `npm run build` 的 `tsc` 段通过，但 `vite build` 在 esbuild 的 `ensureServiceIsRunning` 处 `spawn EPERM`（esbuild 需派生子进程，被沙箱命名管道限制拦截）。这是硬边界，非代码问题。
> - 结论：**前端类型已验证正确；`dist` 仍需用户在本地 `npm install`（完整，勿加 --ignore-scripts，以安装 esbuild 二进制）+ `npm run build` 生成**。注意本沙箱用 `--ignore-scripts` 装的 `node_modules` 缺少 esbuild 二进制，用户本地应重跑一次完整 `npm install`。

### 难点 8：异步任务结果回传前端 —— ⬜ 待解决（M4）

**难点**：Worker 在后台 goroutine 执行，事件落在 SQLite；前端目前没有任何任务面板/实时订阅。

**解决办法**：在 `agent.Service` 关键节点（状态变更/事件/审批）通过 Wails `runtime.EventsEmit` 推送 `agent:task:*` 事件；前端 `EventsOn` 订阅并渲染任务卡片（进度/事件流/审批按钮）。SQLite 仍是持久化真源，事件只做实时推送。

### 难点 9：TTS/ASR 的打断、回声与噪声 —— ✅ 已解决

**难点**：播放中被打断、ASR 把助手语音当用户输入（回声）、噪声误触发。

**解决办法**：`playbackId` 世代计数 + `stopCurrentAudio` 清理；语音门控（RMS 阈值 + hold）；文本相似度过滤（`textSimilarity` 拦回声）；噪声词表过滤（`isLikelyNoiseTranscript`）。均已在 `App.tsx` 实现。

### 难点 10：记忆隐私边界 —— ✅ 已解决

**难点**：Worker 干活时不应越权读到用户全部长期记忆。

**解决办法**：`BuildTaskContext` 只投影「稳定偏好/项目约定/长期指令 + 按 query 检索的事实/事件」，并保存 `task_context_snapshots` 快照审计（已实现，见 `internal/memory/long_term.go`）。

### 难点 11：回复「快 + 自然」的延迟优化（参考 [Shinsekai](https://github.com/RachelForster/Shinsekai)）—— 🔶 进行中（后端已流式，前端逐句 TTS 待接）

**难点**：桌宠回复「慢、机械」。用户对比 Shinsekai（AI RPG maker，支持 GPT-SoVITS、自动切立绘/背景/BGM）后，希望回复又快又自然。

**根因分析（Shinsekai 为何快+自然）**：

| 机制 | 作用 |
| --- | --- |
| **LLM 流式输出** | 不等全文，首 token（~0.3~1s）即出字，感知延迟从「全文几秒」降到「首字几百毫秒」 |
| **逐句分片 TTS** | 流式文本按标点切句，**第一句一结束立刻合成+播放，同时 LLM 继续吐后面的字**——LLM/TTS 形成流水线，总延迟 ≈ max(首句生成, 首句合成) 而非串行相加 |
| **本地 TTS（GPT-SoVITS/VITS）** | 本地 GPU 推理，无云 TTS 的 HTTP 往返+排队；音色克隆自然稳定 |
| **情绪即时驱动立绘/表情** | 情绪从首句/轻量分类即时得出，立刻切表情，不滞后 |
| **可打断（barge-in）** | 能抢话，更像真人 |

**我们（Yuyu-Mind）的两大差距**：

1. **两段式 Planner→Replyer 多一整轮 LLM**：为结构化决策+情绪，先 Planner 完整走一轮（等 JSON 全文），再 Replyer 走一轮（等可见全文），**可见文本才开始产出**。比单阶段流式多一个完整 LLM 往返，且丢掉「首 token 即出字」。
2. **TTS 全文缓冲 + 云 TTS**：Fish Audio 云 TTS 有网络往返；`SynthesizeSpeechStream`/`StartRealtimeSpeech` 目前是 stub，逐句并行播放没跑通。

**解决办法（分两步）**：

1. **后端流式 Replyer**（已落地）：`ReplyerAgent.Stream` 走 Eino `model.Stream`，`streamingSentencer` 增量按标点/超长切句，`Service.streamReply` 边生成边 `EventTypeToken` emit + 持久化。这样 `App.StreamChat`（Wails `chat:event` 实时推送）能真正逐句流式返回。
2. **前端逐句 TTS + 切到流式通道**（待做）：前端从 `SendMessage`（收集全文）切到 `StreamChat` + `EventsOn("chat:event")`，收到一句 `EventTypeToken` 就合成+播放一句（与 LLM 生成重叠）；`EventTypeEmotion` 即时驱动表情。可选接本地 GPT-SoVITS / 流式 TTS 去掉云往返。

> ✅ **已落地（后端流式）**：`agents.go` 抽出 `ReplyerAgent.buildMessages` + 新增 `Stream`；新增 `stream_reply.go`（`streamingSentencer` 增量切句 + `Service.streamReply` 边流边 emit/持久化）；`service.go` 回复段改用 `streamReply`；`stream_reply_test.go` 验证逐句切分/超长强制切分/空输入。`go build ./...` + `go test ./internal/...`（12 包全绿）。注意：前端仍在用 `SendMessage`（收集式），需切到 `StreamChat` 才能获得可见加速。
> ⬜ **待办**：前端逐句 TTS（切 `StreamChat` + `chat:event` 订阅 + 逐句合成播放）；接通流式 TTS（`SynthesizeSpeechStream` 从 stub 落地）；可选接 GPT-SoVITS 本地 TTS。

---

## 三、迭代记录

> 每次开发在这里追加一条：日期 · 做了什么 · 新增/解决了哪个难点。

- 初始基线：完成全量代码分析；确认 `go build ./internal/...` 通过；确认 `frontend/dist` 缺失、插件系统为占位、Worker 执行器为占位、情绪为关键词启发式。建立 `AGENT.md` 与本文件。
- **M1 后端情绪管线**：新增 `internal/chat/emotion.go`（情绪 Schema 白名单+归一化）；`PlannerDecision`/`ChatEvent` 扩展情绪字段；`PlannerAgent.Plan` 提示词要求结构化情绪输出并归一化；`service.go` 发出 `EventTypeEmotion`；`companion.go` 的 `collectingEmitter` 收集情绪、`SendMessage` 优先 LLM 情绪回退启发式。`go build ./internal/...` 与 `go vet` 通过。解决了难点 1 的后端部分。
- **M2 插件系统内核**：新增 `internal/plugin` 包（`Plugin`/`Manifest`/`Action`/`Host` 接口、`Manager` 生命周期+动作派发、`system` 内置插件、`manager_test.go`）；`app.go` 接线插件工具注册 + 挂载内置插件；`plugin_service.go` 暴露 `ListPlugins`/`EnablePlugin`/`DisablePlugin`/`InvokePluginAction`。`go build`/`go vet`/`go test ./internal/plugin` 均通过。解决了难点 2 的进程内内核部分。
- **M3 电脑工具地基**：新增 `internal/ai/tools/workspace.go`（工作区路径 containment + 符号链接逃逸拦截）与 `filesystem.go`（`list_files`/`read_file`/`write_file`）；只读工具注册进 Planner，`write_file` 保留给 Worker；`config` 增加 `App.WorkspaceRoot`；`filesystem_test.go` 验证越界/软链逃逸拒绝。`go build`/`go test ./internal/ai/tools` 通过。解决了难点 3 的 workspace 安全原语部分。
- **M1.5 + M2 前端补齐**：`ChatReply`/`CompanionMessage` 扩展 mood/energy/gesture/hand 并回填；前端 `App.tsx` 用 LLM 表演参数覆盖 `inferAvatarPerformance`（兜底保留）+ 新增插件面板（列表/启停/调用动作）；`App.css` 插件面板样式；wailsjs `App.js`/`App.d.ts`/`models.ts` 补全新方法与情绪字段。Go 侧 `go build` 通过；前端改动因沙箱 `spawn EPERM` 无法本地 `npm run build`，需用户本地验证。
- **情绪持久化**：messages 表新增 emotion/mood/energy/gesture/hand 列（迁移 002 + `ensureSchemaExtensions` 幂等加列）；`MessageRepo` 读写新字段；`SendService.SendGuidedReply` 接收 `EmotionInfo` 并持久化；`companionMessages` 读取（空则回退启发式）；`db_test.go` 增加情绪往返单测。`go build`/`go test ./internal/db` 通过。收尾了难点 1 的持久化部分。
- **前端类型验证**：排查「前端空白」——根因是 `dist` 缺失 + 上轮沙箱 `npm install` 留下的半成品 `node_modules`（缺 typescript）+ 纯 `npm run dev` 无 Wails 运行时。改用 `npm install --ignore-scripts` 重建依赖后 `tsc --noEmit` 通过（exit 0），证实前端 TSX/绑定/models 改动类型正确；`vite build` 仅因 esbuild `spawn EPERM`（沙箱硬边界）无法打包，需用户本地 `npm install`（完整）+ `npm run build`。
- **M4 Worker 真实执行器**：新增 `internal/agent/llm_executor.go`（`ToolCallingModel` 最小接口 + LLM 工具循环 + `filterToolsByActions` 白名单 + `executeWorkerToolCalls` + `buildTaskMessages`）；`internal/app/worker_executor.go` 把 Eino `ToolCallingChatModel` 适配为 `agent.ToolCallingModel` 并创建模型工厂；`app.go` 接 Worker 工具集（含 `write_file`）。`llm_executor_test.go` 用 fake 模型/工具/运行时验证完整循环。`go build`/`go test ./internal/agent` 通过。解决了「Worker 只校验不干活」的核心缺口。
- **M4 聊天→任务闭环（后端）**：Planner 新增 `task` 动作与 `TaskPlan` 任务包（`agents.go` + 提示词）；`chat/task_plan.go` 提供 `ToTaskSpec` 转换；`chat.Service` 注入 `TaskSubmitter` 并在 `service.go` 处理 `task` 动作提交任务 + 让 Replyer 生成确认语；`internal/app/task_submitter.go` 填充默认工作区 + 安全默认只读动作。`task_plan_test.go` 验证转换与标题回退。`go build`/`go test ./internal/chat` 通过。
- **M4 任务事件回传 + 前端任务面板**：`agent.Service` 注入 `Notifier`，`addEvent` 与状态变更点推送变更；`internal/app/task_submitter.go` 的 `taskNotifier` 经 Wails `EventsEmit("agent:task:changed")` 推送；前端 `App.tsx` 新增任务面板（列表/状态/取消/批准/拒绝/补充回答 + 事件订阅刷新）+ `App.css` 样式。`notifier_test.go` 验证通知器在任务生命周期触发；`tsc --noEmit` 通过。至此 M4 全链路闭环。
- **M3 Worker 审批流**：`Runtime` 接口新增 `RequestApproval`；`taskRuntime.RequestApproval` 先消费已存在的 approve/reject/cancel 控制，无决定则写 question 事件 + 置 `waiting_for_approval` + 返回 `errTaskWaitingApproval`；`service.runTask` 处理该哨兵；`llm_executor` 用 `approvalRequiredTools`（含 `write_file`）对危险工具先审批、每轮一次。`approval_test.go` 验证 提交→挂起→批准→完成 全链路。`go build`/`go test ./internal/...` 通过。
- **M3 命令执行工具**：新增 `internal/ai/tools/command.go`（工作区目录内执行 + 超时钳制 + 输出截断）；接入 Worker 工具集；`execute_command` 加入 `approvalRequiredTools`（需审批）；Planner 提示词补充可用动作清单。`command_test.go` 实测 `exec` 可在沙箱内运行（说明之前 `spawn EPERM` 仅限 Node，Go `os/exec` 无碍），越界 workdir/空命令校验通过。`go build`/`go test ./internal/ai/tools` 通过。
- **清理孤儿 pipeline/template**：删除 `internal/ai/pipeline` 与 `internal/ai/template`（互相引用、无业务导入的死代码），`go mod tidy` 清理依赖；README 进度区同步更新。`go build`/`go test ./internal/...` 通过。解决了难点 6。
- **工作区插件（演示生态）**：新增 `internal/plugin/workspace.go`（`workspace` 插件，`list`/`read`/`write` 动作，复用工作区路径隔离）；前端插件面板加 JSON 参数输入；`workspace_test.go` 验证读写列往返 + 越界拒绝。`go test ./internal/plugin` 通过、`tsc --noEmit` 通过。说明：插件动作是「用户主动触发」，Worker 工具是「LLM 触发 + 审批」，两者安全模型不同。
- **聊天编排测试加固**：新增 `internal/chat/chat_test.go`，覆盖 `extractJSONObject`（JSON/围栏/前后文本）、`postprocessReply`（去舞台指示）、`splitReply`、`looksLikeQuestionOrRequest`、情绪归一化、`PlannerDecision.EmotionInfo`、`TurnGate.Evaluate`（问题触发/弱回撤门控）。修复了 TurnGate 弱回撤用例（需 BotStreak/LastBotAt 才能压到阈值下）。`go test ./internal/chat` 通过（9 个测试）。
- **记忆模块测试加固**：新增 `internal/memory/memory_test.go`，覆盖 `Window.Truncate`（滑动窗口/系统消息保留）、`toSchemaRole`/`fromSchemaRole` 往返、`toMessageRow`/`toSchemaMessages` 工具调用往返、`SQLiteStore` 追加/读取/会话隔离。发现并修正：messages 表有 `conversation_id` 外键，测试须先建 conversation。`go test ./internal/memory` 通过（4 个测试）。
- **Web Search 真实实现**：重写 `internal/ai/tools/web_search.go`——`SearchProvider` 接口 + `DuckDuckGoProvider`（Instant Answer API，免 Key）+ `parseDDGResponse` 纯函数；`web_search_test.go` 用 fake provider + 响应解析测试。沙箱无外网故网络路径未测，但结构与解析已覆盖；用户在本地即可用。`go build`/`go test ./internal/ai/tools` 通过。
- **Planner 健壮性**：抽出 `parsePlannerDecision` 纯函数（JSON 解析 + 情绪归一化）；`Plan` 在解析失败时**重试一次**（附「只返回 JSON」提示），空 action 兜底为 `reply`（不再直接报错）。`chat_test.go` 新增 4 个解析用例。缓解真实 LLM 返回不规范 JSON 导致整轮对话失败的问题。`go build`/`go test ./internal/chat` 通过。
- **插件工具进 Worker 工具集**：`app.go` 用 `workerToolReg`（`tools.Registry`）承载 Worker 工具；`agent.NewLLMExecutor` 改为接收 `toolProvider func() []tool.BaseTool`（动态读取，允许运行期注册）；插件 `RegisterTool` 双写 Planner + Worker 注册表。补齐了「PPT/游戏等重任务插件应作为 Worker 工具」的架构缺口。`go build`/`go test ./internal/...` 通过。
- **插件开发指南**：新增 `docs/PLUGIN-GUIDE.md`（接口/Manifest/Host/示例/挂载/约定/路线图），完成 M6 生态的文档部分。
- **配置/用量测试加固**：新增 `internal/config/config_test.go`（默认值/激活切换/更新 provider，不触盘）与 `internal/usage/collector_test.go`（累计/TotalTokens 回退/nil 安全）。至此 9 个包全绿。`go test ./internal/...` 通过。
- **Git 提交流程**：全量成果提交 `1a54913` 并推送 `dev`；此后每阶段完成即 commit + push。
- **插件配置持久化**：`plugin.ConfigStore` 接口 + `Host.Config` 注入 + `Manager.GetConfig/SetConfig`；宿主用 settings 键值表（`plugin.config.<id>`）存储；Wails `GetPluginConfig`/`SetPluginConfig`；前端插件卡片加「配置/保存配置」按钮；`internal/plugin/config_test.go` 验证往返 + 无 store 行为。`go test ./internal/plugin` 通过、`tsc --noEmit` 通过。
- **键鼠输入合成（M5 游戏基础）**：新增 `internal/ai/tools/input.go`（`KeyVK` 按键名→VK 映射 + `InputTool` 跨平台工具）+ `input_windows.go`（user32 SendInput 实现，key_press/type_text）+ `input_other.go`（no-op 兜底）；`send_input` 加入 Worker 工具集与 `approvalRequiredTools`。`KeyVK` 纯函数已测；SendInput 实际注入需用户本地 Windows 验证。`go build`/`go test ./internal/ai/tools` 通过。
- **屏幕截图（M5 观察基础）**：新增 `internal/ai/tools/screen.go`（`ScreenCaptureTool`，保存 PNG 到工作区）+ `screen_windows.go`（gdi32/user32 BitBlt + GetDIBits → image.RGBA）+ `screen_other.go`（no-op）；`screen_capture` 加入 Worker 工具集与 `approvalRequiredTools`。`go build`/`go test ./internal/...` 通过；实际截屏需用户本地 Windows 验证。视觉模型（多模态描述）待接入。
- **「看屏幕」接线**：`ObserveScreen` 改为真正截屏保存到工作区 `screenshots/` 并返回路径 + 诚实提示（视觉模型未接入）；`App` 存储 `workspace`，`tools.CaptureScreen()` 导出。Eino OpenAI 适配器当前不支持多模态图片，视觉描述需升级适配器或直连多模态 API，属依赖用户模型的延后项。`go build`/`go test ./internal/...` 通过。
- **插件 sidecar（阶段 2）**：新增 `internal/plugin/sidecar.go`——`SidecarSpec`/`SidecarPlugin`/`sidecarClient` 通过 stdio 上的 newline-delimited JSON-RPC 驱动外部插件进程；`Manager.Register` 在 Init 后重读协商的 manifest（sidecar 的 manifest 在运行时才确定）；`sidecar_test.go` 用 re-exec 模式（子进程 = 测试二进制）验证 启动→manifest 协商→动作调用→停止 全链路。第三方插件无需重编译宿主即可挂载。`go build`/`go test ./internal/plugin` 通过。
- **多模态视觉描述**：新增 `internal/ai/vision` 包（`Describe` 直连 OpenAI 兼容多模态 API，`buildVisionRequest`/`parseVisionResponse` 纯函数）；`config` 增加 `Vision.Model`；`ObserveScreen` 在配置视觉模型后截屏 + 描述画面（否则回退「截屏保存 + 诚实提示」）。`vision_test.go` 覆盖请求构造/响应解析；网络路径需用户本地视觉模型验证。`go build`/`go test ./internal/...` 通过（10 包全绿）。
- **UI 美化与滚动修复**：重写 `frontend/src/App.css`（现代配色/圆角/聊天气泡/细滚动条）；`App.tsx` 头部精简（标题+状态+窗口按钮）、功能按钮下沉为 `.toolbar`；`.chat-panel` 由 grid 改为 flex 布局——header/toolbar/composer 固定、`message-feed` `flex:1 min-height:0 overflow-y:auto` 独立滚动，彻底解决「内容显示不全 + 无法滚轮滑动」；窗口默认尺寸 1024×768 → 1200×800。`tsc --noEmit` 通过。难点 8 的「前端无滚动/内容溢出」部分随之解决。
- **蓝白配色 + 桌宠透明**：配色从青绿改为**蓝白**（`--accent #2b6cb0`、`--bg #f5f8fb`、浅蓝 stage 渐变）；显式声明 `html.pet-window, body.pet-window` 背景透明，配合 `WindowSetBackgroundColour` 全透明（alpha=0），消除桌宠模式黑底。
- **桌宠原生窗口透明修复（黑底根因）**：定位「桌宠模式整窗除 Live2D 小人外全黑」的根因——前端 CSS 已透明、`WindowSetBackgroundColour(0,0,0,0)` 也把 WebView2 `DefaultBackgroundColor.A` 置 0（内容透明），但 `main.go` 未配置 `Windows` 选项，`win32.SetBackgroundColour(hwnd, 0,0,0)` 把原生 HWND 背景刷子刷成黑色并透出透明 WebView。修复：`main.go` 增加 `Windows: &windows.Options{WebviewIsTransparent: true, WindowIsTranslucent: true, BackdropType: windows.None}`。已读 Wails v2.12.0 源码确认 `WebviewIsTransparent` 在 `WindowSetBackgroundColour` 中强制 `A=0`、`WindowIsTranslucent` 触发 DWM 透传（Win11 `DWMSBT_NONE` / Win10 毛玻璃回退）。`go build ./...` 通过（exit 0）。
- **无边框窗口 + 情绪系统参考（soullink-emotion-sdk）**：① `main.go` 加 `Frameless: true` + `DisableFramelessWindowDecorations: true`，去掉系统边框/标题栏（桌宠模式无边框悬浮；完整模式复用前端自定义 header 拖拽区 + 最小化/关闭按钮），`App.css` 补强拖拽区（`.chat-panel .header-title`/`.status-bar` 可拖、去掉 `status-bar *` 的 no-drag）。② 研读 [soullink-emotion-sdk](https://github.com/nanlingyin/soullink-emotion-sdk)，把「连续 VAD 情绪 / FACS·AU 表情合成 / 分层动画 / 模型自动适配」四大支柱对照进难点 1 作为情绪系统 v2 演进方向（离散→连续、手调→AU 表、硬编码→注册表适配）。`go build ./...` 通过（exit 0）。
- **情绪系统 v2 落地（连续 VAD + FACS/AU + 参数注册表）**：后端 `emotion.go` 新增 `ClampValence`/`ClampDominance`，`PlannerDecision`/`EmotionInfo`/`ChatEvent`/`CompanionMessage`/`ChatReply`/`db.Message` 全链路增加 `valence`(-1..1)/`dominance`(-1..1)（`energy`≡arousal），Planner 提示词要求连续输出、`parsePlannerDecision` 钳制、messages 表加列持久化、`inferValence`/`inferDominance` 兜底。前端新增 `frontend/src/components/emotionEngine.ts`（`computeAUWeights`：离散 emotion/mood 基线 + VAD 调制 → AU 权重；`computeExpressionTargets`：AU→逻辑参数净目标；`PARAM_REGISTRY` 参数注册表 + `applyExpressionTargets` 运行时按 `getParameterIndex` 自动适配模型命名），`Live2DStage.tsx` 用 AU 驱动微笑/眉/嘴型替换 `moodBoost` 手调权重（不触碰眨眼/唇同步/expression 层），`App.tsx`/`models.ts` 贯通 valence/dominance。`go build ./...` + `go test ./internal/...`（10 包全绿）+ `tsc --noEmit`（exit 0）通过。难点 1 收尾为已解决。
- **模型 ASR 接入（Whisper 兼容）**：排查确认「浏览器 ASR（Web Speech）已接通，但后端 `TranscribeAudio` 是占位（返回 not configured）」。新增 `internal/ai/asr` 包——`Transcribe`（multipart/form-data 调 OpenAI 兼容 `/audio/transcriptions`，file+model+language+response_format=json）、`buildTranscriptionRequest`、`parseTranscriptionResponse`（`text` 字段）、`extFromContentType`（webm/m4a/mp3/wav/ogg 推导扩展名）；`config` 新增 `ASR.Model`（为空=未启用）；`companion.go` `TranscribeAudio` 从占位改为真实转录（解码 base64 → 复用激活 Provider BaseURL/APIKey + `asr.model`）。前端 `startModelASRVoiceInput` 已接线无需改动（`VITE_ASR_PROVIDER` 默认 `browser`，设 `model` 走模型识别）。`asr_test.go` 覆盖解析/扩展名/请求构造；`go build ./...` + `go test ./internal/...`（12 包全绿）。网络路径需用户本地验证。
- **回复延迟优化（后端流式 Replyer，参考 Shinsekai）**：分析 [Shinsekai](https://github.com/RachelForster/Shinsekai) 快+自然的根因（LLM 流式 + 逐句 TTS 并行 + 本地 GPT-SoVITS + 情绪即时驱动），定位我们两大差距——① 两段式 Planner→Replyer 多一整轮 LLM；② TTS 全文缓冲 + 云 TTS。落地后端流式：`ReplyerAgent.buildMessages` 抽出 + 新增 `Stream`（Eino `model.Stream`）；新增 `stream_reply.go`（`streamingSentencer` 增量按标点/超长切句 + `Service.streamReply` 边生成边 `EventTypeToken` emit + 持久化）；`service.go` 回复段改用 `streamReply`。`stream_reply_test.go` 验证逐句切分/超长强制切分/空输入。`go build ./...` + `go test ./internal/...`（12 包全绿）。剩余：前端从 `SendMessage` 切到 `StreamChat` + 逐句 TTS（难点 11 待办）。
