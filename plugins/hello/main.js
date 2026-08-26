'use strict';
// 目录插件 sidecar 入口。宿主按需拉起本进程，通过 stdio 上的 newline-delimited JSON-RPC 通信。
// 协议见 README.md：invoke_action / invoke_tool。

const readline = require('readline');

// 宿主注入的配置（JSON 字符串），解析失败则回退到空对象。
let config = {};
try {
  config = JSON.parse(process.env.YUYU_PLUGIN_CONFIG || '{}');
} catch (_) { /* ignore */ }

const rl = readline.createInterface({ input: process.stdin, output: process.stdout });

function reply(msg) {
  process.stdout.write(JSON.stringify(msg) + '\n');
}

function decodeArguments(raw) {
  if (!raw) return {};
  try { return JSON.parse(raw); } catch (_) { return {}; }
}

rl.on('line', (line) => {
  let req;
  try { req = JSON.parse(line); } catch (_) { return; }

  const id = req.id;
  const method = req.method;
  const params = req.params || {};

  try {
    switch (method) {
      case 'invoke_action': {
        const action = params.action;
        const input = params.input || {};
        if (action === 'hello') {
          reply({ id, result: { message: '你好呀，我是「' + (config.name || 'hello') + '」插件！' } });
        } else {
          reply({ id, error: { message: 'unknown action ' + action } });
        }
        break;
      }
      case 'invoke_tool': {
        const toolName = params.tool;
        const args = decodeArguments(params.arguments);
        if (toolName === 'shout') {
          const text = String(args.text || '');
          reply({ id, result: { result: text.toUpperCase() } });
        } else {
          reply({ id, error: { message: 'unknown tool ' + toolName } });
        }
        break;
      }
      default:
        reply({ id, error: { message: 'unknown method ' + method } });
    }
  } catch (err) {
    reply({ id, error: { message: String(err && err.message || err) } });
  }
});
