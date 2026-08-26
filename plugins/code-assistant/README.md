# 代码助手插件（code-assistant）

通过 **Codex CLI** 在项目里写代码，改动发生在 **git feature 分支**上，生成 diff 供评审：可**接受新代码**或**拒绝（保留旧代码）**；并支持一键在 IDE 打开文件/工作区。目录即插件，丢进 `plugins/` 点「重新加载」即可。

## 目录结构（模块化）

```
plugins/code-assistant/
├── plugin.json     # 元数据：entry=src/index.js，声明 actions/tools
├── config.json     # 运行配置
├── package.json    # 模块信息（main=src/index.js）
├── README.md
└── src/
    ├── index.js    # 入口：JSON-RPC 分发（invoke_action / invoke_tool）
    ├── codex.js    # 运行 Codex，流式输出 + 发射进度事件
    └── git.js      # git 评审：feature 分支、diff、接受/拒绝
```

> 复杂插件就这样拆分：入口只做分发，逻辑放到 `src/` 下的多个模块，通过 Node `require` 相互引用，不必全写进一个 `main` 文件。

## 能力

| 能力 | 类型 | 说明 |
|---|---|---|
| `run_agent` | 工具（LLM 触发） | 在 feature 分支上运行 codex 写代码，返回 diff + 进度步骤 |
| `show_diff` | 动作 | 查看待评审的 diff |
| `list_changes` | 动作 | 返回待评审文件列表（新增/修改/删除/重命名） |
| `open_changes` | 动作 | 在 VS Code 打开源代码管理视图，并为变更文件打开 diff 标签 |
| `accept_changes` | 动作 | 接受新代码（合并进基线分支） |
| `reject_changes` | 动作 | 拒绝新代码（丢弃 feature 分支，保留旧代码） |
| `open_workspace` | 动作 | 在 IDE 打开当前评审工作区 |

## 评审式改动工作流

1. 聊天里让 Yuyu「帮我改代码」→ Planner 发起后台任务 → Worker 调用 `run_agent`。
2. 插件把项目初始化成 git 仓库（如还不是），切到 `yuyu/agent-*` 分支，跑 codex。
3. 跑完把结果重置成「相对基线的未提交/已暂存改动」，用 `git diff --cached HEAD` 生成 diff，返回给桌宠。
4. 你在**桌宠日志/任务页**看进度（插件会中途发射 `agent_progress` 事件，宿主写入日志并发 `plugin:progress` Wails 事件）。
5. 在 IDE（VS Code 等）里用 git 源管理器看每一步的增删；或点插件页的 `list_changes` / `show_diff` 看清单与 diff。
6. 决定：点 `accept_changes`（合并新代码）或 `reject_changes`（保留旧代码）。

## 配置项（`config.json`）

| 键 | 默认 | 含义 |
|---|---|---|
| `agent_command` | `codex` | Codex 可执行名 |
| `model` | `""` | `--model`；留空用 codex 默认（**ChatGPT 账号请留空或选受支持模型**） |
| `full_auto` | `true` | `--approve-for-me` 自动批准（隐含 workspace-write 沙箱） |
| `sandbox` | `workspace-write` | 仅 `full_auto=false` 且 `dangerous=false` 时生效 |
| `dangerous` | `false` | `--dangerously-bypass-approvals-and-sandbox`（极危险） |
| `timeout_ms` | `600000` | 单次运行超时（毫秒） |
| `cwd` | `""` | 运行目录（= `-C` 工作区根与沙箱作用域）。留空=桌宠工作区；建议设为项目目录 |
| `ide_command` | `code` | IDE 命令 |
| `extra_args` | `[]` | 追加到 `codex exec` 的参数 |

> Windows 上 codex 是 npm 的 `.cmd` shim，插件会自动定位 `node .../codex.js` 直接用 node 跑（绕开 shell 引号问题）。

## 在 VS Code 中查看改动

codex 的改动会保留为「相对基线的未提交/已暂存改动」，所以只要把项目打开在 **VS Code** 里就能直接看到增删：

1. 点插件页的 **open_workspace**（或 `open_in_ide`）→ 在当前评审目录打开 VS Code。
2. 在 VS Code 打开 **源代码管理**（`Ctrl+Shift+G`）——能看到改动的文件。
3. 点某个文件→「Diff」视图显示 `+` 新增 / `-` 删除。
4. 想更接近常用 agent 插件的直观评审体验，在后台任务里点 **在 VS Code 查看**：插件会用 VS Code CLI 为前几个变更文件打开左右对比 diff 标签。可传 `{"limit": 20}` 打开更多，或传 `{"files":["src/App.tsx"]}` 只看指定文件。若本轮新建的 React/Vite 子项目自带 `.git`，插件会临时把这个新 `.git` 移到系统临时目录，让父仓库按普通目录显示内部文件增删；已有嵌套仓库不会被处理。
5. 决定：**接受**→在桌宠评审面板点「接受新代码」；**拒绝**→点「保留旧代码」。
   （也可在 VS Code 里用 git 操作：暂存+提交=接受，放弃更改=拒绝。）

> 目录不是 git 仓库时会在 `run_agent` 时自动 `git init`；改动都在 `yuyu/agent-*` 分支上，开始评审前会用基线分支做 `git diff --cached` 生成 diff。

实现要点：不依赖 VS Code 私有扩展 API，而是复用标准 git 状态。`list_changes` 读取 `git diff --name-status -M -z HEAD`；`open_changes` 把 `HEAD:<file>` 写到临时基线文件，再执行 `code --diff <base> <worktree>`，因此新增、修改、删除、重命名都能以普通 VS Code diff 标签打开。目录型变更不会再伪装成文件 diff，避免 VS Code 打开后空白。

## 手动测试

```bash
node plugins/code-assistant/src/index.js
# 输入：{"id":1,"method":"invoke_tool","params":{"tool":"run_agent","arguments":"{\"task\":\"列出一个文件\"}"}}
```
