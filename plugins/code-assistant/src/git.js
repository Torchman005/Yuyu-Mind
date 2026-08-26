'use strict';
// git.js —— 管理「评审式改动」的 git 状态：基线分支、feature 分支、diff、接受/拒绝。
// 用法：run_agent 在 feature 分支上让 codex 改代码，之后用户可 show_diff / accept / reject。

const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

// 解析 git 可执行文件完整路径：GUI 桌宠进程的 PATH 常不含 git，直接 spawn 'git' 会 ENOENT。
// 依次：where git → 遍历 PATH 找 git.exe → 常见 Git 安装路径。拿到真实 git.exe 再调用。
function detectGitBin() {
  try {
    const which = process.platform === 'win32' ? 'where' : 'which';
    const out = execFileSync(which, ['git'], { encoding: 'utf8' });
    const lines = out.split(/\r?\n/).filter((l) => l.trim());
    const exe = lines.find((l) => /\.exe$/i.test(l.trim()));
    if (exe || lines[0]) return (exe || lines[0]).trim();
  } catch (_) { /* fall through */ }
  const seen = new Set();
  for (const dir of (process.env.PATH || '').split(path.delimiter)) {
    if (!dir || seen.has(dir)) continue;
    seen.add(dir);
    for (const name of ['git.exe', 'git']) {
      try { const cand = path.join(dir, name); if (fs.existsSync(cand)) return cand; } catch (_) { /* ignore */ }
    }
  }
  const roots = [process.env.ProgramFiles, process.env['ProgramFiles(x86)'], process.env.LOCALAPPDATA, 'D:\\Git'];
  for (const r of roots) {
    if (!r) continue;
    for (const sub of ['Git\\cmd\\git.exe', 'Git\\bin\\git.exe', 'Programs\\Git\\cmd\\git.exe', 'cmd\\git.exe', 'bin\\git.exe']) {
      try { const cand = path.join(r, sub); if (fs.existsSync(cand)) return cand; } catch (_) { /* ignore */ }
    }
  }
  return 'git';
}
const GIT = detectGitBin();

function ensureDir(cwd) {
  try { if (!fs.existsSync(cwd)) fs.mkdirSync(cwd, { recursive: true }); } catch (_) { /* ignore */ }
}

function runSync(cwd, args) {
  try {
    return { ok: true, stdout: execFileSync(GIT, args, { cwd, encoding: 'utf8', windowsHide: true }).trim() };
  } catch (e) {
    return { ok: false, stdout: (e.stdout || '').trim(), stderr: (e.stderr || e.message || '').trim() };
  }
}

// 当前待评审状态：{ cwd, baseBranch, branch }
let review = null;

function currentReview() { return review; }

function ensureRepo(cwd) {
  if (runSync(cwd, ['rev-parse', '--is-inside-work-tree']).ok) return;
  const init = runSync(cwd, ['init']);
  if (!init.ok) throw new Error('git init 失败：' + init.stderr);
}

// 确保至少有一个提交（否则无法基于基线建分支）。
function ensureCommit(cwd) {
  if (runSync(cwd, ['rev-parse', 'HEAD']).ok) return;
  runSync(cwd, ['add', '-A']);
  const c = runSync(cwd, ['commit', '--allow-empty', '-m', 'baseline (agent init)']);
  // 允许失败（例如没有可提交内容），仍继续。
  void c;
}

function beginReview(cwd) {
  ensureDir(cwd);
  ensureRepo(cwd);
  ensureCommit(cwd);
  const baseBranch = runSync(cwd, ['branch', '--show-current']).stdout || 'master';
  const branch = 'yuyu/agent-' + Date.now();
  const co = runSync(cwd, ['checkout', '-b', branch]);
  if (!co.ok) throw new Error('git checkout -b 失败：' + co.stderr);
  review = { cwd, baseBranch, branch };
  return review;
}

function commitAll(cwd, msg) {
  runSync(cwd, ['add', '-A']);
  const c = runSync(cwd, ['commit', '--allow-empty', '-m', msg]);
  return c.ok;
}

function diff(cwd, baseBranch, branch) {
  const d = runSync(cwd, ['diff', baseBranch + '...' + branch]);
  return d.ok ? d.stdout : (d.stderr || '');
}

function acceptChanges() {
  if (!review) throw new Error('暂无待评审的改动');
  const { cwd, baseBranch, branch } = review;
  const co = runSync(cwd, ['checkout', baseBranch]);
  if (!co.ok) throw new Error('checkout 基线失败：' + co.stderr);
  const m = runSync(cwd, ['merge', '--no-ff', branch, '-m', 'accept agent changes']);
  runSync(cwd, ['branch', '-d', branch]); // 已合并则删除
  review = null;
  return { ok: m.ok, baseBranch, merged: m.ok, output: m.stdout || m.stderr };
}

function rejectChanges() {
  if (!review) throw new Error('暂无待评审的改动');
  const { cwd, baseBranch, branch } = review;
  runSync(cwd, ['checkout', baseBranch]);
  runSync(cwd, ['branch', '-D', branch]);
  review = null;
  return { ok: true, baseBranch, rejected: true };
}

module.exports = { currentReview, beginReview, commitAll, diff, acceptChanges, rejectChanges };
