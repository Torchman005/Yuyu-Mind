# 插件开发指南

> 本文档说明如何为 Yuyu Mind 开发插件。插件系统定位：让「可被大模型或用户调用的能力」以插件形式挂载，而不是堆在主流程里。
>
> **当前采用的架构：目录即插件（副进程 sidecar 的即插即用）。** 一个插件 = 一个目录，目录内放元数据文件（`plugin.json`/`.yaml`/`.yml`/`.toml`）与配置文件（`config.<fmt>`）。宿主启动/重载时扫描插件总目录即可识别，**无需改任何 Go 代码**。能力由目录里的入口脚本/可执行文件承担，通过 stdio 上的 JSON-RPC 对外提供。

## 一、插件是什么

一个插件 = **目录 + 元数据（Manifest）+ 能力（工具/动作）+ 入口脚本**。

- **工具（Tool）**：注册进宿主的工具注册表，供 **LLM**（Planner 或 Worker）自主调用。危险工具受白名单 + 工作区 + 审批约束。
- **动作（Action）**：暴露给 **前端插件面板**，由用户手动触发（点击按钮 + 输入 JSON 参数）。

> 说明：内置插件（`system` / `workspace`）仍为进程内 Go 实现（编译进二进制、可信代码）；第三方插件推荐用**目录 + sidecar** 形态，做到即插即用、崩溃隔离。

## 二、目录结构

```
plugins/                          ← 插件总目录（config.app.plugins_root，默认 "plugins"，相对可执行文件目录）
└── {插件名}/                     ← 一个插件 = 一个目录
    ├── plugin.json               ← 元数据(manifest)，也可用 plugin.yaml/.yml/.toml
    ├── config.json               ← 运行配置，也可用 config.yaml/.yml/.toml（缺失时按默认值运行）
    ├── main.js                   ← 入口 sidecar；按 runtime 字段或扩展名启动，stdio JSON-RPC
    └── README.md                 ← 可选
```

宿主在**启动时**扫描 `plugins_root` 下的每个子目录；遇到含元数据文件的目录就注册。插件页点击 **「重新加载」** 会重新扫描：新增目录即时注册、删掉的目录即时卸载（`ReloadPlugins`）。

## 三、元数据文件（manifest）

用 `plugin.json`（或 `.yaml` / `.yml` / `.toml`，按扩展名自动识别）。字段：

```jsonc
{
  "schemaVersion": "1.0",        // 固定 "1.0"
  "name": "hello",               // 唯一 ID（小写字母+数字），如 "hello"
  "displayName": "你好插件",      // 显示名
  "description": "一句话说明",
  "version": "0.1.0",            // 语义化版本
  "author": "Yuyu Mind",
  "entry": "main.js",            // 入口文件（相对插件目录）；缺省则无法被拉起
  "runtime": "node",             // 可选：node | python | 空（按 entry 扩展名推断）
  "permissions": ["system.read"],// 声明所需能力（供前端展示）
  "configSchema": {              // 可选：配置 JSON Schema
    "type": "object",
    "properties": { "name": { "type": "string" } }
  },
  "actions": [                   // 动作：供前端按钮触发
    { "name": "hello", "description": "返回问候语", "inputSchema": {} }
  ],
  "tools": [                     // 工具：注册给 LLM 自主调用
    {
      "name": "shout",
      "description": "把文本转成大写",
      "inputSchema": { "type": "object", "properties": { "text": { "type": "string" } }, "required": ["text"] }
    }
  ]
}
```

- `runtime` 与 `entry` 扩展名的对应：`.js` → `node`，`.py` → `python`，其它扩展名视为可直接执行的文件（如 `.exe` / `.cmd` / `.bat`）。
- 工具会被注册进 **Planner（聊天）与 Worker（任务）** 两个工具集；模型可直接调用。

## 四、入口 sidecar 与协议

宿主按需拉起入口进程（首次调用 `invoke_action`/`invoke_tool` 时才启动，进程缓存复用）。通过 stdio 上的 **newline-delimited JSON-RPC** 通信。

**宿主注入的环境变量：**
- `YUYU_PLUGIN_DIR`：插件目录绝对路径（可用作工作目录、定位配置文件）。
- `YUYU_PLUGIN_CONFIG`：当前配置的 JSON 字符串（sidecar 无需自带 YAML/TOML 解析器即可读取）。

**请求/响应（stdin 读一行 → stdout 写一行）：**
```json
{ "id": 1, "method": "invoke_action", "params": { "action": "hello", "input": {} } }
{ "id": 1, "result": { "message": "你好" } }

{ "id": 2, "method": "invoke_tool", "params": { "tool": "shout", "arguments": "{\"text\":\"hi\"}" } }
{ "id": 2, "result": { "result": "HI" } }

{ "id": 3, "method": "invoke_tool", "params": { "tool": "x" } }
{ "id": 3, "error": { "message": "unknown tool x" } }
```

约定：
- `invoke_action`：命令在 `params.action`，参数在 `params.input`；`result` 为任意 JSON 对象。
- `invoke_tool`：命令在 `params.tool`，参数为 `params.arguments`（JSON 参数字符串）；`result.result` 为该工具返回的字符串。
- 错误统一返回 `{ "id": n, "error": { "message": "..." } }`。

**最小 Node 入口示例：**
```js
'use strict';
const readline = require('readline');
const config = (() => { try { return JSON.parse(process.env.YUYU_PLUGIN_CONFIG || '{}'); } catch (_) { return {}; } })();
const rl = readline.createInterface({ input: process.stdin, output: process.stdout });
const reply = (m) => process.stdout.write(JSON.stringify(m) + '\n');
const argsOf = (raw) => { try { return JSON.parse(raw || '{}'); } catch (_) { return {}; } };

rl.on('line', (line) => {
  let req; try { req = JSON.parse(line); } catch (_) { return; }
  const { id, method } = req; const params = req.params || {};
  try {
    if (method === 'invoke_action') {
      if (params.action === 'hello') reply({ id, result: { message: '你好，我是「' + (config.name || 'hello') + '」插件！' } });
      else reply({ id, error: { message: 'unknown action ' + params.action } });
    } else if (method === 'invoke_tool') {
      const args = argsOf(params.arguments);
      if (params.tool === 'shout') reply({ id, result: { result: String(args.text || '').toUpperCase() } });
      else reply({ id, error: { message: 'unknown tool ' + params.tool } });
    } else reply({ id, error: { message: 'unknown method ' + method } });
  } catch (err) { reply({ id, error: { message: String(err?.message || err) } }); }
});
```

## 五、配置文件

配置存放在插件目录内：`config.json`（或 `.yaml`/`.yml`/`.toml`）。宿主在「配置」面板读写：
- 读取：`GetPluginConfig(name)` → 存在则返回文件内容；不存在返回空对象。
- 保存：`SetPluginConfig(name, config)` → 写回同一格式（无配置文件时默认写 `config.json`；TOML 会剔除 `null` 值）。
- sidecar 每次拉起时通过 `YUYU_PLUGIN_CONFIG` 拿到最新配置。

## 六、完整示例

参考 `plugins/hello/`（元数据 + 配置 + 入口 + README 四件套）。

```bash
plugins/hello/
├── plugin.json     # 见「三」
├── config.json     # {"name": "小宇"}
├── main.js         # 见「四」
└── README.md
```

## 七、编写「让桌宠使用 IDE / 编码 agent」的插件

仓库已内置参考实现 `plugins/code-assistant/`（用 **Codex CLI** 写代码 + 一键开 IDE）。在 `plugins/` 下新建目录，入口脚本里直接 `spawn` 外部 agent/IDE 即可，无需改宿主：

```
plugins/code-assistant/
├── plugin.json     # entry: "main.js", runtime: "node"
├── config.json     # {"agent_command": "codex", "sandbox": "workspace-write", "full_auto": true}
└── main.js
```

`main.js` 对 `invoke_tool` 分发时调用 agent（注意：宿主会把桌宠工作区注入 `YUYU_WORKSPACE`，可用于默认落位）：

```js
const { execFile } = require('child_process');
const cfg = JSON.parse(process.env.YUYU_PLUGIN_CONFIG || '{}');
const cmd = cfg.agent_command || 'codex';
const workspace = process.env.YUYU_WORKSPACE || process.env.YUYU_PLUGIN_DIR;

function runAgent(task, cb) {
  const args = ['exec'];
  // --approve-for-me 隐含 workspace-write 沙箱，不能与 --sandbox 同时传（codex 0.149+ 互斥）。
  if (cfg.dangerous) args.push('--dangerously-bypass-approvals-and-sandbox');
  else if (cfg.full_auto !== false) args.push('--approve-for-me');
  else if (cfg.sandbox) args.push('--sandbox', cfg.sandbox);
  args.push('--skip-git-repo-check', '-C', cfg.cwd || workspace, task);
  execFile(cmd, args, { cwd: cfg.cwd || workspace, timeout: cfg.timeout_ms || 900000, maxBuffer: 8 * 1024 * 1024 }, (e, out, err) => {
    cb(out || String(err || e?.message || ''));
  });
}
```

同时在 `actions` 里加 `open_in_ide`（`spawn('code', [path], { detached: true })`）作为一键打开文件/工作区的动作。注册后，模型通过自然语言即可调用 `run_agent` 工具在项目里写代码，插件页也能一键打开 IDE。

## 八、内部实现（参考）

- `internal/plugin/fileconfig.go`：manifest / config 的 json/yaml/toml 解析 + `FileConfigStore` + `CompositeConfigStore`。
- `internal/plugin/dirplugin.go`：`DirPlugin`（目录→sidecar，懒拉起）+ `DiscoverPluginDirs`（扫描）+ `dirTool`（转发到 sidecar 的工具桩）。
- `internal/plugin/sidecar.go`：stdio JSON-RPC 客户端（`SidecarSpec` / `sidecarClient`）。
- `internal/app/plugin_dir.go`：`resolvePluginsRoot` / `loadDirPlugins` / `ReloadPlugins`。
- `internal/app/app.go`：`Startup` 里建 `NewManager`（插件工具同时进 Planner/Worker 工具集）+ 注册内置插件 + `loadDirPlugins`。

## 九、约定与注意

1. **Name 唯一**：插件名（目录）是唯一 ID，重复会覆盖。
2. **动作用户触发，工具 LLM 触发**：动作不经过审批流（用户显式点击）；工具若危险，请在 `internal/agent/llm_executor.go` 的 `approvalRequiredTools` 里加上工具名，执行前走审批。
3. **工作区约束**：若工具要读写文件/执行命令，建议复用 `internal/ai/tools.Workspace`（`Resolve` 保证不越界）并默认落在工作区根目录。
4. **副作用可逆**：sidecar 进程由宿主 `Stop` 时终止；插件自身应避免长驻后台任务。
5. **配置/类型同步**：动作返回 `map[string]any`，Wails 会 JSON 序列化；字段用 camelCase。
6. **工具桩卸载**：从插件页移除插件目录后，其工具桩仍残留在工具注册表（工具注册表不支持按插件移除）；若重新注册同名插件会覆盖旧桩，可接受。

## 十、路线图

- **阶段 1（已实现）**：进程内内置插件（`system` / `workspace`），`Plugin` 接口 + `Manager`。
- **阶段 2（已实现，当前主推）**：目录即插件 + sidecar。元数据/配置走文件（json/yaml/toml），自动扫描 `plugins_root` 注册，`ReloadPlugins` 热加载；副作用在独立进程，崩溃隔离。
- **阶段 3（可选，待办）**：JS 引擎（goja）脚本插件、动作级权限强制、工具桩按插件摘除。
