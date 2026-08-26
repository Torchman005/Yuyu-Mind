'use strict';
// git.js —— 管理「评审式改动」的 git 状态：基线分支、feature 分支、diff、接受/拒绝。
// 用法：run_agent 在 feature 分支上让 codex 改代码，之后用户可 show_diff / accept / reject。

const { execFileSync } = require('child_process');

function runSync(cwd, args) {
  try {
    return { ok: true, stdout: execFileSync('git', args, { cwd, encoding: 'utf8', windowsHide: true }).trim() };
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
