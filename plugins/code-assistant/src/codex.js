'use strict';
// codex.js —— 运行 Codex CLI，流式输出并发射进度事件（供宿主中途播报）。

const { spawn, execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

function emit(event, data) {
  // 事件行：{"event": "...", "data": {...}}。宿主 sidecar 读循环会把它路由给进度回调。
  process.stdout.write(JSON.stringify({ event, data }) + '\n');
}

function detectCodexNodeScript() {
  if (process.platform !== 'win32') return null;
  try {
    const which = process.platform === 'win32' ? 'where' : 'which';
    const out = execFileSync(which, ['codex'], { encoding: 'utf8' });
    const first = out.split(/\r?\n/).filter(Boolean)[0];
    if (first) {
      const base = path.dirname(first);
      const cand = path.join(base, 'node_modules', '@openai', 'codex', 'bin', 'codex.js');
      if (fs.existsSync(cand)) return cand;
    }
  } catch (_) { /* ignore */ }
  return null;
}

function buildArgs(task, cwd, config) {
  const args = ['exec'];
  if (config.dangerous) args.push('--dangerously-bypass-approvals-and-sandbox');
  else if (config.full_auto !== false) args.push('--approve-for-me');
  else if (config.sandbox) args.push('--sandbox', String(config.sandbox));
  if (config.model) args.push('--model', String(config.model));
  args.push('--skip-git-repo-check', '-C', cwd);
  (config.extra_args || []).forEach((a) => args.push(String(a)));
  args.push(task);
  return args;
}

// 判断某行是否像「有意义的一步」，用于中途播报。
const stepPattern = /(reading|planning|writing|creating|running|executing|testing|installing|updating|deleting|modifying|generating|searching|exploring)/i;

function runCodex(task, cwd, config) {
  return new Promise((resolve) => {
    const cmd = String(config.agent_command || 'codex');
    const args = buildArgs(task, cwd, config);
    const script = detectCodexNodeScript();
    const useNode = script && cmd === 'codex';
    const spawnCmd = useNode ? 'node' : cmd;
    const spawnArgs = useNode ? [script].concat(args) : args;
    const shell = !useNode && process.platform === 'win32';

    emit('agent_progress', { message: '开始执行编码 agent：' + task.slice(0, 160) });

    const child = spawn(spawnCmd, spawnArgs, { cwd, shell, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
    let out = '', errOut = '';
    const steps = [];
    child.stdout.on('data', (d) => {
      const text = d.toString();
      out += text;
      for (const line of text.split(/\r?\n/)) {
        const s = line.trim();
        if (s && stepPattern.test(s) && s.length < 300) {
          steps.push(s);
          emit('agent_progress', { message: s.slice(0, 200) });
        }
      }
    });
    child.stderr.on('data', (d) => { errOut += d; });

    const timer = setTimeout(() => {
      try { child.kill(); } catch (_) { /* ignore */ }
      resolve({ err: Object.assign(new Error('timeout'), { code: 'ETIMEOUT' }), stdout: out, stderr: errOut, steps });
    }, config.timeout_ms || 600000);

    child.on('error', (e) => { clearTimeout(timer); resolve({ err: e, stdout: out, stderr: errOut, steps }); });
    child.on('close', (code) => {
      clearTimeout(timer);
      const err = code === 0 ? null : Object.assign(new Error('exit ' + code), { code });
      resolve({ err, stdout: out, stderr: errOut, steps });
    });
  });
}

module.exports = { runCodex };
