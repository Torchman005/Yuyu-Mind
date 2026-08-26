'use strict';
// 代码助手插件入口（模块化：见 src/codex.js、src/git.js）。
// 协议见 docs/PLUGIN-GUIDE.md。宿主注入 YUYU_PLUGIN_CONFIG / YUYU_WORKSPACE / YUYU_PLUGIN_DIR。

const readline = require('readline');
const path = require('path');
const { spawn } = require('child_process');
const codex = require('./codex');
const git = require('./git');

const config = (() => { try { return JSON.parse(process.env.YUYU_PLUGIN_CONFIG || '{}'); } catch (_) { return {}; } })();
const pluginDir = process.env.YUYU_PLUGIN_DIR || process.cwd();
const workspace = process.env.YUYU_WORKSPACE || pluginDir;

const rl = readline.createInterface({ input: process.stdin, output: process.stdout });
const reply = (m) => process.stdout.write(JSON.stringify(m) + '\n');
const safeParse = (raw) => { try { return JSON.parse(raw || '{}'); } catch (_) { return {}; } };
const truncate = (s, n) => (s && s.length > n) ? s.slice(0, n) + '\n...[截断]' : (s || '');

function targetPath(p) {
  return path.isAbsolute(p) ? p : path.resolve(config.cwd || workspace, p);
}

function spawnIde(args) {
  const ide = String(config.ide_command || 'code');
  try {
    const child = spawn(ide, args, { cwd: workspace, detached: true, stdio: 'ignore', shell: process.platform === 'win32' });
    if (child.unref) child.unref();
  } catch (_) { /* ignore */ }
}

async function handleTool(name, args) {
  if (name !== 'run_agent') throw new Error('unknown tool ' + name);
  const task = args.task || args.prompt || '';
  if (!task) throw new Error('task is required');
  const cwd = String(args.cwd || config.cwd || workspace);

  // 在 feature 分支上跑 codex，改完生成 diff 供评审。
  const review = git.beginReview(cwd);
  const res = await codex.runCodex(task, cwd, config);
  if (res.err) {
    return JSON.stringify({
      ok: false,
      cwd,
      exitCode: res.err.code === 'ETIMEOUT' ? null : 1,
      note: res.err.code === 'ETIMEOUT' ? 'codex 运行超时' : String(res.err.message || res.err),
      output: truncate((res.stdout || '') + '\n' + (res.stderr || ''), 8000),
      steps: res.steps,
    });
  }
  git.commitAll(cwd, 'agent: ' + task.slice(0, 80));
  const diff = git.diff(cwd, review.baseBranch, review.branch);
  return JSON.stringify({
    ok: true,
    cwd,
    baseBranch: review.baseBranch,
    branch: review.branch,
    summary: truncate((res.stdout || '').trim(), 2000),
    steps: res.steps,
    diff: truncate(diff, 20000),
  });
}

async function handleAction(name, input) {
  if (name === 'open_in_ide') {
    const p = input.path || input.file || '';
    if (!p) return { ok: false, error: 'path(input.path) is required' };
    const t = targetPath(p);
    spawnIde([t]);
    return { ok: true, path: t, command: config.ide_command || 'code' };
  }
  if (name === 'open_workspace') {
    const p = input.dir || input.path || config.cwd || workspace;
    const t = targetPath(p);
    spawnIde(['--new-window', t]);
    return { ok: true, path: t };
  }
  if (name === 'show_diff') {
    const rev = git.currentReview();
    if (!rev) return { ok: false, error: '暂无待评审的改动' };
    return { ok: true, baseBranch: rev.baseBranch, branch: rev.branch, diff: truncate(await git.diff(rev.cwd, rev.baseBranch, rev.branch), 20000) };
  }
  if (name === 'accept_changes') return git.acceptChanges();
  if (name === 'reject_changes') return git.rejectChanges();
  throw new Error('unknown action ' + name);
}

rl.on('line', (line) => {
  let req; try { req = JSON.parse(line); } catch (_) { return; }
  const id = req.id, method = req.method, params = req.params || {};
  const run = async () => {
    try {
      if (method === 'invoke_tool') {
        const args = safeParse(params.arguments);
        const result = await handleTool(params.tool, args);
        reply({ id, result: { result } });
      } else if (method === 'invoke_action') {
        const result = await handleAction(params.action, params.input || {});
        reply({ id, result });
      } else {
        reply({ id, error: { message: 'unknown method ' + method } });
      }
    } catch (e) {
      reply({ id, error: { message: String(e && e.message || e) } });
    }
  };
  run();
});
