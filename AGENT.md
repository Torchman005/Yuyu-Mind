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

- 2026-09-09 配色与交互收敛：原 UI「花」的根因不是主色本身，而是**令牌层只是摆设**——`--accent` 等只被少数处使用，另有 ~60 处硬编码 `rgba(255,94,168,*)`、69 处渐变、12 处重复网格纹理与多色相彩色阴影。本轮把配色统一为「雾玫瑰 + 石墨」低饱和体系（`--accent:#b06e93`、中性暖白底、石墨紫灰文字），**删除全部装饰纹理**、副作用改为**中性阴影**（高级感来自层次而非彩色光晕）、并把散落色值全部令牌化（新增 `--accent-strong/-soft-2/-line/-glow`、语义色 soft/line 变体）。同时新增**统一交互层**：共享 `--dur/--ease` 令牌，按钮悬停 `-1px`、卡片 `-2px`、按压 `scale(.985)`、聚焦 `--ring` 光环，只用 transform/opacity/shadow/color，并补 `prefers-reduced-motion` 兜底。经验：**设计令牌若不被强制引用就必然退化**——新增颜色应先落到令牌层，组件禁止硬编码 rgba。
- 2026-09-09 netease-music 自动连播完成：sidecar 内维护「播放队列」（点播搜索结果快照 + 位置），新增 `next`/`prev` 意图与 `/simi/song` 相似歌曲补歌；返回值扩展 `metadata.autoNext`（队列内是否还有下一首）与 `metadata.queue {index,size,source}`。**自动续播由前端在 `<audio>` `ended` 时触发**（`autoNext===true` 才发一次 `下一首`），并用 `musicPlayIdRef` 做世代守卫防重复触发，去重依赖 sidecar 的 `filledFromSimilar` + `queueSize` 上限。经验：**跨进程边界的能力要"由消费者驱动"**——sidecar 只声明"还有没有下一首"，真正切歌由持有音频元素的宿主前端执行；若让 sidecar 定时器驱动会与其无状态生命周期冲突。
- 2026-09-09 详情模式 UI 重构首轮落地：Room 视图成为 web 详情默认页（`activeView='room'`），Live2D 角色回到画面主角（中置舞台 + 表情 chips），聊天/音乐改为右侧玻璃浮岛。关键架构决策：**音乐播放与语音管线彻底解耦**——旧实现把插件 `playbackUrl` 塞进 `audioRef`/`voiceStatus` 语音链路（会触发唇形同步、被 barge-in 当 TTS 打断）；现改为独立 `musicAudioRef` + `musicPlaying` 状态，语音/音乐互斥（`stopCurrentAudio` 会同时停音乐）。
- 2026-09-09 插件「出声」的边界已明确：sidecar 插件无法自己播放音频（宿主只做请求/响应转发），因此 netease-music 采用「sidecar 返回 `metadata.playbackAction/playbackUrl`，前端 `<audio>` 执行」的契约；sidecar 内仅维护逻辑播放状态（当前曲目/播放/暂停）。这类“插件想驱动 UI 状态”的场景都应走 action 返回值契约，而非让插件直接触达 UI。
- 2026-09-09 实时编码验证：插件 JSONL 解析需读取嵌套 `item.type/changes/text`；仅比较文件名会漏报同一文件后续修改。已修复并补回归测试。真实模型写盘复测仍需可用的本机 Codex provider；此前本地转发返回 502，不能当成 VS Code 刷新失败。

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
11. **情绪/表情系统升级方向参考 [soullink-emotion-sdk](https://github.com/nanlingyin/soullink-emotion-sdk)**：其四大支柱是 `Continuous VAD emotion`（连续效价/唤醒/支配，而非离散关键词）、`FACS/AU synthesis`（动作单元 AU1/AU4/AU6/AU12… 合成表情映射到 Live2D 参数）、`Layered animation`（idle + 情绪 + 手势分层混合）、`Automatic model adaptation`（参数注册表自动适配任意模型）。当前实现已有「layered 雏形」（idle + emotion overlay + gesture + lip sync）与「离散 mood→参数映射雏形」，差距在：情绪是离散字符串 + 单一 `energy`（缺 valence/arousal）、参数名硬编码当前模型（缺自动适配）、来源是关键词/LLM 离散输出（非连续 VAD）。情绪系统 v2 已落地（见 Done）。
12. **回复「快+自然」参考 [Shinsekai](https://github.com/RachelForster/Shinsekai)**：其核心是 `LLM 流式 + 逐句分片 TTS 并行 + 本地 GPT-SoVITS + 情绪即时驱动立绘`。我们两大差距——① 两段式 Planner→Replyer 多一整轮 LLM（可见文本迟迟才产出）；② TTS 全文缓冲 + 云 TTS。后端已改流式 Replyer（逐句 emit），但**前端仍在用 `SendMessage` 收集式调用**，需切到 `StreamChat` + `chat:event` 订阅 + 逐句合成播放才能真正见效。详见 `docs/DEVELOPMENT-NOTES.md` 难点 11。

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

- code-assistant 事件回归使用 `node --test plugins/code-assistant/src/codex.test.js`。区分事件模拟、真实 sidecar 调用与完整桌宠 UI 验证，不能以模拟通过代替真实写盘验证。

11. 本机构建需重定向 Go 缓存到工作区（否则被沙箱拦截）：
    ```powershell
    $env:GOMODCACHE="<repo>\.gomodcache"; $env:GOCACHE="<repo>\.gocache"; $env:GOTMPDIR="<repo>\.gotmp"; $env:GOTELEMETRY="off"
    ```
    `<repo>\.gomodcache`、`.gocache`、`.gotmp` 已加入 `.gitignore`。
12. 全量构建顺序：先 `cd frontend && npm run build` 生成 `dist`，再 `go build .` 或 `make build`。
13. 每次开发完成必须同步更新本文件的 `Done`，并在 `docs/DEVELOPMENT-NOTES.md` 记录思路与难点解决办法。

## Done（已完成）

- [x] 2026-09-11 **拟人化 P3 性格随关系漂移**（依据 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md) 可选实验）：让"关系"从一句 prompt 变成**看得见的行为变化**。**问题**：关系状态做完后只影响一句"你们已经很熟了"的模糊描述——能改语气松紧，但**角色本身没有变化**；真人相处久了说话方式会变（更放松、更省客套、更少解释、玩笑更随口），长期陪伴需要这种"时间感"。**实现**（新增 `internal/chat/drift.go`）：按交互次数分档（0 / 40 / 120+）给出**风格偏移**提示，注入 **Replyer**（而不是 Planner）——风格是"怎么说"，越靠近台词生成点越有效；`applyStyleDrift` 把偏移**追加**在原 `style_notes` 之后而非替换（原有人设风格要求必须保留），未达阈值时返回空串使行为与之前**完全一致**。**最关键的设计约束**：漂移方向必须与人设**同向**——人设是"古灵精怪、调皮、**话少**"，所以只做"更放松/更省字/玩笑更随口"这类**单调**变化，绝不出现"越熟越话痨/越热情"（那会把人设推向反方向）；并专门写了断言禁止词表的测试守这条线。关系**不改变称呼**（「主人」）与底线要求（无动作描写/不暴露 AI），漂移只作用于风格。**验证**：`go build ./...` + `go vet` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit`；新增 `drift_test.go` **6 项**（未达阈值完全不影响行为、档位随交互单调不回退、**漂移方向不违反人设且禁止词表**、偏移追加而非替换、无人设风格时的退化、Service 层读取关系给出漂移）。

- [x] 2026-09-11 **拟人化 P2 记忆自动抽取 + 关系状态**（依据 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md)）：补齐"记得住"与"关系会累积"两件事。**问题**：此前长期记忆只有**手动**写入路径（`UpsertMemory`/`AddMemoryCandidate` 仅作为 Wails 方法导出），对话过程完全不落库——角色"读得到、写不进"，你告诉它的事实与偏好下一轮就没了；同时**完全没有关系概念**，每次对话的语气都像初次见面。**实现一（记忆抽取，新增 `internal/chat/memory_extract.go`）**：`ExtractAndStoreMemories` 用**一次低 token 的 LLM 调用**判断这段对话有没有值得长期记住的**用户**信息（偏好/事实/长期要求），只接受这三类 kind、最多 3 条、用稳定 `key` 幂等覆盖（同一偏好复现时更新而非堆重复）；`parseExtractedMemories` 是容错纯函数（容忍 markdown 包裹、前后多余文字、非法 kind、缺字段、非法输入），抽不到就返回空（大多数闲聊都是空，不灌噪声）。抽取在回复产出后**异步**执行（`extractMemoriesAsync`，带 30s 超时与 panic 兜底），用户不必为"记笔记"等待，失败只记日志。**实现二（关系状态，新增 `internal/chat/relationship.go`）**：`RelationshipState{Interactions, FirstSeenAt, LastSeenAt, FamiliarityScore}` 用 `db.Settings` **持久化**（每 5 次交互落库一次，控制写入频率），进程重启后关系不丢；更新用**客观信号**（交互次数/消息长度/礼貌用语/情绪词，单次上限 3），不额外调用 LLM（关系是慢变量，不值得每轮付模型成本）；`relationshipPromptLine` 按相处阶段给出**模糊描述**（"还不太熟"→"已经很熟，像老朋友"）注入 Planner，与情绪底色同样**不暴露数值**。**刻意的边界**：关系只影响语气松紧与话题深度，**不改人设规定的称呼**（「主人」），避免角色前后不一致。**验证**：`go build ./...` + `go vet` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit`；新增 `memory_extract_test.go` **11 项**（解析的 8 个子场景 + 过短输入不调模型 + 关系累积与单次上限 + **跨 store 实例持久化**（模拟重启）+ 关系描述分档且不含数值 + `ObserveUserTurn` 升级路径）。

- [x] 2026-09-11 **拟人化 P1 口语冗余 / 口误自纠正**（依据 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md)）：让语音输出不像"逐字念稿"。**背景**：模型文本永远干净完整有条理，这本身是机器感来源；真人说话有填充词、会重复字、会半截话。语音场景下这比"错别字"更贴切——用户**听**到的是停顿与语气。**实现**：① 新增 `applyDisfluency(sentence, r1, r2)`（`reply_text.go`）——低概率（填充词 12%、重复首字 6%）注入 `嗯…`/`诶，`/`那个…` 等填充词或"我我"式重复；**刻意规避破坏语义**：句子 <4 字不注入、已带前置标点不注入（避免"嗯…嗯…"）、重复只作用于人称/指示/判断类首字（避免"算算法"）。② 新增 `Service.postprocessSpeech` 作为**下发台词的统一入口**（清理舞台提示 → 按配置注入口语），接入流式的两条出口（`flushPart` 的 flat-text 路径与 `flushDialogItem` 的结构化路径），主动发言同样注入。③ **把死配置救活**：`AllowTypoSimulation` 此前全仓库无消费点（README 里声称的功能从未生效），现改为控制口语冗余、默认 `true`，并在字段注释里说明"名字沿用历史命名以兼容既有配置"。**验证**：`go build ./...` + `go vet` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit`；新增 4 项测试（低随机值注入已知填充词且不破坏原句、高随机值原样返回、短句与已带标点不注入、重复首字只在安全首字上生效）。

- [x] 2026-09-11 **拟人化 P1 概率门控稀疏情绪更新**（依据 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md)，对标 MoFox）：让情绪**稀疏演化**而不是每轮重估。**问题**：此前每轮对话都会让 LLM 重新判定心情并推进状态机——用户说句无关紧要的话心情也跟着变（抖动）、逻辑上"每句话都在重新评估你"，很假。真人大多时候心情是延续的，只在被真正触动时才变化。**实现**：① `emotionStateStore.MaybeUpdate(convID, suggested, interest, r, now)` 用 `概率 = 0.5 × 时间增益(1→2×) × 兴趣倍数(0.5→1)`（夹在 `[0.2, 1]`）决定是否推进；**未命中时心情延续、只按经过时间衰减**（久不互动仍会平复）。随机源由调用方注入 → 行为可确定性测试。② 新增 `ExtractInterest(text)` 用**客观信号**估计触发强度（长度/疑问/感叹/情绪词表，纯应答词「嗯/好的」判 0.05）——刻意不再调一次 LLM，否则门控省成本的意义就没了。③ `Update` 保留为"总是推进"的确定性入口（`r<0` 显式强制信号，不依赖随机数与概率上界的边界比较），生产路径改用 `MaybeUpdate`；`service.go` 与主动发言均接入。**过程中测试抓到的真实缺陷**：`ExtractInterest("今天面试好紧张啊")` 只得 0.3——长度阈值定得太高（12 字）且「紧张」不在情绪词表里，导致一句明显带情绪的话不被识别；已修正阈值并补全词表（紧张/委屈/孤独/崩溃/离谱等）。**验证**：`go build ./...` + `go vet` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit` + `npm run build`；新增 4 项测试（门控未命中时心情延续不得跳变、命中时位移恰为惯性比例、同一随机数下高兴趣能推进而低兴趣不能、`ExtractInterest` 信号分档与"应声 < 有内容消息"）。

- [x] 2026-09-11 **拟人化 P1 情绪模糊修饰语注入 Planner**（依据 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md)）：让情绪以**自然语言底色**影响措辞，而**不改任何采样参数**。① 新增 `emotionPromptLine`（`emotion_state.go`）——把 `EmotionVector` 转成**刻意模糊**的一句描述（"你现在心情很好，情绪比较激动。这是你此刻真实的情绪底色，回复时自然地带上它，但不要刻意强调或解释自己的心情。"），按 valence/arousal/dominance 分档措辞；**绝不暴露数值**——否则模型会开始机械执行数字，而不是"带着心情说话"。② `PlannerAgent.Plan` 新增 `currentEmotion` 参数，把该描述作为 `current_mood_background` 注入 user 段，并在 system prompt 说明用法与**情绪惯性**要求（"不要因为一句话就翻到相反情绪，要逐渐移动"）。③ `StreamChat` 先取 `CurrentEmotion` 再进 Planner（让决策有上轮惯性作参照）；**快速通道同样继承情绪底色**，不再只凭关键词重掷情绪。④ 新增 `Service.CurrentEmotion` 读取状态机当前值。**验证**：`go build ./...` + `go vet` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit` + `npm run build`；`emotion_state_test.go` 扩充至 **12 项**，新增 `TestEmotionPromptLineIsVagueNotNumeric`（7 个分档子用例，逐条断言**不含数值**）、`TestEmotionPromptLineEmptyWithoutState`（无状态时返回空串，不伪造心情）、`TestCurrentEmotionReflectsStore`。

- [x] 2026-09-11 **拟人化 P2 主动搭话（模板 → 两段式生成）**（依据 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md)）：此前主动发言是 `companion.go` 里的**硬编码模板**（"我还记着你刚才说的「…」，要不要继续从这里往下处理？"），每次同一句式——比不说话更伤真实感。改为「**选话题 → 判时机 → 才生成**」：① 新增 `internal/chat/proactive.go`——`buildProactiveTopicCandidates` 从历史里只取**用户说过的话**（最新优先、过短过滤、去重）；`pickProactiveTopic` 用一次**廉价 LLM 调用**（`max_tokens=8`）从候选中挑一件此刻最想聊的，模型可回 `none` 表示不打扰；`writeProactiveSpeech` 再生成一句真正的口语发言（带上人设/风格/话题），并让它走**情绪状态机**以延续情绪惯性。② **判时机用客观信号而非再一次 LLM**（距上次主动发言 <90s 直接跳过、无候选话题跳过）——对标 MaiMBot"用二元 gate 挡在生成前"的思路，但用更省的实现。③ `App.GenerateProactiveMessage` 改为调用该链路，保留 `proactiveFallback` 模板作为**模型/网络不可用时的兜底**（功能整体仍可用），并记录 `lastProactiveSpeechAt`；`SourceKind` 区分 `proactive`/`proactive_fallback` 便于排查。④ **前端修复**：后端判断"此刻不开口"会返回空回复，前端原先仍会 `applyResponsePerformance`+`speakResponse` 并**白白吃掉一次主动发言配额**；现检测空回复后直接返回并**回滚本次时间戳**。**过程中测试抓到的真实 bug**：`writeProactiveSpeech` 的 prompt 里注入的是 `BotName`（"你是 Yuyu"）而**不是人设**，主动发言会丢角色感——已修正为显式注入 `Persona`。**验证**：`go build ./...` + `go vet` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit` + `npm run build` + 插件 9/4 项；新增 `proactive_test.go` **7 项**（候选只取用户发言/最新优先/limit、编号解析、`none` 判定、异常回答不越界、太近跳过、无候选跳过、生成段带人设且清洗舞台提示），用自建 `scriptedModel` 覆盖两段式的选择与生成逻辑。

- [x] 2026-09-11 **拟人化 P1 情绪数值动力学**（依据 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md)）：把情绪从「每轮现算的标签」变成「有惯性、会随时间平复的慢变量」。**问题**：LLM 每轮独立给出 `emotion/valence/arousal` 后直接发出，于是会出现"上一秒难过、下一秒雀跃"的跳变——真人情绪不会单轮翻转，也有自己的恢复曲线。**实现**（新增 `internal/chat/emotion_state.go`）：① **时间衰减**——用 `exp(-ln2·dt/halfLife)` 向**不对称基线**回归（valence→0、arousal→0.5，半衰期 3 分钟），按真实经过时间而非轮数计算，所以"离开一会儿回来"情绪会自然平复；② **惯性平滑**——新值只向本轮建议值移动 `inertia=0.35`，单轮无法翻转（实测 +0.9 → 0.27）；③ **最短保持时长**（25s）——刚切换过的离散标签短时间内不再抖动，但**效价位移超过 0.5 时允许立即切换**（避免"刚被凶了还在笑"的滞后违和）；④ 状态按会话隔离、内存态（重启回基线，语义上等同"睡了一觉"）。**顺带修掉一个一致性缺陷**：原先前端表情用平滑值、而 Replyer 的表演指令与消息落库仍读未平滑的 `PlannerDecision`，会出现「表情已经难过、台词却还在雀跃」；现抽出纯函数 `decisionWithEmotion` 把平滑结果**写回 decision**，让表情/台词/历史三者用同一个情绪。**验证**：`go build ./...` + `go vet` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit` + `npm run build` + 插件 9/4 项；新增 `emotion_state_test.go` **9 项**：首次采纳、惯性阻止翻转（含精确位移断言）、保持期内不抖、超时允许切换、强变化绕过保持、一个半衰期后回归基线、会话隔离、越界钳制/非法标签回退、以及 `decisionWithEmotion` 的一致性锁定（防止写回逻辑被误删）。

- [x] 2026-09-11 **拟人化 P1-7 打断记忆一致性**（依据 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md) 中"性价比最高"的一项）：让角色**只记得自己真正说出口的话**。**问题**：流式回复是「生成一句就落库一句」（`stream_reply.go:214`），而 TTS 播放滞后于生成；用户一打断，那些**已生成但还没播出**的句子仍留在历史里，模型于是以为它说过用户从没听见的台词——真人不会这样记账。**实现**：① `db.MessageRepo` 新增 `DeleteMessages`（按 id 批量删，空列表安全 no-op）与 `TailAssistantMessages`（取会话末尾**连续**的助手发言，按 SQLite `rowid` 排序——因为同一轮多条消息 `created_at` 相同，仅靠时间无法定序）；② 新增 `internal/chat/interruption.go`：`Service.RecordInterruption(ctx, conversationID, playedCount)` 保留已播出、删除未播出，并追加一条 `[被用户打断]` 标记（`source_kind=interruption`）让下一轮知道"话没说完就转了话题"（`playedCount<0` 按 0 处理；超出实际句数不误删）；③ `internal/app/app.go` 暴露 `RecordInterruption` Wails 方法，并手补生成的绑定（`App.js`/`App.d.ts`）；④ 前端 `App.tsx`：新增 `streamSpokenCountRef` 在 `finishSpeaking`（**句子真正播完**时）计数、每轮流式开始重置，`recordInterruptionIfNeeded()` 在**三个真实打断点**调用——barge-in 抢话（`interruptWithBargeIn`）、键盘中断正在朗读（`voiceStatus==='speaking'` 分支）、`abortStreamReply` 收尾；并用 `streamDoneRef` 守卫**只在真的没说完时**记录（正常说完后的清理会走同一函数，不该污染历史）。**验证**：`go build ./...` + `go vet` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit` + `npm run build` + 插件 9/4 项；新增 `interruption_repo_test.go`（插入序/边界不跨轮/批量删除）与 `interruption_test.go`（**5 项语义**：保留已播出丢弃未播出、一句没播时清空、超计数不误删、负值钳制、空会话报错），日志实测 `kept=2 dropped=2`。

- [x] 2026-09-11 **拟人化 P0 实施**（依据 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md)）：① **打破必回**——`turn_gate.go` 的 `private_session` 基础分 `0.60`→`0.48`（原先恒超阈值 0.45 → 用户每条必回），弱回撤惩罚 `0.30`→`0.35`，新增 `ChatConfig.AllowSilenceOnBackchannel`（默认 true；显式 false 可回滚旧行为），并在 `NewTurnGate` 处理「Go bool 零值无法区分未设置」的问题（完全未配置时按默认启用；配置路径由 `config.Load` 先填默认再 JSON 覆盖）；弱回撤词表扩展为「好嘞/知道了/明白了/收到/没事儿/OK/好的。」等（剥句末标点后比对），但**带疑问语气（「哦？」「嗯？」）不算弱回撤**——那是追问，必须回复。② **放松表达约束**——Replyer prompt 由「1–2 句、不要 ramble」改为「按情境 1–4 句，允许语气词（嗯/诶/啊/呀/嘛/啦）、轻微重复、自我更正（我是说…）、允许只用短句反应（诶？）」，persona/style_notes 移入 user 段；`config.go` 默认人设同步放宽（允许语气词与口语冗余，长度看情境）。③ **清洗放宽**——`reply_text.go` 的 `postprocessReply` 不再删除**所有**括号内容（会连「那个（我是说）…」这类口语插入语一起删掉），改为只删「整行纯舞台提示」与「行首提示」，行内括号保留。**验证**：`go build ./...` + `go test ./internal/...`（13 包全绿）+ `tsc --noEmit` + `npm run build`；新增 `TestTurnGateBehaviorMatrix`（**15 个真实场景**：普通陈述/提问/请求/被点名/长句倾诉/冷场必回，纯应答词静默，疑问语气必回）、`TestTurnGateSilenceOnBackchannel`、`TestTurnGateSilenceDisabled`，以及 `reply_text_test.go`（`postprocessReply` **此前完全无测试**，现覆盖 9 用例）；P0-2 反应停顿（`ThinkingPause` 250–900ms、`sync.Once` 仅首句、可被 ctx 取消）核实**已实现且有 AST 测试保护**，本轮无需改动。

- [x] 2026-09-11 详情模式体验修复（用户反馈）：① **取消关闭按钮**，改为点击抽屉外关闭（新增 `.drawer-backdrop` 遮罩，z-index 低于抽屉；顺带挡住对底层舞台的误触）；② **修复模型信息标题重复**（`ModelView` 自带标题，删掉外层重复标题行）；③ **后台任务重构为「列表 + 详情」两级**——原先把全部详情堆在列表卡片里导致文字截断/坍缩，现列表只给标题+目标+状态+变更数+时间，点击进详情分区展示（执行信息 meta 网格 / 代码变更与补丁 / 任务包 JSON / 结果），新增 `task-list*`/`task-detail*`/`task-meta-grid`/`task-status`/`task-spec-pre` 等样式；④ 导航收敛为 6 项（移除「对话」「外观/皮肤」），移除房间的「列表视图」按钮与 `onExitRoom`。**⚠️ 过程中的事故**：用 PowerShell 正则做多行 JSX 整段替换时把新内容插到了文件开头，随后又按错误行号裁剪，导致 `App.tsx` 一度被截断到 206 行；经备份定位（发现 `)}import …` 被拼接在同一行）恢复为 3454 行完整文件并通过 tsc。**教训**：整段 JSX 不做脚本化替换；脚本写文件前必备份，且先校验边界再落盘。验证：`tsc --noEmit` + `npm run build` 通过；产物核对 `task-list-item`/`task-detail-section`/`drawer-backdrop` 就位、`drawer-close` 在 JS 中归零；后端 12 包 + `go vet` + 插件测试全绿。

- [x] 2026-09-09 **修复详情模式右侧空白** + 取消列表视图（用户截图反馈）。**根因**：上轮抽屉迁移用 `{drawerView !== null && (<div className="drawer-panel">…)}` 包裹面板时，误把 `{activeView === 'chat' && …}` 与 `{activeView === 'skins' && …}` 两个分支**一起包进了该条件容器内**——于是 `drawerView === null` 时（点「对话」/「外观/皮肤」正是此情形）整个容器不渲染，两个核心页面直接消失。**教训**：JSX **嵌套位置**本身会改变渲染条件，`tsc` 对合法语法不报错，只有实际点击才会暴露；上轮"零功能回归风险"的判断因此是错的。**修复**：彻底移除 `drawer-panel`/`drawer-head`/`drawer-body` 三处包裹容器，所有面板回到 `web-content` 下的**平级分支**，抽屉定位与玻璃样式改由分支自身的 `.drawer-content` 类（纯 CSS `absolute`）承担，结构上杜绝"分支被条件容器吞掉"；关闭按钮改为内嵌各面板标题行（复用 `.ghost-button`）。**取消列表视图**：详情模式统一为「房间常驻 + 右侧抽屉」，导航只保留 房间/模型信息/插件管理/后台任务/桌宠日志/设置（移除独立的「对话」「外观/皮肤」——聊天在房间聊天岛、形象在舞台），移除房间里的「⇱ 列表视图」按钮，切换/新建会话后停留在房间。验证：`tsc --noEmit` + `npm run build` 通过；产物 `drawer-content`（JS 5 / CSS 4）、`drawer-close`（JS 5）、旧 `drawer-panel` 归零；后端 12 包与插件测试全绿。

- [x] 2026-09-09 详情模式：**状态岛**（`RoomView.tsx`、`utils.ts`、`App.css`）。在舞台左下角新增玻璃胶囊状态岛：情绪标签（按情绪取色，复用语义色令牌）+ 陪伴统计（陪伴天数 / 对话数 / 当前会话消息数）。**关键发现**：原计划认为它"需后端数据支撑"，实际核查后**现有数据已足够**——`conversations`/`messages` 表都有 `created_at`，无需新增接口。**一个易错点**：会话列表是 `ORDER BY updated_at DESC`，所以**不能取首项或末项**当作首次对话时间（续聊旧会话会让它在列表里靠前）——新增纯函数 `earliestTimestamp()` 真正求最早值，并忽略解析失败项、无合法值时返回 `undefined`（调用方据此隐藏，避免显示「第 NaN 天」）。验证：`tsc --noEmit` + `npm run build` 通过，产物核对含 `room-status-island`（JS 1 / CSS 2）与 `room-status-emotion`（CSS 6 条情绪取色规则）。

- [x] 2026-09-09 详情模式：**旧视图迁入 Room 抽屉浮层**。此前点插件/任务/日志/设置/模型会把整个页面切走、角色随之消失（违背"角色为王"）。现改为：Room 常驻为背景，这 5 个能力面板以**右侧玻璃抽屉**（`min(680px, 62%)`、半透明 + `backdrop-filter`、`drawer-slide-in` 动效）叠加展示，角色仍在背后可见。**实现要点**：① 新增 `drawerView` 状态，导航时 `activeView` 保持 `'room'` 并把目标视图写入 `drawerView`——因此**侧栏高亮与 `room-active` 判定都无需改动**；② 5 个面板分支由 `activeView ===` 改判 `drawerView ===`，并整体包进 `.drawer-panel` 容器（**不搬移各分支 JSX，零功能回归风险**）；③ 抽屉打开时 `.web-content.drawer-open .room-islands` 淡出并右移让位，避免与聊天岛视觉打架；④ **状态一致性**：抽屉关闭入口覆盖侧栏选「房间」、抽屉内「✕ 关闭」、ESC 键、以及 `newConversation`/`selectConversation`/`onExitRoom` 切回列表视图的全部路径，杜绝"页面已切换但浮层仍开着"的孤儿状态；⑤ `prefers-reduced-motion` 下关闭滑入动画。验证：`tsc --noEmit` + `npm run build` 通过，产物核对含 `drawer-panel`（JS 1 / CSS 2）、`drawer-slide-in`、`drawer-open` 与 `room-active`（5）等规则；后端 12 包回归与插件测试全绿。

- [x] 2026-09-09 详情模式 P2-25：**舞台点击互动**（`frontend/src/components/RoomView.tsx`、`App.css`）。点击房间中置舞台的角色区域即触发一次即时反应：从 6 条专属短台词中随机取一条 + 对应情绪（均在情绪白名单内，保证与 Live2D 表达式映射一致），同时播放 620ms「下压回弹」脉冲动效（`@keyframes room-stage-tap`，只动 `transform`）并显示台词气泡，3 秒后自动恢复。**关键实现细节**：① **事件只绑在 `.room-stage-live2d` 区域**而非整个舞台——否则顶部表情 chip / 窗口按钮 / 底部门廊按钮的点击会冒泡误触发；② **本地临时覆盖、不写后端情绪**：`stageEmotion = tapReaction ? reaction.emotion : emotion`，避免把互动反应污染成真实情绪状态；③ **外部情绪变化即让出控制权**（`useEffect(..., [emotion])` 清空反应），保证用户点表情 chip 或 LLM 情绪事件能立即覆盖；④ **脉冲生命周期交给 effect 自管**（`tapPulse` 置真后计时复位、cleanup 自动清理），避免手工维护多个定时器造成泄漏（初版即踩到「两个 timeout 共用一个 ref」的覆盖 bug，已改掉）；⑤ **气泡显示条件补 `isSpeaking` 守卫**——`assistantLine` 说完后仍会保留，仅以「有文本」为条件会导致气泡常驻（实现中发现并修正）；⑥ 支持键盘 Enter/Space 触发与 `:focus-visible` 焦点环（无障碍）。验证：`tsc --noEmit` + `npm run build` 通过，产物核对含 `is-tapped`（JS 1 处 / CSS 1 处）与 `room-stage-tap`（CSS 2 处）；`go test ./internal/...` 12 包 + 插件 9 项 + codex 4 项全绿。

- [x] 2026-09-09 拟人化：**情绪→台词对齐通道** + `send_service.go` 死代码清理。**收益项**：Planner 产出情绪（驱动前端表情）但 Replyer 写台词时**并不知道本轮该是什么情绪**——其提示词只给了取值范围与"让每句情绪匹配内容"，导致 LLM 逐句重猜、台词与情绪/表情脱节。新增纯函数 `formatEmotionDirective(decision)` 注入一行 `performance_directive`（emotion + mood，并把 valence/energy/dominance 转成**自然语言提示**如"偏积极/情绪激动/自信主导"），情绪为空时不注入。**清理项**：动手时发现 `SendService.SendGuidedReply` **已无任何调用点**（`sender` 仅被构造、从未调用），该文件早被流式 `streamReply` 取代——把仍被复用的纯函数（`postprocessReply`/`splitReply`/`isSentenceBoundary`/`trimSentencePart`/`nonEmpty`）迁至自包含的 `reply_text.go`，删除 `send_service.go` 与 `Service.sender` 接线。**教训**：改动前必须先确认目标代码是否可达（`grep` 调用点），否则会白改甚至留下半新半旧的死代码。验证：新增 `TestFormatEmotionDirective`（含"中性区间不误加提示"边界）；`go build ./...` + `go vet ./internal/...`（归零）+ `go test ./internal/...` 12 包全绿。

- [x] 2026-09-09 拟人化 P0-②：**反应停顿（thinking pause）** + 顺带修复 `go vet` copylocks。**反应停顿**：新增 `ChatConfig.ThinkingPauseMinMs/MaxMs`（json `thinking_pause_min_ms`/`max_ms`，默认 **250/900ms**，均 ≤0 即关闭、`max<min` 自动归一化）；`stream_reply.go` 新增纯函数 `ThinkingPause()`（零值安全 + 越界随机数折回）与可取消的 `waitThinkingPause()`；**停顿只作用于首个片段之前**且用 `sync.Once` 保证 dialog/flat-text 两条路径合计只停一次，后续句子仍保持流式。因 `Load()` 是「先 `DefaultConfig()` 再 `Unmarshal`」，**用户现有 config.json 无需改动即自动生效**，设为 0 回到旧行为。**防静默失效**：除 10 组边界单测外，新增 AST 级接线测试确认两条闭包路径都调用了停顿函数，并**做了变异验证**（临时删除一处调用 → 测试如期失败并精确指出路径）。**顺带**：`ApplyJSON` 原先 `temp := *c` 整体复制含 `sync.RWMutex` 的 Config（vet copylocks），改为零值逐字段装配 + 新增 `ApplyFrom`，并加反射测试 `TestApplyFromCoversAllFields` 防止将来新增字段被静默丢弃；`go vet ./internal/...` 已归零。验证：`go build ./...` + `go test ./internal/...`（12 包全绿）+ `go vet` 通过；`configs/config.example.json` 已同步字段。
- [x] 2026-09-09 拟人化差距分析文档：新增 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md)（约 2.2 万字符）。① **本项目事实基线**：逐项核查出 7 条"不像真人"的工程根因，全部附 `文件:行号`（必回机制 `turn_gate.go:45,75`、零时间节奏、情绪无状态、主动发言模板 `companion.go:584`、记忆只读不写、表达过度约束、快速通道情绪退化）；② **外部标杆研究**：Neuro-sama 复刻（人格特质向量 / 6 态情感状态机 / 认知帧 / 30% 探索率 / **打断记忆一致性** / 带优先级注入总线 / few-shot 台词种子）与 MaiMBot（**二维 valence·arousal + 指数不对称衰减** / MoFox 概率门控稀疏更新 / 情绪注入 Planner 而非 Replyer / 错别字模拟器 / 主动搭话三段式 / 关系值数学模型）；③ **横向对比表** + 学术证据（一致性即真实、不可预测性是最强吸引力）；④ **25 项可执行改进清单**（P0 改配置 → P1 加状态 → P2 架构级）+ **单用户场景明确"不要做"清单** + **来自标杆真实缺陷的反面教训**；⑤ 来源索引（含"GitCode 博客是 AI 生成稿、其结论非源码实现"的来源更正）。验证：文档内所有本项目结论均可按行号复核；外部结论已区分 `[外部事实]`/`[推断]`。
- [x] 2026-09-09 配色收敛 + 高级感 + 丝滑交互（`frontend/src/App.css`、`DESIGN.md`）：把配色从「糖果粉/紫/蓝」收敛为**雾玫瑰 + 石墨低饱和体系**（`--accent:#b06e93`、`--accent-2:#7d9ec0`、底色 `#f7f6f8`、文字 `#2b2732`），新增令牌 `--accent-strong/-soft-2/-line/-glow` 与语义色 soft/line 变体、统一中性阴影 `--shadow/-soft/-lift`、动效令牌 `--ease/--ease-out/--dur-fast/--dur/--dur-slow/--ring`。**根治"花"的来源**：删除全部 12 处 `repeating-linear-gradient` 装饰纹理、把 66+ 处硬编码 `rgba(255,94,168,*)`/`rgba(67,183,255,*)`/`rgba(201,120,173,*)` 与旧色值（`#ff7eba`/`#b91f6b`/`#c978ad` 等 43 处）全部替换为令牌引用，彩色光晕改为中性阴影。新增**统一交互层**：`transition` 统一到令牌时长/缓动，按钮悬停 `translateY(-1px)`、卡片 `-2px`、按下 `scale(.985)`、聚焦 `--ring` 光环，全部只动 transform/opacity/shadow/color（GPU 友好），并补 `prefers-reduced-motion` 兜底。同步 `DESIGN.md`（front-matter 色值/圆角/新增 motion 段 + 正文原则：无装饰纹理、阴影中性、组件禁止硬编码色值）。验证：`tsc --noEmit` + `npm run build` 通过；CSS 变量完整性校验 40 个引用全部有定义（`--pet-scale`/`--room-scale` 为运行时注入属预期）；残留纹理 0、残留高饱和 rgba 0。
- [x] 2026-09-09 netease-music 自动连播 / 下一首（v0.2.0）：sidecar 新增播放队列（`state.queue/queueIndex/queueSource`）+ `next`/`prev` 意图 + `/simi/song` 相似歌曲补歌（`fillQueueFromSimilar`，`filledFromSimilar` 去重 + `queueSize` 上限）；`playTrack` 抽出统一的「取直链→更新状态→返回 metadata」路径供 play/next/prev 共用；新增 `normalizeConfig`（`loadConfig` 与 `createController` 共用，修复「直接构造 controller 时 `cfg.autoNext` 为 undefined 导致连播恒关」的健壮性缺陷）；配置新增 `autoNext`(默认 true)/`queueSize`(默认 20，1-100)；`metadata` 扩展 `autoNext`/`queue{index,size,source}`。前端：`playMusicUrl` 在 `ended` 时按 `autoNext` 自动 `nextMusicTrack()`（`musicPlayIdRef` 世代守卫防重复触发），音乐岛新增 ⏭「下一首」按钮与队列位置显示（`2/5`），`musicTypes.ts` 补齐 `next/prev`/`autoNext`/`MusicQueueInfo` 类型；`plugin.json` 版本 0.1.0→0.2.0、intent 枚举与工具描述同步。测试：`main.test.js` 新增「自动连播」用例（队列保留完整搜索结果、队中 autoNext=true、队尾 false、相似歌曲续播、无相似时明确提示），9 项全绿。文档：README 增「自动连播」章节与契约字段、configSchema、AGENT/DEVELOPMENT-NOTES 同步。
- [x] 2026-09-09 详情模式（Room）UI 重构首轮：新增 `frontend/src/musicTypes.ts`（netease-music 结果类型/纯函数）、`frontend/src/components/MusicIsland.tsx`（可折叠音乐岛：自然语言点歌输入 + 正在播放卡 + 暂停/停止 + 搜索结果列表，结果经 `normalizeMusicResult` 统一规整）、`frontend/src/components/RoomView.tsx`（Room 主视图：中置 Live2D 舞台 + 表情 chips（`onPickEmotion` 联动模型）+ 角色说话气泡 + 右侧聊天岛（复用 App 内 composer/messages/feedRef，语音/V 键/发送全保留）+ 音乐岛 + 舞台内最小化/关闭按钮）。App.tsx 新增 `room` ViewKey（设为默认视图）+ `musicResult/musicPlaying/roomScale` 状态 + `runMusicCommand`（netease-music `control` 动作封装）+ `resizeRoomWithWheel`；web-shell 在 room 视图下收窄为全宽（侧栏改「房间门廊」悬浮浮层，`sidebarVisible` 切换）；**音乐播放与语音管线解耦**：新增 `musicAudioRef` + `playMusicUrl/pauseMusic/resumeMusic/stopMusic`，`handlePluginPlaybackResult` 改走独立音乐通道（不再触发唇形同步/barge-in/TTS 抢播），`stopCurrentAudio` 与音乐互斥联动；删旧 `playPluginAudioUrl`（语音管线路径死代码）。工程：无第三方 UI 库（延续「零额外框架」约定），样式追加在 `App.css` Room 段（玻璃/圆角/backdrop-filter 与 DESIGN.md token 一致）。验证：`tsc --noEmit` + `npm run build` + netease-music 8 项 + codex 4 项 Node 测试全绿。
- [x] 2026-09-09 netease-music 目录插件补全（原为 plugin.json/config.json 空壳）：新增 `main.js`（Node sidecar：NeteaseCloudMusicApi 客户端 + `createController` 播放状态机 + 自然语言意图解析 + stdin JSON-RPC 壳，`require.main===module` 守卫便于测试）、`main.test.js`（8 项回归全绿：意图/状态机/付费兜底/LRC/JSON-RPC 闭环/连接错误）、README。确立「sidecar 出指令、前端出声」契约：`metadata.playbackAction`(play/pause/resume/stop)+`playbackUrl` 由前端 `<audio>` 消费；`App.tsx` `invokePluginAction` 改走 `handlePluginPlaybackResult`，netease-music 详情页新增“音乐播放”输入框（自然语言→`control` action），通用动作网格排除 `control`。`tsc --noEmit` 通过。真实出声需本机启动 NeteaseCloudMusicApi（默认 3000 端口）。
- [x] 2026-09-09 code-assistant：修复嵌套文件事件与正文总结、UTF-8 分块解码、同文件重复写入通知；连接错误即时上报，超时保留最近错误。4 项 Node 回归测试通过；未改变任务完成与评审解耦语义。

### 基础与后端

- [x] Wails 项目骨架、`main.go`/`app.go` 绑定层、窗口参数（1024×768，可切桌宠透明窗口）。
- [x] 配置系统：多 Provider（OpenAI/DeepSeek/Moonshot/Ollama）+ 激活切换 + 本地 `config.json` 持久化。
- [x] 配置单测（`internal/config/config_test.go`：默认值/激活切换/更新 provider）。
- [x] Token 用量单测（`internal/usage/collector_test.go`：累计/TotalTokens 回退/nil 安全）。
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
- [x] 桌宠原生窗口透明（黑底修复）：`main.go` 配置 `Windows: &windows.Options{WebviewIsTransparent: true, WindowIsTranslucent: true, BackdropType: windows.None}`，消除「桌宠模式整窗除 Live2D 小人外全黑」（根因：原生 HWND 背景刷子被刷成黑色透出透明 WebView）。`go build ./...` 通过，需本地 `wails dev` 验证。
- [x] 无边框窗口（去系统边框/标题栏）：`main.go` 加 `Frameless: true` + `DisableFramelessWindowDecorations: true`，完整模式用前端自定义 header（拖拽区 + 最小化/关闭按钮），桌宠模式天然无边框无标题栏。`App.css` 补强 `.chat-panel .header-title`/`.status-bar` 拖拽区（去掉 `status-bar *` 的 no-drag）。`go build ./...` 通过，需本地 `wails dev` 验证。
- [x] 语音：Fish Audio TTS（buffered + 流式事件）、系统 TTS 兜底、浏览器 ASR、语音门控、打断（barge-in）、连续/自由对话、主动发言与追问。
- [x] UI 美化：重写 `App.css`（现代简洁配色 + 圆角卡片 + 聊天气泡）；头部精简为标题+状态+窗口按钮，功能按钮下沉为独立工具栏；`.chat-panel` 改 flexbox 布局彻底修复消息区滚动（header/toolbar/composer 固定、`message-feed` 独立滚动 + 细滚动条）；窗口默认尺寸提升到 1200×800。
- [x] 前端二次元浅色重构（frontend-design-premium）：新增 `DESIGN.md`/`UX-CONTRACT.md`/`premium-ui.json` 记录 Yuyu candy studio 设计与 UX ownership；`App.css` 从 web 工具页暗黑风改为浅色缤纷主题（莓粉/汽水蓝/薄荷绿 + 全局滚动条/focus/reduced-motion）；`App.tsx` 拆出 `appConfig.ts`、`appTypes.ts`、`utils.ts`、`components/AppShell.tsx`，先迁移聊天组合器、桌宠模式、侧边栏、聊天页、皮肤页、模型页等纯展示壳，保留语音/TTS 状态机在主组件内避免播放链路风险。验证：`tsc --noEmit`、`npm run build`、premium strict audit 通过，`designmd lint DESIGN.md` 0 errors（仅 orphan token warnings）。

### 情绪管线（M1）

- [x] 统一情绪 Schema（`internal/chat/emotion.go`：emotion/mood/gesture/hand 白名单 + energy 钳制）。
- [x] Planner 结构化产出情绪（`PlannerDecision` 增加 emotion/mood/energy/gesture/hand + 提示词 + 归一化）。
- [x] 情绪经 `ChatEvent(EventTypeEmotion)` → `collectingEmitter` → `ChatReply.Emotion` 端到端流转，`SendMessage` 优先用 LLM 情绪、回退 `inferEmotion`。
- [x] `ChatReply`/`CompanionMessage` 扩展 mood/energy/gesture/hand 字段，`SendMessage` 填充完整表演参数。
- [x] 前端 `App.tsx` 用 LLM 表演参数（mood/energy/hand）覆盖启发式 `inferAvatarPerformance`，并回退兜底（代码已写，需本地 `npm run build` 验证）。
- [x] 情绪持久化到 messages 表（`emotion/mood/energy/gesture/hand` 列 + 迁移 + `MessageRepo` 读写 + `SendGuidedReply` 回填 + `companionMessages` 读取回退启发式 + 单测）。

### 情绪系统 v2（连续 VAD + FACS/AU，参考 soullink-emotion-sdk）

- [x] 后端连续 VAD：`emotion.go` 新增 `ClampValence`/`ClampDominance`（valence -1..1、dominance -1..1，`energy`≡arousal 0..1）；`PlannerDecision`/`EmotionInfo`/`ChatEvent` 增加 `valence`/`dominance`；Planner 提示词要求输出连续情绪；`parsePlannerDecision` 归一化钳制。
- [x] VAD 端到端流转：`service.go` 事件 → `collectingEmitter` → `ChatReply`/`CompanionMessage`（新增 valence/dominance）→ `SendMessage` 回填（`inferValence`/`inferDominance` 启发式兜底）。
- [x] VAD 持久化：messages 表新增 `valence`/`dominance` 列（`ensureSchemaExtensions` 幂等）+ `MessageRepo` 读写 + 单测。
- [x] 前端 FACS/AU 合成引擎：新增 `frontend/src/components/emotionEngine.ts`——`computeAUWeights`（离散 emotion/mood + 连续 VAD → AU1/AU4/AU6/AU12/AU15… 权重）+ `computeExpressionTargets`（AU→逻辑参数净目标）+ `PARAM_REGISTRY` 参数注册表（逻辑参数→候选 Live2D 参数 ID，运行时 `getParameterIndex` 自动适配模型命名）。
- [x] `Live2DStage.tsx` 用 `computeExpressionTargets`/`applyExpressionTargets` 替换手调 `moodBoost`（微笑/眉/嘴型改由 AU 驱动，眨眼/唇同步/expression 层不受影响）；`AvatarPerformance` 增加 valence/dominance。
- [x] 前端 `App.tsx` 贯通 valence/dominance（`PerformanceHint`/`applyResponsePerformance`/`avatarPerformance` 合并 + `inferAvatarPerformance` 文本启发式兜底）；`models.ts` 同步补字段。
- [x] 验证：`go build ./...` + `go test ./internal/...`（10 包全绿）+ `tsc --noEmit`（exit 0）。前端渲染效果需本地 `npm run build` 验证。

### 语音识别（ASR）

- [x] 浏览器 ASR（Web Speech API）前端已接通：`App.tsx` `startBrowserVoiceInput`/`startBargeInListening` 用 `SpeechRecognition` 识别，配合语音门控/连续对话/打断。
- [x] 模型 ASR（Whisper 兼容）后端落地：新增 `internal/ai/asr` 包（`Transcribe` 调 OpenAI 兼容 `/audio/transcriptions` multipart 上传 + `parseTranscriptionResponse`/`extFromContentType` 纯函数 + `asr_test.go`）；`config` 新增 `ASR.Model`；`companion.go` `TranscribeAudio` 从占位改为真实转录（复用激活 Provider 的 BaseURL/APIKey + `asr.model`）。
- [x] 前端模型 ASR 已接线（`startModelASRVoiceInput` 录音 → base64 → `TranscribeAudio`），无需改动；`VITE_ASR_PROVIDER` 默认 `browser`，设 `model` 走模型识别。
- [x] 验证：`go build ./...` + `go test ./internal/...`（12 包全绿）。网络路径需用户本地验证。

### 回复延迟优化（流式 Replyer + 逐句 TTS，参考 Shinsekai）

- [x] 后端流式回复：`ReplyerAgent` 抽出 `buildMessages` + 新增 `Stream`（Eino `model.Stream`）；新增 `stream_reply.go`（`streamingSentencer` 增量按标点/超长切句 + `Service.streamReply` 边生成边 `EventTypeToken` emit + 持久化）；`service.go` 回复段改用 `streamReply`。
- [x] 前端流式通道 + 逐句 TTS：`App.tsx` `sendContent` 从 `SendMessage`（收集全文）切到 `StreamChat` + `EventsOn("chat:event")`；收到一句 `EventTypeToken` 就 `speakText` 合成+播放一句（`streamSentenceQueueRef`/`sentencePlayingRef`/`streamReplyActiveRef`/`streamDoneRef` 状态机，与 LLM 生成重叠）；`EventTypeEmotion` 即时驱动情绪；`done` 收尾（刷新消息 + scheduleFollowUp）；`error` 中止。`App.StreamChat`（app.go）补 `ensureCompanionReady`。
- [x] Planner 快速通道（降 LLM 首字延迟）：`shouldSkipPlanner` 保守判断「简单闲聊」直接流式回复、跳过 Planner 一整轮（`fast_path.go` + 单测）；`InferEmotionFromText` 快速通道兜底情绪；情绪事件改到流式文本之前发出（早于逐句朗读驱动表情）；Planner/Replyer 加 `model.WithMaxTokens` 约束输出长度。`App.tsx` 无 LLM mood 时清空 `performanceHint` 回退文本启发式。
- [x] 逐句预合成（消除句间空档）：`playSpeechReply` 从 `speakWithBufferedCloudVoice` 抽出「播放已合成音频」；`startPrefetch` 后台 peek 队首并预 `SynthesizeSpeech`（`prefetchedSpeechRef`/`prefetchedTextRef`/`prefetchInFlightRef`）；`finishSpeaking` drain 优先播预合成句、否则现场合成，并清理过期预取。
- [x] 修复「发消息不回复」：TurnGate 基础分 0.38→0.60（直接消息必回复）、删除 bot_streak_penalty、新增 looksLikeWeakBackchannel 只过滤极短应答词。
- [x] 验证：`go build ./...` + `go test ./internal/...`（12 包全绿）+ `tsc --noEmit`（exit 0）。渲染/延迟效果需本地 `wails dev` 验证。

### 插件系统（M2 内核）

- [x] 插件接口与契约（`internal/plugin/plugin.go`：`Plugin`/`Manifest`/`Action`/`Host`）。
- [x] 插件管理器（`internal/plugin/manager.go`：注册/启用/停用/列表/动作派发/停用全部）。
- [x] 内置示例插件（`internal/plugin/builtin_system.go`：`system` 插件，`ping`/`version` 动作）。
- [x] 内置工作区插件（`internal/plugin/workspace.go`：`workspace` 插件，`list`/`read`/`write` 动作，复用工作区路径隔离；`workspace_test.go` 验证读写列往返 + 越界拒绝）。
- [x] 插件工具同时进入 Planner 与 Worker 工具集（`app.go` 用 `workerToolReg` 注册表承载 Worker 工具，执行器改为 `toolProvider` 动态读取，插件 `RegisterTool` 双写）。
- [x] 宿主接线（`app.go`）：插件工具注册进工具注册表、挂载内置插件（system + workspace）、Shutdown 时 StopAll。
- [x] Wails 方法：`ListPlugins`（真实数据）/`EnablePlugin`/`DisablePlugin`/`InvokePluginAction`（`internal/app/plugin_service.go`）。
- [x] 单元测试（`internal/plugin/manager_test.go`：生命周期 + 动作派发 + 禁用/启用 + 校验；`workspace_test.go`）。
- [x] 前端插件面板（`App.tsx` 插件列表/启停/调用动作 + JSON 参数输入 + 配置查看/保存 + `App.css` 样式；`tsc --noEmit` 通过）。
- [x] 插件配置持久化（`plugin.ConfigStore` 接口 + `Host.Config` + `Manager.GetConfig/SetConfig` + 宿主 settings 键值表 + Wails `GetPluginConfig`/`SetPluginConfig` + 前端配置按钮；`config_test.go`）。
- [x] wailsjs 绑定补全（`App.js`/`App.d.ts` 新增 `EnablePlugin`/`DisablePlugin`/`InvokePluginAction`/`GetPluginConfig`/`SetPluginConfig`，`models.ts` 新增情绪表演字段）。
- [x] 子进程 sidecar（阶段 2）：`internal/plugin/sidecar.go` 通过 stdio JSON-RPC 驱动外部插件进程（`SidecarSpec`/`SidecarPlugin`/`sidecarClient`），`Manager.Register` 在 Init 后重新读取协商的 manifest；`sidecar_test.go` 用 re-exec 模式验证全链路。第三方插件无需重编译宿主即可挂载。
- [x] code-assistant VS Code 评审体验增强：`run_agent` 结束后保留相对基线的标准 git 工作区改动，并把 `cwd/baseCommit/branch/files/diff` 原样写入后台任务 `result_json.metadata.code_review`；后台任务页直接显示代码变更清单、VS Code 查看、补丁、接受/拒绝。修复目录/嵌套 git 仓库（如生成的 React 子项目）被当作普通文件 diff 导致 VS Code “打开但没内容”的问题；本轮新建的嵌套 `.git` 会临时移到系统 temp，让父仓库按普通目录显示内部文件级增删，已有嵌套仓库不动。插件管理页同步美化为插件目录 + 详情工作台，只保留插件说明、模型工具、手动动作与配置。

### 电脑工具（M3 地基）

- [x] 工作区安全边界（`internal/ai/tools/workspace.go`：路径 containment + 符号链接逃逸拦截）。
- [x] 文件系统工具（`internal/ai/tools/filesystem.go`：`list_files`/`read_file`/`write_file`）。
- [x] 只读工具接入 Planner（`list_files`/`read_file` 已注册；`write_file` 仅 Worker 使用，未注册进同步工具集）。
- [x] 配置新增 `App.WorkspaceRoot`（默认回退用户主目录）。
- [x] 单元测试（`internal/ai/tools/filesystem_test.go`：越界/软链逃逸拒绝 + 读写列往返）。
- [x] Worker 真实执行器（`internal/agent/llm_executor.go`：LLM 工具循环 + `allowed_actions` 白名单过滤 + 事件/操作日志；`internal/app/worker_executor.go` 模型适配；`app.go` 接入 Worker 工具集含 `write_file`；`llm_executor_test.go` 4 个单测通过）。
- [x] Worker 审批流（`Runtime.RequestApproval` + `waiting_for_approval` 状态机 + approve/reject/cancel 消费；`llm_executor.go` 对 `write_file` 等危险工具先审批；`approval_test.go` 验证提交→挂起→批准→完成全链路）。
- [x] 命令执行工具（`internal/ai/tools/command.go`：工作区目录内执行 + 超时 + 输出截断；`execute_command` 加入 Worker 工具集与 `approvalRequiredTools`；`command_test.go` 实测 exec 通过）。
- [x] 键鼠输入合成（`internal/ai/tools/input.go` + `input_windows.go`（SendInput）+ `input_other.go`（no-op）；`send_input` 加入 Worker 工具集与 `approvalRequiredTools`；`input_test.go` 验证按键名→VK 映射）。
- [x] 屏幕截图（`internal/ai/tools/screen.go` + `screen_windows.go`（BitBlt/GetDIBits→PNG）+ `screen_other.go`（no-op）；`screen_capture` 加入 Worker 工具集与 `approvalRequiredTools`）。
- [x] 「看屏幕」接线（`ObserveScreen` 截屏保存到工作区 screenshots/ 并返回路径/描述）。
- [x] 多模态视觉描述（`internal/ai/vision/vision.go`：直连 OpenAI 兼容多模态 API；`ObserveScreen` 在配置 `Vision.Model` 后描述画面；`vision_test.go` 覆盖请求构造/响应解析）。
- [ ] 剪贴板工具（Windows 特定，命令工具经 PowerShell 可部分替代）。

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
- [x] `docs/GPT-SOVITS-GUIDE.md`（GPT-SoVITS 音色复刻训练 + 接入指南）。
- [x] `docs/SENSEVOICE-ASR-GUIDE.md`（SenseVoice/FunASR 本地语音识别接入指南）。
- [x] 配置管理：`config.json` 位于**项目 `configs/` 目录**（`configDir()` 优先读 `<cwd>/configs/config.json`，回退 `%APPDATA%\Yuyu-Mind\config.json`）；`configs/config.example.json` 为模板；新增 `services` 段（gpt_sovits_root/conda_exe/sensevoice_env/model）供启停脚本读取；**`start-all.bat` / `stop-all.bat` 为双击入口**（内部调用对应 .ps1，带 `-ExecutionPolicy Bypass`），`.ps1` 为实现（JSON 解析 + 服务窗口 + 构建 + wails dev；`-SkipBuild` 可跳构建）；.ps1 需 UTF-8 BOM、.bat 需无 BOM。

### 语音合成（GPT-SoVITS 本地音色复刻）

- [x] 后端 GPT-SoVITS provider：新增 `internal/app/gpt_sovits.go`（`synthesizeGptSovitsSpeech` POST JSON → `/tts` 可配 `endpoint`；`parseGptSovitsResponse` 兼容 api_v2 的 `data[0].audio` base64 与 api.py 原始 WAV 字节；`gpt_sovits_test.go` 覆盖两种响应 + 空响应）。
- [x] 配置：`config.go` `Speech.Provider`（`fish_audio`/`gpt_sovits`）+ `Speech.GPTSoVITS`（base_url/endpoint/refer_audio_path/prompt_text/prompt_lang/text_lang）；`SynthesizeSpeech` 按 provider 路由。
- [x] 验证：`go build ./...` + `go test ./internal/app ./internal/config` 通过。音色复刻训练需用户本机跑 GPT-SoVITS（见 `docs/GPT-SOVITS-GUIDE.md`）。

## Next（待办 / 路线图）

> ✅ **前端类型已验证**：`tsc --noEmit` 通过（exit 0），前端 TSX/绑定/models 改动类型正确。剩余只需本地 `npm install`（完整）+ `npm run build` 生成 `dist`（沙箱内 esbuild `spawn EPERM` 无法打包）+ `wails dev` 启动。Go 后端已全程 `go build`/`go test` 通过（10 包全绿）。

核心愿景能力（情绪/对话/电脑工具/任务/插件生态/屏幕观察）已全部实现并提交推送。剩余均为环境依赖或可选：

- [ ] 生成 `frontend/dist`，打通全量 `go build .` / `make build`（用户本地）。
- [ ] 剪贴板工具（可选，`execute_command` 经 PowerShell 可替代）。
- [ ] JS 脚本插件（可选阶段 3，goja）。
- [ ] 拆分 `App.tsx`（前端 3400+ 行，可维护性优化）。
- [x] 配色收敛 + 高级感 + 丝滑交互：雾玫瑰/石墨低饱和体系、清除全部装饰纹理与硬编码色值、中性阴影分层、统一 `--dur/--ease` 交互令牌与 reduced-motion 兜底（已落地，见 Done；`DESIGN.md` 已同步）。
- [x] **拟人化 P0（已落地，见 Done）**：打破必回（`AllowSilenceOnBackchannel` + 弱回撤静默，但疑问语气必回）+ 反应停顿（`ThinkingPause` 250–900ms，仅首句、可取消）+ 放松表达约束（prompt 允许语气词/冗余/自我更正；清洗保留口语插入语）。方案见 [`docs/REALISM-ANALYSIS.md`](docs/REALISM-ANALYSIS.md)。
- [x] **拟人化 P1-7 打断记忆一致性（已落地，见 Done）**：打断时只保留真正播出的句子、丢弃未播出的，并插入 `[被用户打断]` 标记——让角色只记得自己说出口的话。
- [ ] **拟人化 P1/P2（待排期）**：情绪模糊修饰语注入 Planner + 情绪数值动力学（指数不对称衰减）+ 概率门控稀疏更新 + 口语冗余/口误自纠正；P2 记忆自动回灌与关系印象、主动搭话三段式、prompt 文件化。**注意**：单用户场景不要照搬群聊机制（willing/Focus/发言频率），否则会变成"该回话时不理你"。
- [ ] 详情模式后续批次：旧视图（插件/任务/日志/设置/皮肤）迁入 Room 抽屉式浮层；状态岛（好感度/陪伴统计，需后端数据）。**舞台点击互动已完成**（见 Done）。
- [x] 情绪系统 v2（参考 [soullink-emotion-sdk](https://github.com/nanlingyin/soullink-emotion-sdk)）：连续 VAD（valence/dominance，energy≡arousal）+ FACS/AU 表驱动表情合成 + Live2D 参数注册表自动适配（已落地，见 Done）。
- [x] 模型 ASR（Whisper 兼容）+ 回复流式（后端逐句 emit + 前端逐句 TTS）+ GPT-SoVITS 本地 TTS provider（已落地，见 Done）。
- [ ] 音色复刻训练（用户本机，GPT-SoVITS）：按 `docs/GPT-SOVITS-GUIDE.md` 训练并启动 API 后，在 `config.json` 设 `speech.provider=gpt_sovits` + 参考音频路径。
- [ ] 情绪系统 v2.1（可选）：语音 VAD 实时推断（音频连续情绪，而非仅文本/LLM）；直接接入 `@soullink-emotion/live2d-pixi` SDK 替换自研合成层。
- [ ] 本地端到端验证（`wails dev`）：Planner 稳定性、Live2D 情绪、任务执行、审批流、无边框拖拽、逐句 TTS、GPT-SoVITS 音色。

- [x] Settings workspace root editor: added a dedicated settings control for app.workspace_root. Saving validates and persists the path, refreshes file tools, updates background task defaults, refreshes workspace plugin, and reloads directory plugins so later sidecars receive YUYU_WORKSPACE.
