# 你好插件（hello）—— 目录插件的即插即用示例

这是一个最小的 Node.js sidecar 插件，演示「插件 = 一个目录」：
把一个目录丢进插件总目录（`plugins_root`，默认 `plugins/`）即可被识别加载，**无需改动任何 Go 代码**。

## 文件结构

```
plugins/hello/
├── plugin.json   元数据（manifest）：名称、入口、动作、工具、配置结构
├── config.json   运行配置（宿主从这里读写；缺失时插件按默认值运行）
├── main.js       入口：由宿主按需拉起，通过 stdio 上的 JSON-RPC 对外提供能力
└── README.md
```

## 协议（宿主 ↔ sidecar 的 JSON-RPC）

宿主把 `config` 以 JSON 注入环境变量 `YUYU_PLUGIN_CONFIG`，把插件目录注入 `YUYU_PLUGIN_DIR`。
sidecar 从 stdin 读取按行分隔的 JSON 请求，向 stdout 写按行分隔的 JSON 响应：

```json
{ "id": 1, "method": "invoke_action", "params": { "action": "hello", "input": {} } }
{ "id": 1, "result": { "message": "..." } }

{ "id": 2, "method": "invoke_tool", "params": { "tool": "shout", "arguments": "{\"text\":\"hi\"}" } }
{ "id": 2, "result": { "result": "HI" } }
```

约定：
- `invoke_action` 命令进入 `params.action`，参数进入 `params.input`；`result` 为任意 JSON 对象。
- `invoke_tool` 命令进入 `params.tool`，参数为 `params.arguments`（JSON 参数字符串）；`result.result` 为工具返回的字符串。
- 错误统一返回 `{ "id": n, "error": { "message": "..." } }`。

## 配置项

| 键 | 含义 |
|---|---|
| `name` | 插件自我介绍的称呼（默认 `hello`） |

## 手动测试

插件页点「重新加载」即可识别该目录；点动作卡片上的 `hello` 会得到问候；
聊天里让 Yuyu 调用 `shout` 工具（如果工具已暴露给模型）会得到大写文本。
也可以在终端直接跑 sidecar 手动验证：

```
npm install   # 本示例无第三方依赖，可跳过
YUYU_PLUGIN_CONFIG='{"name":"小宇"}' node plugins/hello/main.js
```

然后输入上面任意 JSON 请求即可看到响应。
