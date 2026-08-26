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
| `accept_changes` | 动作 | 接受新代码（合并进基线分支） |
| `reject_changes` | 动作 | 拒绝新代码（丢弃 feature 分支，保留旧代码） |
| `open_in_ide` / `open_workspace` | 动作 | 在 IDE 打开文件/工作区 |

## 评审式改动工作流

1. 聊天里让 Yuyu「帮我改代码」→ Planner 发起后台任务 → Worker 调用 `run_agent`。
2. 插件把项目初始化成 git 仓库（如还不是），切到 `yuyu/agent-*` 分支，跑 codex。
3. 跑完自动 `git commit`，用 `git diff 基线...分支` 生成 diff，返回给桌宠。
4. 你在**桌宠日志/任务页**看进度（插件会中途发射 `agent_progress` 事件，宿主写入日志并发 `plugin:progress` Wails 事件）。
5. 在 IDE（VS Code 等）里用 git 源管理器看每一步的增删；或点插件页的 `show_diff` 看 diff。
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

## 手动测试

```bash
node plugins/code-assistant/src/index.js
# 输入：{"id":1,"method":"invoke_tool","params":{"tool":"run_agent","arguments":"{\"task\":\"列出一个文件\"}"}}
```
