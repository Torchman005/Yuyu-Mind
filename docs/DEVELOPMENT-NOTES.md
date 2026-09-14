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
>
> ✅ **已升级为「目录即插件」的即插即用（M5）**：插件 = 一个目录，元数据写在 `plugin.json`/`.yaml`/`.yml`/`.toml` 之一，配置写在 `config.<fmt>`。宿主在启动 / `ReloadPlugins` 时扫描 `plugins_root`（`config.app.plugins_root`，默认 `plugins/`，相对可执行文件目录）下的每个子目录，读 manifest 注册能力；实现能力的是 sidecar 入口（默认 Node.js `main.js`，按 `runtime` 字段或扩展名识别），按需拉起、stdio JSON-RPC（`invoke_action`/`invoke_tool`）。配置读写走 `FileConfigStore`（插件目录内的 config 文件，内置插件回退到 settings）。新增/删除插件目录后在插件页点「重新加载」即热生效，**无需改任何 Go 代码**。
>   - 新增：`internal/plugin/fileconfig.go`（manifest/config 的 json/yaml/toml 解析 + `FileConfigStore` + `CompositeConfigStore`）、`internal/plugin/dirplugin.go`（`DirPlugin` + `DiscoverPluginDirs` + 懒拉起 sidecar + `SetBaseDir` 注入 `YUYU_WORKSPACE`）、`internal/app/plugin_dir.go`（`resolvePluginsRoot`/`loadDirPlugins`/`ReloadPlugins`）；`Manager.Remove` 支持热卸载；`Manifest` 增加 `Runtime`/`Tools`；`PluginInfo` 增加 `tools`/`loadedTools`；`app.go` 对发现的目录插件调用 `SetBaseDir(workspace.Root())`。
>   - 示例：`plugins/hello/`（plugin.json + config.json + main.js + README，一条动作 `hello` + 一个工具 `shout`）、`plugins/code-assistant/`（用 **Codex CLI** 写代码的工具 `run_agent` + 一键开 IDE 的动作 `open_in_ide`/`open_workspace`）、`plugins/netease-music/`（搜歌/点歌/播放控制的 `control` 动作与 `control_netease_music` 工具，经本机 NeteaseCloudMusicApi 出播放直链，前端出声）。
>
> ✅ **桌宠日志（M6）**：新增 `internal/loghub`（既是 `slog.Handler` 又缓存内存环形缓冲），前端详情页新增「桌宠日志」导航项 `GetLogs`/`GetLogLevel`/`SetLogLevel`，展示含插件调用在内的日志；等级由配置 `log_level` 控制（默认 DEBUG），可在页面运行时切换并写回配置文件。插件工具/动作调用会 `slog.Info("plugin tool/action invoked: ...")`，因此排查「桌宠为何不做事」可先看这页日志。
> ⬜ **待办**：动作级权限强制、`goja` 脚本引擎（阶段 3）、工具桩在卸载后的摘除（当前重名覆盖、不主动 `RemoveTool`）。

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

### 难点 11：回复「快 + 自然」的延迟优化（参考 [Shinsekai](https://github.com/RachelForster/Shinsekai)）—— ✅ 已解决（后端流式 + 前端逐句 TTS）

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
2. **前端逐句 TTS + 切到流式通道**（已落地）：`sendContent` 从 `SendMessage`（收集全文）切到 `StreamChat` + `EventsOn("chat:event")`；收到一句 `EventTypeToken` 就 `speakText` 合成+播放一句（`streamSentenceQueueRef`/`sentencePlayingRef`/`streamReplyActiveRef`/`streamDoneRef` 状态机，与 LLM 生成重叠）；`EventTypeEmotion` 即时驱动表情；`done` 收尾、`error` 中止；`App.StreamChat` 补 `ensureCompanionReady`。

> ✅ **已落地（后端流式 + 前端逐句 TTS）**：
> - 后端：`agents.go` 抽出 `ReplyerAgent.buildMessages` + 新增 `Stream`；新增 `stream_reply.go`（`streamingSentencer` 增量切句 + `Service.streamReply` 边流边 emit/持久化）；`service.go` 回复段改用 `streamReply`；`stream_reply_test.go` 验证逐句切分/超长强制切分/空输入。
> - 前端：`App.tsx` 新增 `ChatStreamEvent` 类型 + `DESKTOP_COMPANION_CONVERSATION_ID` + 流式状态 refs（`streamSentenceQueueRef`/`sentencePlayingRef`/`streamReplyActiveRef`/`streamDoneRef`/`chatEventHandlerRef`）；`chat:event` 订阅（最新 ref 模式避免闭包过期）；`sendContent` 切到 `StreamChat`；`finishSpeaking` 接逐句 drain；`handleChatEvent` 处理 token/emotion/error/done；`App.StreamChat`（app.go）补 `ensureCompanionReady`。
> - 验证：`go build ./...` + `go test ./internal/...`（12 包全绿）+ `tsc --noEmit`（exit 0）。渲染/延迟效果需本地 `wails dev` 验证。
> ⬜ **待办（可选 v2）**：接通流式 TTS（`SynthesizeSpeechStream` 从 stub 落地，进一步去云往返）；可选接 GPT-SoVITS 本地 TTS 换自然音色。

### 难点 12：sidecar 插件如何让桌宠「出声」—— ✅ 已解决（netease-music 点歌）

**难点**：目录插件（`netease-music`）跑在 Node sidecar 进程里，只能经 stdio JSON-RPC 回一行结果；桌宠的前端 `<audio>` 与语音状态机都在 React 侧，sidecar 无法自己播放，也无法收到“播完了”回调。若只回文本，LLM/用户能搜到歌却永远听不到声音。

**解决办法**：确立「**sidecar 出指令、前端出声**」的返回契约——`control`（action 与 tool 共用 `createController` 内核）返回 `message`（人类可读摘要）+ `track`（规整曲目）+ `metadata`：

```jsonc
{ "ok": true, "intent": "play", "message": "正在播放：晴天 - 周杰伦（4:29）",
  "track": { "songId": 186016, "name": "晴天", "artists": "周杰伦", ... },
  "playbackUrl": "http://...",
  "metadata": { "playbackAction": "play|pause|resume|stop", "playbackUrl": "...", "track": {...}, "state": {...} } }
```

- `play`/`resume` 且 `openPlaybackUrl=true`：sidecar 调本机 NeteaseCloudMusicApi（默认 `http://127.0.0.1:3000`）`/search` → `/song/url/v1?level=...` 拿直链，前端收到 `metadata.playbackUrl` 即 `new Audio(url).play()`。
- `pause`/`stop`：前端只按 `playbackAction` 暂停/停止当前音频；`resume` 复用上次直链。
- 前端配套：`App.tsx` 的 `invokePluginAction` 从“只播 URL”改为统一走 `handlePluginPlaybackResult`（此前是死代码），netease-music 详情页渲染“音乐播放”输入框（自然语言→`message`，回车/点歌触发 `control`），通用动作网格排除 `control` 避免重复。
- 播放状态（当前曲目/播放/暂停）存 sidecar 内；sidecar 常驻（宿主懒拉起后缓存复用），多次调用共享状态。

**自然语言解析**（`resolveIntent`/`commandOf`/`stripIntent`）：动词表 + 前缀匹配，需“独立成词”防误判——`放风筝`/`点歌台`/`听雨` 是歌名不是指令；`继续播放`→resume、`暂停一下`→pause、`别放音乐`/`别播放了`→stop、`现在放什么`→status；`播放 1` 命中最近搜索缓存，未搜过则回退按关键词搜（避免生硬“请先搜索”）。**测试驱动修正**：初版把“搜索 周杰伦”整个当关键词（漏剥 2 字动词）、`播放 2` 未搜过直接报错、LRC 解析把 `[mm:ss]` 带进正文，均由回归测试暴露后修复。

**工程**：`main.js` 纯 Node 无第三方依赖（`http/https` + `URL`，Node 18+ 无需 fetch polyfill）；顶层 stdin 壳用 `require.main === module` 守卫，require 供测试时不挂 readline（否则 `node --test` 因 stdin 句柄永不退出）。回归测试 `main.test.js`（8 项）分三层：① mock api 的意图/状态机/付费兜底；② 进程内 mock HTTP 服务 + spawn 真实入口的 JSON-RPC 闭环；③ 连接拒绝兜底。真实网易服务联调需用户本机启动 NeteaseCloudMusicApi。

---

## 三、迭代记录

- **2026-09-11 拟人化 P3：性格随关系漂移（"相处久了，说话方式会变"）**：用户说"继续"，遂做可选项里最有价值的一项——它把前两轮做的"关系"从一句 prompt 变成**看得见的行为变化**。
  **要解决的问题**：关系状态做完之后，它只影响一句"你们已经很熟了"的模糊描述。这能改语气松紧，但**角色本身没变**。真人相处久了会变：更放松、更省客套、更少解释、玩笑更随口。长期陪伴需要这种"时间感"，否则一百轮和第一轮没有区别。
  **做法**（`internal/chat/drift.go`）：按交互次数分档（0 / 40 / 120+）给出风格偏移提示，注入 **Replyer** 而不是 Planner——风格是"怎么说"，越靠近台词生成点越有效（Planner 只决定"说不说、用什么情绪"）。`applyStyleDrift` 把偏移**追加**在原 `style_notes` 之后而不是替换，因为原有的人设风格要求（口语化、不要动作描写）必须保留；未达阈值时返回空串，让行为与改造前**完全一致**。
  **最关键的一个约束（也是我特意守住的线）**：漂移方向必须与人设**同向**。人设是"古灵精怪、调皮、**话少**"，所以漂移只能往"更放松 / 更省字 / 玩笑更随口"走——**绝不能**出现"越熟越话痨、越热情、越多说"，那等于把人设往反方向拉。我为这条线专门写了**禁止词表断言**（话痨/啰嗦/多说/长篇/更热情/滔滔不绝…），并让最高档也明确写"有时一句吐槽就够了"。阈值也刻意设得高（40/120 次交互），要让用户真的感到"处久了"，而不是聊十句就换个人。
  **另一条边界**：漂移**不改变称呼**（「主人」）与底线要求（无动作描写、不暴露 AI 身份）。这些属于角色设定，任何关系阶段都不动——这已经是第三次在"拟人化 vs 角色一致性"冲突时选择保一致性（前两次是关系不改称呼、情绪不越权改称呼）。
  **验证**：新增 `drift_test.go` 6 项（未达阈值完全不影响行为、档位随交互单调不回退、**方向不违反人设且禁止词表**、偏移追加而非替换、无人设风格时的退化、Service 层读取关系给出漂移）。全量：`go build ./...`、`go vet`、`go test ./internal/...`（13 包）、`tsc --noEmit` 全绿。

- **2026-09-11 拟人化 P2：记忆自动抽取 + 关系状态（"记得住"与"关系会累积"）**：用户要"挨个执行"，这是最后两项之一（也是分析文档里标为"最大一块"的）。
  **问题一：记忆读得到、写不进**。长期记忆的**写入**此前只有手动路径（`UpsertMemory`/`AddMemoryCandidate` 仅作为 Wails 方法导出给人调用），对话过程里发生的事完全不落库。于是"你告诉它的事实/偏好，下一轮就没了"，跨会话更像初次见面。**做法**（`internal/chat/memory_extract.go`）：回复产出后用**一次低 token 的 LLM 调用**判断"这段对话里有没有值得长期记住的**用户**信息"，只接受 preference/fact/instruction 三类、最多 3 条、并要求模型给出**稳定的 key**——这样同一偏好再次出现时是**覆盖更新**而不是堆一堆近似重复的记忆（这是"记忆越用越脏"的常见根因）。解析写成容错纯函数：容忍 markdown 包裹、前后多余文字、非法 kind、缺字段、彻底非 JSON；抽不到就返回空——**大多数闲聊本来就该是空**，不灌噪声。抽取**异步**执行（30s 超时 + panic 兜底），因为它是"记笔记"，不该让用户等；失败只记日志。
  **问题二：完全没有关系概念**。每次对话的语气都像第一次认识。**做法**（`internal/chat/relationship.go`）：维护 `{交互次数, 首次/最近互动时间, 亲密度}`，用 `db.Settings` **持久化**（复用既有键值表，不新增 schema）；更新用**客观信号**——交互次数、消息长度、礼貌用语、情绪词，单次上限 3，**不额外调用 LLM**（关系是慢变量，不值得每轮付一次模型成本）；每 5 次交互才落库一次，控制写入频率。注入 Planner 时按阶段给出**模糊描述**（"还不太熟"→"已经聊过一阵子"→"比较熟"→"已经很熟，像老朋友"），与情绪底色同样**不暴露数值**。
  **一个刻意的边界**：关系**不改变人设规定的称呼**（「主人」）。我考虑过"越熟越随意、可以改口"，但那会破坏角色一致性——人设里写死了这个称呼。所以关系只影响**语气松紧、玩笑尺度、话题深度**。这是"拟人化"与"角色一致性"冲突时的一次取舍，我选择保一致性。
  **测试抓到的两个坑**：① `Service` 里已有 `longMemory *memory.ServiceMemory`，我一度另起了一个 `memoryExtractor` 接口，结果签名对不上（真实返回 `*db.UserMemory`）、还多导入了用不到的 `db` 包——**先查已有字段再定义抽象**；② 插入关系代码时把 `currentEmotion, _ :=` 写了两遍，Go 直接报 "no new variables on left side of :="，这类错误编译期就能拦住。
  **验证**：新增 `memory_extract_test.go` 11 项覆盖解析的 8 个子场景、过短输入不调模型、关系累积与单次上限、**跨 store 实例持久化（模拟重启后关系不丢）**、关系描述分档且不含数值、以及 `ObserveUserTurn` 的关系升级路径。全量：`go build ./...`、`go vet`、`go test ./internal/...`（13 包）、`tsc --noEmit` 全绿。

- **2026-09-11 拟人化 P1：口语冗余 / 口误自纠正（把死配置救活）**：用户说"继续"，这一项改动小、但直接改善"像不像在说话"。
  **要解决的问题**：模型文本永远"干净、完整、有条理"——这本身就是机器感。真人说话有填充词（嗯…、那个…）、会重复字（"我我"）、会半截话。而且**语音场景下这比"错别字"更贴切**：用户听的是停顿和语气，不是字形。
  **做法**：`applyDisfluency(sentence, r1, r2)` 以很低概率（填充词 12%、重复首字 6%）注入。**关键在"不破坏语义"**：句子 <4 字不加（"嗯…好"很怪）、已带前置标点不加（否则"嗯…嗯…"叠两段停顿）、重复只作用于人称/指示/判断类首字（"我我""这这"自然，"算算法"就是错）。
  **顺带发现并救活一个死配置**：`AllowTypoSimulation` —— 全仓库**只有定义和赋值、没有任何消费点**（README 里把它写成已有功能，实际上从未生效）。现在它控制口语冗余、默认开启；字段名沿用以兼容既有配置，注释里写清了这件事。
  接入方式：新增 `Service.postprocessSpeech` 作为**下发台词的统一入口**（先清理舞台提示，再按配置口语化），避免在多个出口重复逻辑；流式的 flat-text 与结构化两条路径、以及主动发言都走它。
  **验证**：新增 4 项测试（低随机值注入已知填充词且不损原句、高随机值必须原样返回、短句与已带标点不注入、重复首字只在安全首字生效）。全量：`go build ./...`、`go vet`、`go test ./internal/...`（13 包）、`tsc --noEmit` 全绿。

- **2026-09-11 拟人化 P1：概率门控稀疏情绪更新（"心情不该每句话都重估"）**：用户要"再做 1"，即把情绪更新从"每轮必推"改为"稀疏演化"，对标 MoFox。
  **要解决的问题**：上两轮让情绪有了惯性与底色注入，但它仍然**每轮都被推一次**。后果有三个：① 用户说句无关紧要的话（"嗯""好的"）心情也跟着动，表现为抖动；② 每次都多付一次"情绪判定"的隐含成本；③ 逻辑上像"每句话都在重新评估你"，非常假。真人的心情是**延续的**，只在被真正触动时才变。
  **做法**：`MaybeUpdate(convID, suggested, interest, r, now)`，用
  `概率 = 0.5 × 时间增益(1→2×) × 兴趣倍数(0.5→1)`，夹在 `[0.2, 1]`。
  - **未命中**：心情**延续**（标签与基调都保留），只按经过时间做衰减——所以"隔很久回来"仍会平复，但"连着说几句废话"不会把心情搅乱。
  - **时间增益**：距上次推进越久越容易变化，避免长时间延续同一情绪时僵死。
  - **下限 0.2**：防止概率归零导致情绪彻底锁死。
  - 随机源 `r` 由调用方注入（不是内部 `rand()`），这样行为可以**确定性测试**。
  **兴趣度怎么来**：新增 `ExtractInterest(text)`，用**客观信号**——长度、疑问、感叹、情绪词表，纯应答词（嗯/好的/收到）判 0.05。这里刻意**不再调一次 LLM**：门控本身就是为了省成本与降抖动，若为此再加一次模型调用就本末倒置了。
  **一个设计上的自我修正**：我最初让 `Update` 内部也走门控（用 `rand()`），结果两个既有测试立刻失败——因为它们依赖确定性推进。这暴露了接口语义混乱：`Update` 应当明确表示"总是推进"。改为 `r < 0` 作为**显式的强制信号**（而不是假设"随机数恰好小于概率上界 1"这种边界巧合），`Update` 保留为确定性入口，生产路径才用 `MaybeUpdate`。
  **测试又抓到一个真实缺陷**：`ExtractInterest("今天面试好紧张啊")` 只算出 0.3 —— 长度阈值定成了 12 字（这句只有 8 字），而且**「紧张」根本不在情绪词表里**。也就是说，一句明显带情绪的话不会推动心情。修正为长度 ≥8 字即加分，并补全词表（紧张/委屈/孤独/崩溃/离谱等）。**测试的价值正在于此：它逼我把参数和词表的漏洞暴露出来，而不是让"稀疏更新"悄悄变成"几乎不更新"**。
  **验证**：新增 4 项测试——门控未命中时心情必须延续且不跳变、命中时位移恰为惯性比例、同一随机数下高兴趣能推进而低兴趣不能、`ExtractInterest` 的信号分档与"应声 < 有内容消息"。全量：`go build ./...`、`go vet`、`go test ./internal/...`（13 包）、`tsc --noEmit`、`npm run build` 全绿。

- **2026-09-11 拟人化 P1：情绪以"模糊底色"注入 Planner**：用户选了这一项（改动最小、直接改善措辞自然度）。
  **要解决的问题**：上一轮做完情绪状态机后，情绪**已经**有惯性了，但它只作用于"前端表情"与"落库字段"——**Planner 完全不知道角色此刻什么心情**。于是会出现"表情是低落的，它下一秒又用很亢奋的语气回复"这种脱节。
  **做法**：新增 `emotionPromptLine()` 把 `EmotionVector` 翻译成**一句人话**（"你现在心情很好，情绪比较激动。这是你此刻真实的情绪底色…"），按 valence/arousal/dominance 分档挑措辞，作为 `current_mood_background` 注入 Planner 的 user 段；同时在 system prompt 里说明**怎么用**——特别是要求"情绪有惯性，不要因为一句话就翻到相反情绪，要逐渐移动"。
  **三个刻意的设计选择**：
  ① **措辞必须模糊**：不写"请输出 20 字以内"，也**不暴露任何数值**。理由来自对标 MaiMBot 的观察——情绪一旦变成精确指令或参数，模型会开始"执行数字"而不是表演情绪，机器感立刻回来。为此专门写了断言"描述里不能出现 `0.` / `%` 等数值片段"的测试。
  ② **不改采样参数**：temperature/max_tokens 一律不动。情绪只影响"选词与语气倾向"，这也与 MaiMBot 的实际实现一致（它从不用情绪改 temperature）。
  ③ **快速通道也要继承情绪**：原先快速通道（≤40 字闲聊、跳过 Planner）是"用关键词从用户消息重掷一个情绪"，与前面的情绪完全无关；现改为**优先沿用当前情绪底色**，只有没有历史情绪时才回退关键词判断。否则"跳过 Planner"就等于"情绪断层"。
  **验证**：`emotion_state_test.go` 从 9 项扩到 12 项，新增 `TestEmotionPromptLineIsVagueNotNumeric`（7 个分档子用例 + 逐条断言不含数值）、`TestEmotionPromptLineEmptyWithoutState`（会话刚开始时返回空串，**不伪造**一个心情）、`TestCurrentEmotionReflectsStore`。全量：`go build ./...`、`go vet`、`go test ./internal/...`（13 包）、`tsc --noEmit`、`npm run build` 全绿。

- **2026-09-11 拟人化 P2 主动搭话：把模板换成"选话题 → 判时机 → 才生成"**：用户选了这一项，理由是观感提升最明显。
  **问题**：原先的主动发言是 `buildProactiveLine` —— **纯字符串拼接、完全不经过 LLM**，永远是"我还记着你刚才说的「X」，要不要继续从这里往下处理？"。每 8 分钟空闲就复读一次同一句式，**比不说话更伤真实感**（复读机特征太明显）。
  **做法**：抄 MaiMBot 的三段式骨架，但把中间那步换成更省的实现：
  ① **选话题**（一次廉价 LLM 调用，`max_tokens=8`）：从历史里挑出候选喂给模型，让它只回一个**编号**或 `none`。候选的构造有讲究——只取**用户说过的话**（主动搭话应该是"接住对方提过的事"，不是复述自己），最新优先、过短过滤（<4 字没信息量）、去重、截断到 40 字。
  ② **判时机**：这里**没有**用第三次 LLM 调用，而是用客观信号——距上次主动发言不足 90 秒就跳过。MaiMBot 用"只输出 yes/no 的廉价调用"挡在生成前，思路对，但"刚说完又搭话"这种事用时间戳判断既更准也更省。
  ③ **才生成**：带上人设、风格、选定话题，生成一句口语化的主动发言；并让它走**情绪状态机**，这样主动开口时情绪是延续的，而不是凭空来一句。
  **容错设计**：模型回答不符合格式（既不是编号也不是 none）时，**保守地当作"不开口"**，而不是随机猜一个话题——宁可这次不说话，也不要说出莫名其妙的内容。生成链路整体失败时退回原模板（`proactiveFallback`），保证"主动搭话"这个功能不至于因为模型不可用而完全消失。
  **过程中测试抓到两个真 bug**：
  1. **prompt 注入错了字段**：`writeProactiveSpeech` 里写的是 `nonEmptyString(s.cfg.Chat.BotName, ...)`，于是系统提示变成"你是 Yuyu。说话简短"——**人设根本没进去**，主动发言会丢掉角色感。测试断言"prompt 应包含人设"直接把它抓出来了，改为显式注入 `Persona`。
  2. **前端白吃配额**：后端判断"此刻不开口"时返回空回复，而前端 `GenerateProactiveMessage().then(...)` 无条件执行 `applyResponsePerformance` + `speakResponse`，并且**已经把这次时间戳记进了配额数组** —— 于是一次"沉默"会白白吃掉一次主动发言机会。改为先检测回复是否为空，为空则不朗读、不更新表演，并把刚压入的时间戳**回滚**。
  **验证**：新增 `proactive_test.go` 7 项，用自建 `scriptedModel`（实现 `Generate`+`Stream`）覆盖候选构造、编号解析、`none` 判定、异常回答不越界、太近跳过、无候选跳过、生成段带人设且清洗舞台提示。全量：`go build ./...`、`go vet`、`go test ./internal/...`（13 包）、`tsc --noEmit`、`npm run build`、插件 9+4 项全绿。

- **2026-09-11 拟人化 P1 情绪数值动力学（"情绪要有惯性、会平复"）**：用户说"继续"，遂推进 P1 里被标为质变的一项。
  **要解决的问题**：LLM 每轮独立吐出 `emotion/valence/arousal` 就被直接发出去驱动表情，于是**情绪是"每轮重掷的骰子"**——上一句还难过、下一句就雀跃，单轮翻转。真人的情绪是慢变量：有惯性、被打断后需要时间平复、也不会因为一句话就换个人。
  **做法（`internal/chat/emotion_state.go`）**：把"建议值"与"实际生效值"分离开，中间夹一层状态机。
  ① **时间衰减**用 `exp(-ln2·dt/halfLife)`（半衰期 3 分钟）向**不对称基线**回归——`valence→0`、`arousal→0.5`（心情归中性，但唤醒度不该归零，否则会显得呆滞）。刻意用**真实经过时间 dt** 而不是"经过几轮"：这样"用户离开十分钟再回来"会自然平复，而连聊十句则保持热度，符合直觉。
  ② **惯性平滑**：新值只向本轮建议走 `35%`。实测把 `+0.9` 的建议配给一个正在开心的角色，结果落在 `0.27` —— **没有一步翻到负值**，这正是要的（要连续几轮负面才会真的低落）。
  ③ **离散标签的最短保持时长（25s）**防表情抖动，但留了一个**逃生口**：效价位移超过 `0.5` 时允许立即切换。原因很实际——如果刚被凶了还因为"保持期"继续笑，那种滞后比抖动更违和。
  ④ 状态按会话隔离、放进程内存。重启回基线，语义上等于"睡了一觉心情平复"，可接受；要跨重启延续就把 `EmotionVector` 落库替换 load/store。
  **顺手修掉的一个一致性问题（值得单独说）**：我先只把平滑值发给前端，转头发现**Replyer 的表演指令和消息落库读的仍是未平滑的 `PlannerDecision`** —— 结果是"表情已经难过、台词却还在雀跃"，前后矛盾比原来的跳变更假。于是抽出纯函数 `decisionWithEmotion()` 把平滑结果**写回 decision**，让表情（事件）、台词（Replyer 指令）、历史（落库）三者用同一个值；并专门写了一个测试锁住这一步，防止日后有人顺手把写回删掉。
  **验证**：新增 `emotion_state_test.go` 共 9 项（首次采纳 / 惯性阻止翻转含精确位移 / 保持期内不抖 / 超时切换 / 强变化绕过保持 / 一个半衰期回归基线 / 会话隔离 / 越界钳制与非法标签回退 / 一致性写回）。全量：`go build ./...`、`go vet`、`go test ./internal/...`（13 包）、`tsc --noEmit`、`npm run build`、插件 9+4 项全绿。
  **过程中自己踩的坑**：惯性测试第一次失败，报"单轮不应穿过中性点，实际 0.27"——其实 0.27 是**正确结果**，是我的断言条件写反了（把"应该没翻负"写成了"应该为负"）。测试失败时先核对期望值本身，而不是急着改实现。

- **2026-09-11 拟人化 P1-7 打断记忆一致性（"让角色只记得自己真正说出口的话"）**：这是 `REALISM-ANALYSIS.md` 里标为**性价比最高**的一项，也是多数同类项目都漏做的细节。
  **问题的本质**：流式回复的持久化是「**生成一句就落库一句**」（`stream_reply.go` 的 `flushPart`），而 TTS 播放**滞后于生成**。用户在 Yuyu 说到一半时插话，那些已经生成、已落库、却**还没播出去**的句子仍留在历史里——下一轮模型读历史时会以为自己说过这些话。真人不是这样记账的：被打断时，只有真正出口的部分才算"我说过"。穿帮表现是它坚称"我刚才说了…"，而用户根本没听见。
  **为什么"删掉未播出的"是正确解**：我比较过三种做法——① 只在历史里插一条"被打断"标记（最小改动，但未播出的台词仍留在历史里，模型还是"记得"没说的话）；② 只删不标记（模型不知道自己被打断，可能又从头讲一遍）；③ **删除未播出 + 插入标记**（两者都解决）。选 ③。
  **实现上的一个坑**：`messages` 表的排序只按 `created_at`，而**同一轮回复的多条消息是在同一毫秒写入的**，时间戳相同——用 `ORDER BY created_at` 根本分不出先后，按时间去删会删错。解法是用 SQLite 的隐式 `rowid`（TEXT 主键表也有）排序，这在 `TailAssistantMessages` 里固定下来，并写了专门的回归测试（刻意用相同时间戳创建三条消息）。
  **分层**：仓储层新增 `DeleteMessages`（空列表安全 no-op）与 `TailAssistantMessages`（只取末尾**连续**的助手发言，遇到第一条非 assistant 就停，避免把上一轮也算进来）；`chat.Service.RecordInterruption(ctx, convID, playedCount)` 承担业务语义（保留前 playedCount 句、删其余、追加 `[被用户打断]` 标记 + `source_kind=interruption`）；app 层暴露 Wails 方法（生成的绑定需手补，否则前端拿不到 `window.go` 上的方法）。
  **前端的判定逻辑（最容易做错的地方）**：`playedCount` 的口径必须和"未播出"严格互补。我在 `finishSpeaking` 里计数——那是**一句真正播完**的时刻（多个播放入口最终都汇聚到这里，比在 6 个 `playQueueHead` 分支里分别埋点可靠得多）。同时必须用 `streamDoneRef` 守卫：**正常说完后的清理走的是同一个 `abortStreamReply`**，若不判断"是否真的没说完"，每次正常回复结束都会被记一次打断、历史里会堆满 `[被用户打断]`。所以 `recordInterruptionIfNeeded()` 只在「流式仍活跃 且 LLM 尚未 done」时生效，并在结算后立刻把计数与标志清零（顺序不能反，否则 cleanup 会把计数重置掉）。接入的三个真实打断点是：barge-in 抢话、键盘中断正在朗读、`abortStreamReply`。
  **验证**：新增 `interruption_repo_test.go`（插入序 / 不跨轮 / 批量删除）与 `interruption_test.go`（保留已播出并丢弃未播出 / 一句没播时清空 / 超计数不误删 / 负值钳制 / 空会话报错）；日志实测 `interruption: dropped unplayed sentences kept=2 dropped=2`。全量：`go build ./...`、`go vet`、`go test ./internal/...`（13 包）、`tsc --noEmit`、`npm run build`、插件 9+4 项全绿。

- **2026-09-11 拟人化 P0 实施（用户："开始"）**：按 [`REALISM-ANALYSIS.md`](REALISM-ANALYSIS.md) 的 P0 清单落地前三项，目标是让 Yuyu 从"应答机"变成"在场的角色"。
  **① 打破必回（`turn_gate.go`）**：原先 `private_session` 白送 `0.60` 分，而阈值只有 `0.45`——于是**用户发的每条消息都必然触发回复**，`looksLikeWeakBackchannel` 的 `-0.30` 永远翻不了盘，"允许沉默"在结构上不可能发生。这是"秒回客服感"的第一来源。改为：基础分 `0.48`（普通陈述刚好过线）、弱回撤惩罚加重到 `0.35`（`0.48-0.35=0.13`，即使叠加冷场加成 `0.15` 也只有 `0.28`，稳稳低于阈值）；新增 `ChatConfig.AllowSilenceOnBackchannel`（默认 true，显式 false 可完整回滚）。**踩到的坑**：`bool` 零值无法区分"未设置"与"显式关闭"，导致既有测试用 `ChatConfig{ReplyThreshold:0.45}` 构造时开关被当成 false、测试直接挂掉；解法是 `NewTurnGate` 里把"其余拟人化字段同时为零"视为未配置、按默认启用——与 `config.Load()` 的"先填默认再 JSON 覆盖"语义保持一致。
  **② 弱回撤识别扩展**：词表补到「好嘞/知道了/明白了/收到/没事儿/OK/好的。」等，并先剥掉句末标点再比对；关键是**带疑问语气（「哦？」「嗯？」）必须排除**——那是在追问而不是单纯应声，静默会让你觉得它没听见。
  **③ 放松表达约束**：Replyer prompt 原本写死「Split into 1-2 short lines; keep the reply brief (say little, don't ramble or pad)」——这条直接制造了"每条回复形状都一样"的机器感。改为按情境 1–4 句、明确欢迎语气词（嗯/诶/啊/呀/嘛/啦）、轻微重复、自我更正（"我是说…"），并允许只用一句短反应（"诶？"）；同时把 persona/style_notes 从 system 段移到 user 段（更靠近生成点），默认人设也从"话很少、说得少而精"放宽为"随性口语化、看心情"。
  **④ 清洗放宽（`reply_text.go`）**：`postprocessReply` 原来用一个宽泛的行内规则删除**所有**括号内容，会把真人口语里的插入语（「那个（我是说）…」）一起删掉，是回复过度书面化的帮凶。改为只删「整行纯舞台提示」与「行首提示」，行内括号保留。顺带发现：**这个函数此前完全没有测试**（正是研究中提到的"静默失效"隐患），本轮补上 9 个用例。
  **验证**：`go build ./...` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit` + `npm run build`；新增 `TestTurnGateBehaviorMatrix` 用 **15 个真实场景**锁定整体行为（普通陈述/提问/请求/被点名/长句倾诉/冷场 → 必回；「嗯/好的/知道了/收到/OK/好的。」→ 静默；「哦？」「嗯？」→ 必回），作为"过度沉默"的防线。**核实**：P0-2 反应停顿（`ThinkingPause` 250–900ms、`sync.Once` 保证只作用于首句、可被 ctx 取消）此前已实现且有 AST 测试保护，本轮无需改动。

- **2026-09-11 详情模式体验修复（用户反馈 3 项）+ 一次严重的文件损坏事故**：
  **① 取消关闭按钮、改为点击抽屉外关闭**：原先每个面板都在标题行内嵌一个「✕ 关闭」，用户反馈"太丑、占一整行"。移除全部内嵌按钮，改为在 `web-content` 内渲染一层透明 `.drawer-backdrop`（`inset:0; z-index:25`，低于抽屉的 30）承接"点外部关闭"——**顺带解决了另一个隐患**：若直接监听外部点击而不加遮罩，点舞台空白会同时触发下方的角色点击互动；遮罩把事件截住就避免了这种双重响应。遮罩关闭时一并 `setTaskDetailId(null)`，回列表态。
  **② 模型信息页标题重复**：因为 `ModelView` 自带「模型信息」标题，而我这轮又在抽屉外层加了一个同标题行 → 显示两次。删除外层标题行，抽屉只渲染 `ModelView` 本体。
  **③ 后台任务页"坍缩"、文字被截断 → 重构为「列表 + 详情」两级**：原实现把**全部详情**（代码变更、逐条文件、补丁、操作按钮）都堆在列表的每张卡片里，抽屉宽度下必然挤压变形。现改为：列表只给「标题 + 目标（单行省略）+ 状态徽标 + 变更数 + 时间 + ›」，点击整行进入详情；详情再分区块展开（执行信息 meta 网格、代码变更与补丁、任务包 JSON、结果），审批/取消按钮收进 `task-actions-bar`。状态徽标、meta 网格、JSON 预览（`max-height` + 内部滚动）都补了专用样式，JSON 用等宽且 `overflow-wrap:anywhere`，不再撑破抽屉。
  **④ 导航收敛**：`WEB_NAV` 去掉「对话」「外观/皮肤」两项，只保留 房间/模型信息/插件管理/后台任务/桌宠日志/设置；移除房间里那个「⇱ 列表视图」按钮与 `RoomView.onExitRoom`；`newConversation`/`selectConversation` 改为停留在房间。
  **⚠️ 事故与恢复（必须记录）**：我用 PowerShell 正则做「多行 JSX 整段替换」时，`.Remove($m.Index,$m.Length).Insert($m.Index,...)` 这一步把新内容插到了**文件开头**并留下重复块，随后我又用错误的行号切片试图修复，导致 `frontend/src/App.tsx` 一度被截断到 206 行。恢复过程：先 `Copy-Item` 出备份（`.broken`/`.sosave`），再用**行号+字符串定位**找到真实边界（发现第 166 行是 `)}import {FormEvent…}` 被拼接在同一行——即"新块 + 原文件第 2 行起"，原文件第 1 行被并入），据此切回 3454 行完整文件，`tsc --noEmit` 通过。**教训**：① 对整段 JSX 做脚本化替换风险极高，后续改为**先写新片段到临时文件、再用精确边界替换、替换后立刻 tsc**；② 任何批量/脚本写文件前先备份，且**先只做"匹配与边界校验"、确认后再落盘**；③ 不要用推测的数组索引去裁剪大文件。

- **2026-09-09 修复「详情模式右侧空白」+ 取消列表视图（用户反馈）**：用户截图反馈「点击对话或外观/皮肤，右边什么都不显示」。**根因**：上一轮做抽屉迁移时，我用 `{drawerView !== null && (<div className="drawer-panel">…<div className="drawer-body">)}` 包住了一批面板，但把 `{activeView === 'chat' && …}` 与 `{activeView === 'skins' && …}` 两个分支也**一起包进了这个容器内部**——于是当 `drawerView === null`（即用户点"对话"/"外观/皮肤"，此时 `activeView` 是 chat/skins、`drawerView` 为空）时，整个容器不渲染，两个核心页面直接消失。**这也是我上轮"零功能回归风险"判断失误的地方**：当时只想着"不搬移 JSX"，却忽略了 JSX **嵌套位置**会改变渲染条件；`tsc` 不会报错（语法合法），只有实际点击才会暴露。**修复**：把三处包裹容器（`drawer-panel`/`drawer-head`/`drawer-body`）整体移除，所有面板回到 `web-content` 下的**平级分支**，抽屉的定位与玻璃样式改由各分支自身的 `.drawer-content` 类承担（纯 CSS `absolute` 定位，不再依赖容器），因此再也不会出现"某个分支被条件容器吞掉"的问题；各面板的关闭按钮改为内嵌在标题行（复用 `.ghost-button`），删掉原来独立的抽屉头。**顺带按用户要求取消列表视图**：详情模式统一为「房间常驻 + 右侧抽屉」——导航只保留 房间/模型信息/插件管理/后台任务/桌宠日志/设置（移除了「对话」「外观/皮肤」两项，因为聊天就在房间的聊天岛里、形象就在舞台上），并移除房间里那个「⇱ 列表视图」按钮；新建/切换会话后停留在房间而非切到已取消的独立对话页。验证：`tsc --noEmit` + `npm run build` 通过；产物核对 `drawer-content`（JS 5 / CSS 4）、`drawer-close`（JS 5）、旧 `drawer-panel` **归零**；后端 12 包与插件测试全绿。

- **2026-09-09 详情模式：状态岛（用现有数据，无需后端）**：最后一项「状态岛」在规划里被标为"需后端数据支撑"，但**核查后发现并不需要**——`conversations`/`messages` 两张表都有 `created_at`，陪伴天数与互动量都能在现有数据上算出来，于是直接落地而不是挂起等后端。**内容**：舞台左下角玻璃胶囊，含情绪标签（按情绪取色，复用 `--success/--danger/--warning` 与雾蓝令牌，保持低饱和体系）+ 三项统计（陪伴天数 / 对话数 / 当前会话消息数）。**踩到的一个坑（值得记）**：会话列表在仓储层是 `ORDER BY updated_at DESC`，因此**不能取首项或末项**当作"首次对话时间"——用户续聊一个很久以前的会话会让它排到列表前面，取末项会得到错误的（偏晚的）起始时间。改为新增纯函数 `earliestTimestamp()` 在全部会话里**真正求最早值**，并忽略无法解析的项；若一个合法值都没有则返回 `undefined`，调用方据此隐藏该项（避免显示"第 NaN 天"）。同时把情绪与统计做成 `pointer-events: none` 的纯展示层，不干扰舞台点击互动与滚轮缩放；窄屏（≤1000px）下移到顶部避免与底部按钮重叠。验证：`tsc --noEmit` + `npm run build` 通过，产物核对含 `room-status-island`（JS 1 / CSS 2）与 6 条情绪取色规则；全栈回归（Go 12 包 + `go vet` + 插件 9 项 + codex 4 项）全绿。

- **2026-09-09 详情模式：旧视图迁入 Room 抽屉浮层**：最后一个"角色会被切走"的缺口。此前 `activeView` 是**互斥页面**模型——点插件/任务/日志/设置/模型就把整个 `web-content` 换掉，角色连同聊天一起消失，与「角色为王」的定位冲突。**方案选择**：没有把 5 个面板的 JSX 搬进 RoomView（那是 300+ 行搬移、回归风险大），而是**保留 `activeView` 的页面语义、新增 `drawerView` 表示"叠加在 Room 之上的能力面板"**：导航到这类视图时 `activeView` 仍为 `'room'`、目标写入 `drawerView`，于是 **侧栏高亮、`room-active`、`:has(> .room-view)` 等既有判定全都不用改**；5 个分支只把条件由 `activeView ===` 改判 `drawerView ===`，再整体包进 `.drawer-panel` 容器。**代价与取舍**：抽屉是右侧浮层（`min(680px,62%)` + 半透明 + 模糊），**会盖住 Room 右侧的聊天岛**；处理方式是抽屉打开时让 `.room-islands` 淡出右移让位，并在抽屉头提供「✕ 关闭」与 ESC 收起——即"要看工具就先收起工具再看聊天"，而不是把三栏硬挤进 1200px（那样舞台会被压到 ~350px，角色会很小，得不偿失）。**状态一致性**：逐一排查了所有会让 `activeView` 变成非 room 的路径（`newConversation`、`selectConversation`、`onExitRoom`），全部补上 `setDrawerView(null)`，避免"页面已切换但浮层仍开着"的孤儿浮层；ESC 也一并收拾起。**无障碍/动效**：`role="dialog"` + `aria-label`、`drawer-slide-in` 进入动画、`prefers-reduced-motion` 下关闭动画。验证：`tsc --noEmit` + `npm run build` 通过；产物核对 `drawer-panel`（JS 1 / CSS 2）、`drawer-slide-in`、`drawer-open`、`room-active`（5）均存在；后端 12 包与插件测试全绿。

- **2026-09-09 详情模式 P2-25：舞台点击互动**：目标里的「舞台点击互动」落地。**交互设计**：点击房间中置舞台的角色区域 → 随机取一条专属短台词（6 条，含"别戳啦，会痒的嘛。"这类符合人设的反应）+ 对应情绪（`surprised/happy/focused/thinking`，**限定在情绪白名单内**以保证与 `Live2DStage` 的表达式映射一致）+ 620ms 下压回弹脉冲动效 + 台词气泡，3 秒后自动恢复。**踩到并修掉的三个具体问题（都是实现中发现、非事后补记）**：① **事件冒泡**——若把 `onClick` 绑在整个 `.room-stage`，顶部表情 chip、窗口最小化/关闭、底部「房间门廊/列表视图」的点击都会冒泡触发互动；改为只绑 `.room-stage-live2d`，语义也更准确（点角色才反应）。② **定时器覆盖**——初版用同一个 ref 存两个 `setTimeout`（先置真、再置假），后者覆盖前者，导致"置假"定时器永远不被清理；改为让**脉冲生命周期由 effect 自管**（`tapPulse` 为真时启动计时复位，依赖变化/卸载自动 cleanup），从根上消除泄漏。③ **气泡常驻**——`assistantLine` 说完后仍保留值，若只以「有文本」为渲染条件，非说话状态也会常驻气泡；补上 `isSpeaking` 守卫，并把点击反应台词并入同一 `stageLine` 表达式。**状态隔离**：点击反应是**本地临时状态**（`tapReaction`）、不写后端情绪，避免把"被戳了一下"污染成真实情绪；并用 `useEffect(..., [emotion])` 让外部情绪一变化就立刻收回控制权（用户点表情 chip 或 LLM 情绪事件都能即时覆盖）。**无障碍**：支持 Enter/Space 触发与 `:focus-visible` 焦点环。验证：`tsc --noEmit` + `npm run build` 通过，产物核对含 `is-tapped`（JS/CSS 各 1）与 `room-stage-tap`（CSS 2）；后端 12 包与插件 9 项 + codex 4 项全绿（本轮纯前端，未触碰后端）。

- **2026-09-09 情绪→台词对齐通道 + `send_service.go` 死代码清理**：本轮先想修「同步发送路径超长时把整条回复（含已播出的句子）丢弃」这一记录不一致问题（属于拟人化域的「AI 说的 ≠ 实际说的」），**但在动手时发现 `SendService.SendGuidedReply` 已无任何调用点**（`sender` 字段仅被构造、从未调用），即该文件早已被流式路径 `streamReply` 完全取代。**关键判断**：在死代码里改行为没有价值，正确做法是清理它——于是把仍被复用的纯函数（`postprocessReply`/`splitReply`/`isSentenceBoundary`/`trimSentencePart`/`nonEmpty`）迁到新文件 `reply_text.go`（自包含，不再跨文件隐式依赖 `stream_reply.go`），删除 `send_service.go`（约 108 行）与 `Service.sender` 字段及其构造。**教训**：我原本的"修复"落在一个不可达路径上——说明**动手前必须先确认代码是否活着**（`grep` 调用点），否则会白改甚至留下半新半旧的死代码。

  随后做本轮真正的拟人化收益项：**把 Planner 已决定的情绪显式传给 Replyer**。此前 Planner 负责产出情绪（供前端驱动表情），Replyer 才写台词，但 Replyer 的提示词只给了情绪**取值范围**并要求"让每句情绪匹配内容"，**没有告诉它本轮该是什么情绪**——于是 LLM 只能逐句重新猜，容易出现"台词内容与该有的情绪对不上"或"台词与前端表情不一致"。新增纯函数 `formatEmotionDirective(decision)`：当 Planner 给出情绪时，注入一行 `performance_directive`（含 emotion、mood，并把 valence/energy/dominance 转成**自然语言提示**如"偏积极/情绪激动/自信主导"，比裸数字更利于模型把握语气），并明确要求台词用词与语气和该情绪一致；情绪为空时不注入（避免空指令行）。测试：新增 `TestFormatEmotionDirective` 覆盖空情绪不注入、情绪+基调格式、高唤醒正向/低唤醒负向的提示、**中性区间不误加提示**、纯空白情绪。验证：`go build ./...` + `go vet ./internal/...`（已归零）+ `go test ./internal/...` 12 包全绿。

- **2026-09-09 顺带修复 `go vet` copylocks（`config.ApplyJSON`）**：在验证反应停顿改动时发现 `go vet ./internal/...` 有两处 `copylocks` 报错（`config.go:298/303` 的 `temp := *c` 与 `*c = temp`）——`Config` 内含 `sync.RWMutex`，整体结构体复制既违反 vet 检查，也把锁状态带进副本（持锁复制锁属未定义行为）。**这不是本次新引入的问题**，但它使 vet 长期不绿、会掩盖将来真正的新问题，因此一并修掉。**做法**：`ApplyJSON` 改为**从零值 `Config{}` 开始逐字段装配**（先拷入当前值做反序列化基底 → 应用 JSON → 保留 `filePath` → `ApplyFrom` 回写业务字段），彻底不做结构体整体复制；新增 `ApplyFrom` 集中承载"哪些字段属于业务字段"这一知识。**配套防回归**：新增 `TestApplyFromCoversAllFields`（**反射**遍历 `Config` 全部带 json 标签的字段，逐一比对 src/dst，遗漏即失败）——因为逐字段拷贝最大的风险是"将来新增字段忘记补进 ApplyFrom"，那会导致该字段**静默丢弃**（表现为"配置改了不生效"），与本文档反复强调的静默失效同类；另加 `TestApplyJSONPreservesFileFieldsAndParses` 验证部分覆盖语义（未出现的字段保留、`filePath` 不丢、确实落盘）。验证：`go vet ./internal/...` 归零、`go build ./...` 通过、12 包测试全绿。**教训**：vet 不绿本身就是技术债——它会训练人忽略警告，进而放过真实缺陷。

- **2026-09-09 拟人化 P0-②：反应停顿（thinking pause）**：开始按 [`REALISM-ANALYSIS.md`](REALISM-ANALYSIS.md) 清单落地。**为什么先做这一项**：它是清单里**唯一零行为风险**（不改变"是否回复"的语义、不引入新模型调用、不增加成本）却能立刻改善"机器感"的一环，且与分析结论第 2 条根因（全链路零延迟）直接对应。**实现要点与取舍**：① **只停首句之前**——停顿若加在每一句之间会破坏"边生成边说"的流水线（首句延迟收益会被抵消），而真人对话的"思考感"主要来自**回答开始前**的那段间隙；② 用 `sync.Once` 而非布尔标志——因为 `streamReply` 有**两条产出路径**（结构化 `dialog` 分支与 flat-text 回退分支），用 Once 可保证两路合计只停一次，避免以后新增路径时漏加或重复加；③ **可取消**：`waitThinkingPause` 用 `select` 监听 `ctx.Done()`，用户打断（barge-in）时立即返回，否则会出现"被打断后仍被延迟卡住"的新 bug；④ **零值安全**：`ThinkingPause` 对 `min/max<=0`（关闭）、`max<min`（归一化）、随机数越界（折回 [0,1)）都做了定义，并抽成**纯函数**以便单测；⑤ **默认值温和**：250–900ms（随机），刻意不取更大值以免语音交互显得迟滞；`Load()` 采用「先 `DefaultConfig()` 再 `Unmarshal`」，因此**现有用户配置无需改动即生效**，想回到旧行为把它置 0 即可。**接线防静默失效**：除边界单测外，新增 `TestStreamReplyAppliesThinkingPause`——用 **AST**（非字符串匹配）确认 `flushPart` 与 `flushDialogItem` **两条闭包路径**都调用了 `applyThinkingPause`，并确认时长确实引用配置字段而非硬编码；**并做了变异验证**：临时删除 dialog 路径那一行调用后测试如期失败并精确指出 `flushDialogItem`，恢复后通过——即该断言确实能捕获静默失效，而非形式主义。验证：`go test ./internal/...` 12 包全绿；`configs/config.example.json` 已同步字段便于用户发现与调整。

- **2026-09-09 拟人化差距分析（产出 `docs/REALISM-ANALYSIS.md`）**：用户反馈"感觉 Yuyu 跟 Neuro 那种 AI 项目还是有差距，不像真人"，要求联网研究并把分析写成文档。**做法**：并行两路联网深挖（Neuro-sama 复刻实现 / MaiMBot 情绪与语气）+ 本地逐行核查本项目。**本地诊断（关键收获）**：问题**不在模型能力，而在工程约束**——① `turn_gate.go:45` 的 `private_session` 白送 0.60 > 阈值 0.45 → 用户每条消息必回、永不沉默；② 全链路无打字/思考延迟（`MinReplyIntervalSeconds=0`），且 `AllowTypoSimulation` 是**全仓库无消费点的死配置**；③ 情绪无状态（`emotion.go` 只做白名单+钳制，每轮 Planner 现算，无衰减/惯性，前端 `emotionEngine.ts` 也无平滑）；④ 主动发言 `companion.go:584` 是**硬编码模板、不经 LLM**；⑤ 长期记忆**只读不写**（`query_memory` 可召回，但对话过程无自动抽取，`AddMemoryCandidate` 仅 Wails 导出）；⑥ prompt 硬性"1–2 行" + 清洗正则删除所有括号 → 过度书面化、无冗余；⑦ ≤40 字闲聊走快速通道跳过 Planner，情绪退化为关键词。**外部对标收获**：Neuro 复刻用「5 维人格特质向量 + 6 态情感状态机 + 认知帧 + 30% 探索率」演出人格，并有一条极高性价比的做法——**打断时把 assistant 末条改写为"实际已播出内容 + …"并写入 `[Interrupted by user]`**，让 AI 只记住自己真说过的话；MaiMBot-Classical 用 **valence/arousal 二维值 + 指数不对称衰减**（valence→0、arousal→0.5，`exp(-k·dt)` 连续时间），MoFox 分支进一步用**概率门控（基础 5%、只对"感兴趣"的消息更新）+ 180s 无互动触发"冷静"**，且**情绪只注入 Planner（决策层）不改采样参数、只用模糊修饰语**；错别字模拟器被源码证实是有效的"不完美"手段。**方法论要点**：把社区流传的 GitCode 分析博客识别为 **AI 生成稿**——其"应答意愿 = f(唤醒)×g(愉悦)"等属**建议而非实现**，已从文档剔除并加来源更正；同时发现标杆自身有真实缺陷（情绪更新因 `emotion[0]` 取字符串首字符 + 未知 key 静默 return 而**整条链路空转无报错**），作为反面教训收录。**产出**：7 条根因（附行号）+ 横向对比表 + 25 项可执行清单（P0/P1/P2）+ 单用户场景"不要做"清单（willing/Focus/发言频率等群聊机制）+ 来源索引。**文档纪律**：全部结论区分 `[事实]`/`[外部事实]`/`[推断]`，本项目结论可按 `文件:行号` 逐条复核。

- **2026-09-09 配色收敛 + 高级感 + 丝滑交互**：用户反馈「配色太花里胡哨，但也不要太古板简朴，要高级感和丝滑的组件交互」。**诊断**：问题不在主色，而在**令牌形同虚设**——`App.css` 里 `--accent` 只被少数处引用，同时存在 ~60 处硬编码 `rgba(255,94,168,*)`（高饱和粉）、69 处 `linear-gradient`、12 处 `repeating-linear-gradient` 网格纹理，以及粉/紫/蓝多色相彩色阴影，叠加后就是"廉价花哨"。**处置**：① 收敛色相——主色由 `#c978ad`（糖果粉）改为 `#b06e93`（雾玫瑰），辅助 `#7d9ec0` 灰蓝/`#7fa99b` 雾青，底色 `#fbf7fb`→`#f7f6f8`，文字 `#33283d`→`#2b2732`，语义色（成功/警告/危险）统一降饱和并补 soft/line 令牌；② 删掉全部装饰纹理（`repeating-linear-gradient` 归零），大面积背景只留一层柔和渐变；③ 彩色光晕改**中性阴影**三档（`--shadow/-soft/-lift`），高级感来自层次而非光；④ 令牌化——43 处旧色值 + 66 处透明色 rgba 全部替换为 var()，并在 `:root`/`.app-shell.web` 补齐 `--accent-strong/-soft-2/-line/-glow`、语义色变体、`--ease/--ease-out/--dur-fast/--dur/--dur-slow/--ring`；⑤ **丝滑交互层**：统一 `transition` 到令牌时长与缓动，按钮 hover 上浮 1px、卡片 2px、按下 `scale(.985)` 回弹、`:focus-visible` 用 `--ring` 光环，只动画 transform/opacity/shadow/color（GPU 友好、不触发布局），并加 `prefers-reduced-motion` 兜底把位移/缩放全部关掉。**同步 `DESIGN.md`**（front-matter 色值/圆角/新增 motion 段；正文补充两条硬规则：无装饰纹理、阴影中性、组件必须引用令牌）。验证：`tsc --noEmit` + `npm run build` 通过，CSS 变量完整性 40 项引用全部有定义，残留纹理 0、残留高饱和 rgba 0。**经验**：设计令牌若不强制引用就必然退化，新增颜色必须先入令牌层。

- **2026-09-09 netease-music 自动连播（v0.2.0）**：补齐 README「已知待办」里的自动连播。**难点**：sidecar 是无状态短生命周期进程（宿主按需拉起、idle 可能被回收），不能用定时器自己驱动"播完切歌"；而真正持有 `<audio>` 的是 React 前端。**解决办法**：确立「**队列与判定在 sidecar、切歌动作在消费者前端**」的分工——① sidecar 维护 `state.queue`（点播搜索结果的快照）+ `queueIndex`，把「还有没有下一首」通过 `metadata.autoNext` 和 `metadata.queue{index,size,source}` 声明出去；② 前端在音频 `ended` 且 `autoNext===true` 时发一次 `下一首`（`playMusicUrl` 的 `onended` → `nextMusicTrack()`），并用 `musicPlayIdRef` 世代号守卫，保证同一首歌只触发一次、用户手动切歌后旧回调不生效；③ sidecar 到队尾时用 `/simi/song` 相似歌曲补歌续播，靠 `filledFromSimilar`（同一 seed 只补一次）与 `queueSize` 上限防止无限扩张，相似也拿不到就回 `已经是最后一首了。` 让前端安静停止。**顺带修掉一个真实缺陷**：`createController` 原先直接用外部传入的 cfg，绕过 `loadConfig` 的字段兜底，导致 `cfg.autoNext` 为 `undefined`、连播标记恒为 false（测试与嵌入方都会踩）；抽出 `normalizeConfig` 由两处共用。测试新增用例覆盖「队列保留完整搜索结果 / 队中 autoNext=true / 队尾 false / 相似歌曲续播 / 无相似明确提示 / prev 队首边界」，9 项全绿；文档同步 README 自动连播章节 + 契约字段 + configSchema + AGENT.md。
- **2026-09-09 详情模式（Room）UI 重构首轮**：让 Live2D 角色回到 web 详情主界面（此前角色只在桌宠窗口/皮肤预览出现）。新增 `RoomView.tsx`（中置舞台 + 表情 chips + 聊天玻璃岛 + 音乐岛 + 舞台内窗口按钮）、`MusicIsland.tsx`（可折叠；点歌自然语言输入/正在播放卡/暂停停止/搜索列表）、`musicTypes.ts`（netease-music 返回契约的类型化）。`activeView` 新增 `'room'` 并设为默认；room 下 web-shell 全宽、侧栏变「房间门廊」悬浮浮层（`sidebarVisible`）。**关键难点与决策**：音乐播放原实现挂在语音管线（`handlePluginPlaybackResult`→`audioRef`+`voiceStatus='speaking'`），会触发唇形同步、被 barge-in 当 TTS、与逐句 TTS 抢播放器。本轮改为独立 `musicAudioRef`+`musicPlaying` 通道（`playMusicUrl/pauseMusic/resumeMusic/stopMusic`），`stopCurrentAudio()` 会连带停音乐实现两通道互斥，删掉旧的 `playPluginAudioUrl` 死代码。沿用「不引第三方 UI 库」约束，Room 样式追加在 App.css 独立段（与 DESIGN.md 玻璃 token 一致），工程规模可控且保留旧视图可回退。验证：`tsc --noEmit`、`npm run build`、netease-music 8 项 + codex 4 项 Node 测试全绿。

- **2026-09-09 netease-music 插件落地**：目录插件从“只有 plugin.json/config.json 的空壳”补全为可运行插件——`main.js`（sidecar：NeteaseCloudMusicApi 客户端 + `createController` 状态机 + stdin JSON-RPC 壳 + 自然语言意图解析）、`main.test.js`（8 项回归全绿）、README、plugin.json 契约更新。确立「sidecar 出指令、前端出声」契约（见难点 12）：`metadata.playbackAction`(play/pause/resume/stop) + `playbackUrl` 驱动前端 `<audio>`；前端 `invokePluginAction` 改走 `handlePluginPlaybackResult`，netease-music 详情页新增“音乐播放”自然语言输入框。前端 `tsc --noEmit` 通过；真实出声需用户本机启动 NeteaseCloudMusicApi（默认 3000 端口）后在桌宠插件页验证。

- **2026-09-09 沙箱复测纠正**：旧隔离测试仓库最初由 CodexSandboxOffline 创建，当前用户启动的 CLI 无权调整其 ACL（日志 SetNamedSecurityInfoW error 5），这属于测试夹具所有权问题。改为当前用户创建独立副本后，以继承 `:workspace` 的临时权限 profile 执行 `codex sandbox`，实际写入并读回探针文件成功；未关闭沙箱、未修改原项目权限。完整模型测试仍被当前 cc-switch 本地转发 502 阻断，不能以探针通过宣称模型逐块写入或 VS Code 高亮通过。

- **2026-09-09 实时变更事件修复**：上一轮隔离 sidecar 实测被本地 Codex provider 的 502 阻断，没有真实写盘，因此不能宣称实时评审通过。本轮确定性回放复现两个问题：`jsonEventText` 只看顶层而丢失 `item.type=file_change`；轮询只比较 `kind:path` 而漏报同一文件后续修改。修复为解析嵌套事件（保留 item id 区分独立补丁）、总结提取 `agent_message.text`、StringDecoder 保留跨 chunk 中文字节，并按全部变更文件的 size/mtime/ctime 检测重复写入，展示仍最多 20 项。`error/turn.failed` 立即发进度，超时 note 包含最近错误，终止失败不能被 exit 0 掩盖。4 项 Node 回归测试覆盖嵌套补丁、中文、同大小写入及第 21 个文件、连接失败、纯文本兼容。插件继承本机 Codex provider，未自动修改用户全局配置；真实写盘/VS Code 增量差异复测需先恢复连接。

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
- **前端逐句 TTS（流式通道接通）**：`App.tsx` 新增 `ChatStreamEvent` 类型 + `DESKTOP_COMPANION_CONVERSATION_ID` + 流式状态 refs（`streamSentenceQueueRef`/`sentencePlayingRef`/`streamReplyActiveRef`/`streamDoneRef`/`chatEventHandlerRef`）；`chat:event` 订阅用「最新 ref」模式避免闭包过期；`sendContent` 从 `SendMessage` 切到 `StreamChat`；`finishSpeaking` 接逐句 drain（下一句接着播、done 后清理并走 relisten）；`handleChatEvent` 处理 token（逐句 speakText）/emotion（即时驱动）/error（abortStreamReply）/done（completeStreamReply：结束发送态+刷新消息+scheduleFollowUp）；`App.StreamChat`（app.go）补 `ensureCompanionReady`。`go build ./...` + `go test ./internal/...`（12 包全绿）+ `tsc --noEmit`（exit 0）。渲染/延迟效果需本地 `wails dev` 验证。
- **GPT-SoVITS 本地 TTS provider（音色复刻）**：`config.go` 新增 `Speech.Provider`（`fish_audio`/`gpt_sovits`）与 `Speech.GPTSoVITS`（base_url/endpoint/refer_audio_path/prompt_text/prompt_lang/text_lang）；新增 `internal/app/gpt_sovits.go`（`synthesizeGptSovitsSpeech` POST JSON → 可配 endpoint；`parseGptSovitsResponse` 兼容 api_v2 的 `data[0].audio` base64 与 api.py 原始 WAV）；`SynthesizeSpeech` 按 provider 路由；`gpt_sovits_test.go` 覆盖两种响应格式 + 空响应。新增 `docs/GPT-SOVITS-GUIDE.md`（训练 + 接入步骤）。`go build ./...` + `go test ./internal/app ./internal/config` 通过。音色复刻训练需用户本机（RTX 4060 Ti）跑 GPT-SoVITS。
- **LLM 首字延迟优化（Planner 快速通道）**：定位「LLM 回复慢」的根因——两段式 Planner（全文 JSON 一轮）→ Replyer（首字）串行，首字前多一整轮。落地：`shouldSkipPlanner`（保守判断简单闲聊 → 跳过 Planner 直接流式，`fast_path.go` + 单测）+ `InferEmotionFromText`（快速通道兜底情绪，词组级关键词）+ 情绪事件移到流式文本之前（`EventTypeEmotion` 早于 `EventTypeToken`，前端逐句朗读前先驱动表情）+ Planner/Replyer 加 `model.WithMaxTokens(256/600)` 约束输出 + 前端无 mood 时清空 `performanceHint` 回退文本启发式。`go build ./...` + `go test ./internal/...`（12 包全绿）+ `tsc --noEmit`（exit 0）。
- **修复「发消息不回复」**：根因是 TurnGate 回复门控过激——`private_session` 基础分 0.38 低于阈值 0.45，导致非提问、非@、且距上一条 bot 回复 <8s 的普通陈述句（如「你好」「今天好累」）被门控不回复；`bot_streak_penalty` 又在连续回复后进一步压低。修复：基础分 0.38→0.60（直接消息必回复）、删除 `bot_streak_penalty`（语音噪声已由前端 `isUsableVoiceTranscript` 上游过滤）、新增 `looksLikeWeakBackchannel`（「好的/嗯/哦/知道了」等极短应答词 -0.30 落到阈值下，避免无意义复读）。`chat_test.go` 增加「普通陈述句在 streak 下仍回复」用例。`go test ./internal/...` 全绿。
- **逐句预合成（消除句间空档）**：语音「慢」的另一来源是句与句之间——上一句播完才现场合成下一句，产生 0.5~1s 空档。落地：`playSpeechReply` 从 `speakWithBufferedCloudVoice` 抽出「播放已合成 base64 音频」；`startPrefetch` 后台 peek 队首并预 `SynthesizeSpeech`（`prefetchedSpeechRef`/`prefetchedTextRef`/`prefetchInFlightRef`）；`finishSpeaking` drain 优先播「已预合成且对应队首」的句子、否则现场合成，并清理过期预取（避免双重合成/丢句）。`tsc --noEmit`（exit 0）通过。
- **GPT-SoVITS 流式合成（消除句间停顿）**：实测 GPT-SoVITS 每短句合成约 2s（GPU），短于短句播放时长，句间预合成常来不及 → 停顿。落地：后端 `GetSpeechStreamUrl` 返回带 `streaming_mode` 的 GET 流式 URL（`<audio src>` 渐进播放，首块更快、近零句间停顿，新增 `speech.gpt_sovits.streaming_mode` 配置，默认 1）；前端 `ensureStreamSupported`/`preloadStreamAudio` 提前预载流、`playStreamAudio` 播放、`playQueueHead` 统一 drain（流式优先 → base64 兜底），`finishSpeaking` 改为委托 `playQueueHead`；`discardPrefetchAudio` 放弃未播放的预载流避免占资源；`GetSpeechStreamUrl` 加单测。`go test ./internal/...`（12 包全绿）+ `tsc --noEmit`（exit 0）。流式播放/预载需本地 `wails build` 重新生成绑定并重建二进制验证。
- **对齐 Shinsekai 的「流畅」三件套（本机改动，未推送）**：分析 [Shinsekai](https://github.com/RachelForster/Shinsekai) 后确认其流畅来自「一句台词 = 流水线最小单位 + 线程/队列真并行 + 单轮直出（情绪随台词走）」，而非字节级流式 TTS。落地两点：① **去掉简单闲聊的 Planner 双轮**——`shouldSkipPlanner` 长度上限 24→40 字符，更多 1-2 句闲聊直接走 Replyer 流式，压缩首字延迟（复杂意图仍由信号词拦下走 Planner）；② **情绪逐句下发**——`streamReply` 新增 `refineSentenceEmotion`（内容带明显情绪、与上次不同且非中性才下发），每句前下发 `EventTypeEmotion`，让 Live2D 逐句反应、表情随台词走（前端已支持该事件，无需改动）。TTS 并行为「流式播放 + 单句 lookahead 预载」：当前句播放时下一句已并发地拉 GPT-SoVITS 流（双流重叠），故未再引入风险较高的双 lookahead。`go test ./internal/...`（12 包全绿）。
- **修复「回复读不完整/截断」（根因：text_lang 被误设日语）**：排查发现前端 `DEFAULT_SPEECH_LANGUAGE` 默认 `'ja'`，导致 `SynthesizeSpeech(text,'ja')` 把 `text_lang` 覆盖成 ja，GPT-SoVITS 用日语 G2P 读中文文本 → 乱码/截断。修复：① 前端 `DEFAULT_SPEECH_LANGUAGE` 默认改成 `'zh'`（匹配中文人设+中文回复）；② 后端 `synthesizeGptSovitsSpeech`/`GetSpeechStreamUrl` 新增 `DetectTextLang`（含假名→ja，含汉字→zh），按**文本实际脚本**强制 `text_lang`，即使前端传错语言也能正确朗读。`DetectTextLang` 加单测。`go test ./internal/...`（12 包全绿）+ `tsc --noEmit`（exit 0）。另：流式合成（`VITE_ENABLE_GPT_SOVITS_STREAMING`）默认关闭以保稳定，避免浏览器对 chunked WAV 播放不可靠导致的不出声。
- **修复「偶发某句不读」（句末无标点 + 合成瞬失败）**：排查发现偶发不读的多为**句末无句号/问号**的台词（如「主人就是您呀~」）。这类句子只能等到流结束 flush 才 emit，紧贴 `done`，前端此时易被 drain 丢一句；且 GPT-SoVITS 偶发瞬时失败也会静默丢句。修复：① `isSentenceBoundary` 增加 `~`/`～`，让「~」结尾的台词在流中即切成句、更早下发（切分也更贴近可见气泡）；② 前端 `speakWithBufferedCloudVoice` 合成失败**重试一次**再放弃，避免瞬时失败导致静默丢句。均加/更新单测，`go test ./internal/...`（12 包全绿）+ `tsc --noEmit`（exit 0）。
- **修复「首句/某小句没声音」（最终：去掉 Web Audio 路由，仿 Shinsekai）**：前几轮在 AudioContext 上兜底（unlock + running 判定）仍不稳定。最终按用户要求**仿照 Shinsekai**——Shinsekai 的 `SoundPlayer` 只用 `new Audio(url).play()` 走浏览器默认输出，从不把音频路由进 Web Audio。Yuyu 之前 `attachLipSync` 用 `createMediaElementSource(audio)` 把 `<audio>` 永久路由进 AudioContext，context suspended（首句刚创建、无近期手势）时整句静音。最终：**彻底移除 `createMediaElementSource`/Web Audio 路由**，TTS 音频走默认输出保证一定有声音；口型改为**基于播放时间的简易动画**（`setMouthLevel` 用正弦+噪声近似，不再做音频分析）。`tsc --noEmit`（exit 0）+ `npm run build` 通过。
- **修复「读不全（后续句被丢）」（根因：playQueueHead 里 playbackId 取值时序错）**：`playQueueHead` 的流式/base64/等待各分支里，`pbId` 是在 `stopCurrentAudio()` **之前**取的；而 `stopCurrentAudio()` 会 `playbackIdRef.current += 1`。于是播放该句时传入的是旧 id，`audio.onended → finishSpeaking(旧id)` 命中守卫 `playbackId !== playbackIdRef.current` **提前 return**，drain 链就此中断 → 队里后续句子永远不被播（首句能播、后续丢）。修复：所有分支改为**先 `stopCurrentAudio()`、再取 `const pbId = playbackIdRef.current`**。`tsc --noEmit`（exit 0）+ `npm run build` 通过。
- **修复「短片段被 GPT-SoVITS 哼声」（根因：尾随逗号 + cut5 过度切分）**：实测「这一声主人」单独合成正常（1.86s 文字），但「这一声主人，」（带尾随逗号）只有 0.94s 哼声——因为合成请求不带 `text_split_method` 时 GPT-SoVITS 默认 `cut5` 会在逗号处切分，把「这一声主人，」切成短片段而哼声。修复：`isSentenceBoundary` 增加全/半角逗号 `，、；,;`（应用侧也在逗号处切句）+ 新增 `trimSentencePart` 去掉片段**尾随逗号**，使 GPT-SoVITS 收到无尾随逗号的干净短语（如「这一声主人」）从而正常读出。`go test ./internal/...`（12 包全绿）。
- **修复「过短片段被打哼声」（回退过度切分 + 设 cut1）**：上一轮按逗号切句过于激进，把「主人」「那里」等 2 字短片段单独下发，GPT-SoVITS 把它们打成轻哼（实测「主人」仅 0.78s、「那里」1.34s）。而整句用 `cut5` 默认会过度切分（读得最少 9.34s），`cut1`/`cut0` 读得更全（10.46s/11.42s）。回退：`isSentenceBoundary` 只保留强句界 `。！？.!?\n`（整句作为一个 chunk 下发），并新增 `ttsSplitMethod="cut1"` 发给 GPT-SoVITS（不按逗号激进切分、对整句朗读更稳），避免短片段被哼声/丢读。`go test ./internal/...`（12 包全绿）。
- **Replyer 结构化输出（模型逐句情绪/动作，对齐 Shinsekai）**：让 Replyer 不再输出纯文本，而是流式输出 `{"dialog":[{"speech","emotion","mood","energy","valence","dominance","gesture","hand"}]}`——每一句台词自带自己的情绪/动作参数，使 Live2D「表情随台词走」而非仅由 Planner 一次性给出。落地：① `agents.go` Replyer 系统提示改为要求输出该 JSON（中文、2-4 句短台词、每句选匹配的情绪/表演参数）；② 新增 `internal/chat/dialog_parser.go`（`DialogItem` + `dialogStreamParser`，仿 Shinsekai 用引号感知+括号深度扫描，逐个完整 `{...}` 归一化为项，含 `completeJSONObjectSpan`）——`streamReply` 改用 dialog parser，对每个 item 先下发其自带 `EventTypeEmotion` 再下发 `EventTypeToken` 并持久化；若模型没输出 JSON（flat text）则回退到原有按句切分（`refineSentenceEmotion` 启发式兜底），保证 TTS 不受影响；③ `dialog_parser_test.go` 覆盖非流式包装/流式逐项/flat-text 回退/未闭合 JSON。`go test ./internal/...`（12 包全绿）。前端已支持 `EventTypeEmotion` 逐句，无需改动。
- **模型逐句 gesture/hand 驱动 Live2D（表情+动作同步）**：结构化输出已让每句带 gesture/hand；此前前端只在 mood/energy/hand 上推导动作，且 `avatarPerformance` 未透传 `performanceHint.gesture`。本次：① `AvatarPerformance` 增加 `gesture` 字段（Live2DStage 类型 + defaultPerformance）；② `avatarPerformance` memo 与 `inferAvatarPerformance` 透传 gesture；③ `Live2DStage.gestureKindForPerformance` 优先用模型显式 gesture（合法值），否则回退按 mood/emotion 推导；④ `updateGesturePhrase` 触发 key 加入 gesture，同 mood 下显式 gesture 变化也能触发新动作。`tsc --noEmit`（exit 0）+ `npm run build` 通过。
- **改为「古灵精怪、调皮话多」人设 + 回复像真人说话（无动作/心理描写）**：把 `chat.persona`（配置文件 + `config.go` DefaultConfig + `config.example.json`）改成古灵精怪、话多调皮的小恶魔，语气轻松俏皮、妙语连珠，但仍以「主人」称呼。`style_notes` 加硬性要求：语言自然口语化、纯粹是「说出来的话」，严禁任何动作/心理/神态描写（（笑）（歪头）（开心地）心想 看着主人 笑了笑 眨了眨眼睛 顿了顿 等）。Replyer 系统提示（`agents.go`）同步强化该要求并让每句情绪/动作偏活泼调皮；`postprocessReply` 增加行内动作/心理描写的剥离（`inlineStagePattern`），并作用于结构化路径的 `speech`（`flushDialogItem`），双保险确保最终文本无动作/心理描写。`go test ./internal/...`（12 包全绿）。
- **前端二次元浅色重构 + 第一轮解耦**：使用 frontend-design-premium 建立生产 UX 契约，新增 `DESIGN.md`（Yuyu candy studio：莓粉/汽水蓝/薄荷绿/奶油白，混合桌宠品牌感与工具页扫读效率）、`UX-CONTRACT.md`（Select/Listbox 原生 ownership、全局滚动条、轻量表单规则）和 `premium-ui.json`。视觉上将 web 详情页从深色控制台改为浅色缤纷工作室，保留 diff/code/result 的深色技术块以保证对比；新增全局滚动条、focus-visible、reduced-motion，配置 textarea 改为固定尺寸内部滚动。结构上不一次性拆语音/TTS 状态机（播放队列、barge-in、预合成很脆），先迁出低风险纯展示和纯工具：`appConfig.ts`、`appTypes.ts`、`utils.ts`、`components/AppShell.tsx`，包括 ChatComposer、PetModeView、WebSidebar、ChatView、SkinsView、ModelView；`App.tsx` 继续作为状态编排层，后续可再拆插件/任务/日志面板与语音 hooks。验证：`node node_modules/typescript/bin/tsc --noEmit`、`npm run build`、`audit_project.py --mode strict --no-write` 全通过；`npx -p @google/design.md designmd lint DESIGN.md` 经提权下载后 0 errors，仅报告 7 个 orphan token warnings（语义 token 已在 CSS 中使用，lint 未追踪 prose/CSS 映射）。
- **code-assistant 的 VS Code 可视化评审**：用户希望像常用 agent 插件一样直观看到「哪些文件改了、每个文件哪里增删」。实现取舍：不做 VS Code 扩展、不依赖私有 API，而是把 Codex 结果落成标准 git 工作区状态，让 VS Code Source Control 原生接管。`run_agent` 结束后 `reset --soft baseCommit + git add -A`，HEAD 回到基线，改动保留为相对基线的 staged changes，并返回 `cwd/baseCommit/baseBranch/branch/files/diff`。用户实测“查看变更结果啥都没有”的根因有四层：① 变更项可能是目录/嵌套 git 仓库（截图里 `react-skeleton`/`react-website` 是目录级变更），旧逻辑拿目录当普通文件传给 `code --diff`；② 变更信息放在插件详情页，而实际编码由后台任务触发，任务结果只保存模型总结，结构化 diff 可能被模型吞掉；③ 后台任务把 `files` 作为对象数组传回 `open_changes`，插件旧版按字符串路径过滤，`String({path:"x"})` 变成 `[object Object]`，导致 `diffPairs()` 筛成空列表；④ 新生成的 React/Vite 子项目可能自带 `.git`，父仓库只能看到一个 gitlink/目录级变更，VS Code 不会高亮内部文件。修复：`beginReview()` 记录基线已有嵌套仓库；`stageReview()` 只把本轮新出现的嵌套 `.git` 安全挪到系统 temp，使父仓库按普通目录 `git add -A`，内部文件级新增/修改即可在 VS Code SCM 和 `code --diff` 里审核；已有嵌套仓库不动。`open_changes` 也会对旧任务做同样的安全补救：只吸收在基线 `HEAD` 中不存在的嵌套仓库目录。`diffPairs()` 同时接受字符串路径和 `{path}` 对象，若传入列表筛空则兜底打开全部当前变更；移除不存在的 `code --command` 调用，普通文件直接用同步 `code --diff` 打开并把 CLI 结果返回给任务页。`LLMExecutor` 专门识别 `run_agent` 工具结果，把结构化 review 写入 `TaskResult.Metadata["code_review"]` 并设置 `NeedReview`，前端后台任务页解析 `result_json.metadata.code_review` 显示变更清单、VS Code 查看、补丁、接受/拒绝。插件详情页改回插件自身说明/工具/动作/配置，避免把任务产物放错位置。验证：`node --check plugins/code-assistant/src/git.js`、`node --check plugins/code-assistant/src/index.js`、直接调用 `reviewFiles()`/`diffPairs()`（含临时嵌套仓库 + 对象数组 files + 新/旧嵌套仓库吸收用例）、`go test ./internal/...`、`tsc --noEmit` 与 `npm run build` 通过。

- Settings workspace root editor: the settings page now exposes app.workspace_root as a first-class input instead of requiring manual JSON edits. Backend SetWorkspaceRoot persists the cleaned absolute root and updates runtime workspace-dependent tools/defaults for subsequent tasks and plugin sidecars.
