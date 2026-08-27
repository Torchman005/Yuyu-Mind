'use strict';
// git.js —— 管理「评审式改动」的 git 状态：基线、live 工作区、diff、接受/拒绝。
// 用法：run_agent 默认让 codex 直接写当前工作区，之后用户可在 VS Code 逐块评审。

const { execFileSync } = require('child_process');
const fs = require('fs');
const os = require('os');
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

function runBufferSync(cwd, args) {
  try {
    return { ok: true, stdout: execFileSync(GIT, args, { cwd, windowsHide: true }) };
  } catch (e) {
    return { ok: false, stdout: e.stdout || Buffer.alloc(0), stderr: (e.stderr || e.message || '').toString().trim() };
  }
}

// 当前待评审状态：{ cwd, baseBranch, baseCommit, branch, mode }
let review = null;

const scanSkipDirs = new Set(['.git', 'node_modules', 'dist', 'build', '.gocache', '.gotmp', '.gomodcache', '.next', '.vite']);

function normalizePath(p) {
  return path.resolve(p).toLowerCase();
}

function scanNestedGitRoots(root, maxDirs = 12000) {
  const result = [];
  const rootAbs = path.resolve(root);
  let seen = 0;
  function walk(dir) {
    if (++seen > maxDirs) return;
    let entries = [];
    try { entries = fs.readdirSync(dir, { withFileTypes: true }); } catch (_) { return; }
    const hasGit = entries.some((entry) => entry.name === '.git');
    if (hasGit && normalizePath(dir) !== normalizePath(rootAbs)) {
      result.push(path.resolve(dir));
      return;
    }
    for (const entry of entries) {
      if (!entry.isDirectory() || scanSkipDirs.has(entry.name)) continue;
      walk(path.join(dir, entry.name));
    }
  }
  walk(rootAbs);
  return result;
}

function absorbNewNestedGitRepos(cwd) {
  if (!review) return [];
  const baseline = new Set((review.baselineNestedRepos || []).map(normalizePath));
  const nested = scanNestedGitRoots(cwd);
  return absorbNestedGitRepos(cwd, nested.filter((repoRoot) => !baseline.has(normalizePath(repoRoot))));
}

function relForGit(cwd, absPath) {
  return path.relative(cwd, absPath).replace(/\\/g, '/');
}

function isAbsentFromHead(cwd, absPath) {
  const rel = relForGit(cwd, absPath);
  if (!rel || rel.startsWith('..')) return false;
  const out = runSync(cwd, ['ls-tree', 'HEAD', '--', rel]);
  return out.ok && !out.stdout.trim();
}

function absorbReviewNestedGitRepos(cwd) {
  const nested = scanNestedGitRoots(cwd).filter((repoRoot) => isAbsentFromHead(cwd, repoRoot));
  const absorbed = absorbNestedGitRepos(cwd, nested);
  if (absorbed.length > 0 && review && review.mode === 'branch') runSync(cwd, ['add', '-A']);
  return absorbed;
}

function absorbNestedGitRepos(cwd, nested) {
  const absorbed = [];
  for (const repoRoot of nested) {
    const gitDir = path.join(repoRoot, '.git');
    try {
      if (!fs.statSync(gitDir).isDirectory()) continue;
      const backupRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'yuyu-nested-git-'));
      const target = path.join(backupRoot, path.basename(repoRoot) + '.git');
      fs.renameSync(gitDir, target);
      absorbed.push({ repoRoot, gitBackup: target });
    } catch (_) { /* keep the nested repo if it cannot be safely moved */ }
  }
  if (review) review.absorbedNestedRepos = absorbed;
  return absorbed;
}

function currentReview() { return review; }

function restoreReview(input) {
  if (!input || typeof input !== 'object') return review;
  const cwd = String(input.cwd || '').trim();
  if (!cwd) return review;
  review = {
    cwd,
    baseBranch: String(input.baseBranch || input.base_branch || 'master'),
    baseCommit: String(input.baseCommit || input.base_commit || ''),
    branch: String(input.branch || ''),
    mode: String(input.mode || input.reviewMode || input.review_mode || 'live'),
  };
  return review;
}

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

function hasWorkingChanges(cwd) {
  return workTreeStatusFiles(cwd).length > 0;
}

function beginReview(cwd, options = {}) {
  ensureDir(cwd);
  ensureRepo(cwd);
  ensureCommit(cwd);
  const mode = String(options.mode || 'live');
  const allowDirty = Boolean(options.allowDirty);
  if (!allowDirty && hasWorkingChanges(cwd)) {
    throw new Error('目标项目已有未提交改动。为避免把你的手动改动和 agent 改动混在一起，请先提交、暂存或清理后再运行；确实要混合评审时可在插件配置里启用 allow_dirty_review。');
  }
  const baseBranch = runSync(cwd, ['branch', '--show-current']).stdout || 'master';
  const baseCommit = runSync(cwd, ['rev-parse', 'HEAD']).stdout || '';
  const baselineNestedRepos = scanNestedGitRoots(cwd);
  const branch = mode === 'branch' ? 'yuyu/agent-' + Date.now() : baseBranch;
  if (mode === 'branch') {
    const co = runSync(cwd, ['checkout', '-b', branch]);
    if (!co.ok) throw new Error('git checkout -b 失败：' + co.stderr);
  }
  review = { cwd, baseBranch, baseCommit, branch, mode, baselineNestedRepos };
  return review;
}

// 评审结束前整理嵌套仓库。live 模式保留未暂存改动，VS Code 可对单个 hunk 执行还原；
// branch 模式保持旧行为：reset --soft 到基线并 stage 全量改动，供整轮合并。
function prepareReviewChanges(cwd) {
  if (!review) return;
  const absorbed = absorbNewNestedGitRepos(cwd);
  if (review.mode === 'branch') {
    if (review.baseCommit) runSync(cwd, ['reset', '--soft', review.baseCommit]);
    runSync(cwd, ['add', '-A']);
  }
  return absorbed;
}

function statusLabel(code) {
  const c = String(code || '').charAt(0);
  if (c === 'A') return 'added';
  if (c === 'D') return 'deleted';
  if (c === 'R') return 'renamed';
  if (c === 'C') return 'copied';
  if (c === 'M') return 'modified';
  if (c === 'T') return 'typechanged';
  if (c === 'U') return 'unmerged';
  if (c === '?') return 'added';
  return 'changed';
}

function changedFiles(cwd) {
  const out = runBufferSync(cwd, ['diff', '--name-status', '-M', '-z', 'HEAD']);
  const byPath = new Map();
  if (!out.ok || !out.stdout.length) {
    for (const file of workTreeStatusFiles(cwd).filter((f) => String(f.status || '').includes('?'))) {
      byPath.set(file.path, file);
    }
    return Array.from(byPath.values());
  }
  const parts = out.stdout.toString('utf8').split('\0').filter(Boolean);
  const files = [];
  for (let i = 0; i < parts.length;) {
    const status = parts[i++];
    if (!status) continue;
    if (/^[RC]/.test(status)) {
      const oldPath = parts[i++] || '';
      const filePath = parts[i++] || oldPath;
      files.push({ status, kind: statusLabel(status), path: filePath, oldPath });
    } else {
      const filePath = parts[i++] || '';
      files.push({ status, kind: statusLabel(status), path: filePath });
    }
  }
  for (const file of files) byPath.set(file.path, file);
  for (const file of workTreeStatusFiles(cwd).filter((f) => String(f.status || '').includes('?'))) {
    if (!byPath.has(file.path)) byPath.set(file.path, file);
  }
  return Array.from(byPath.values());
}

function isDirectory(cwd, filePath) {
  try { return fs.statSync(path.resolve(cwd, filePath)).isDirectory(); } catch (_) { return false; }
}

function isGitRepo(cwd) {
  return runSync(cwd, ['rev-parse', '--is-inside-work-tree']).ok;
}

function workTreeStatusFiles(cwd) {
  const out = runBufferSync(cwd, ['status', '--short', '--untracked-files=all', '-z']);
  if (!out.ok || !out.stdout.length) return [];
  const parts = out.stdout.toString('utf8').split('\0').filter(Boolean);
  const files = [];
  for (let i = 0; i < parts.length; i += 1) {
    const row = parts[i];
    const status = row.slice(0, 2).trim() || 'M';
    const rawPath = row.slice(3);
    if (!rawPath) continue;
    const arrow = rawPath.indexOf(' -> ');
    if (arrow >= 0) {
      files.push({ status, kind: statusLabel(status), oldPath: rawPath.slice(0, arrow), path: rawPath.slice(arrow + 4) });
    } else {
      files.push({ status, kind: statusLabel(status), path: rawPath });
    }
  }
  return files;
}

function enrichFile(cwd, file) {
  const abs = path.resolve(cwd, file.path);
  const directory = isDirectory(cwd, file.path);
  const nestedRepo = directory && isGitRepo(abs);
  const nestedFiles = nestedRepo ? workTreeStatusFiles(abs).slice(0, 200) : [];
  return {
    ...file,
    absolutePath: abs,
    directory,
    nestedRepo,
    nestedCount: nestedFiles.length,
    nestedFiles,
  };
}

function reviewFiles(cwd) {
  return changedFiles(cwd).map((file) => enrichFile(cwd, file));
}

function sanitizeTempName(filePath) {
  return String(filePath || 'file').replace(/^[a-zA-Z]:/, '').replace(/[\\/:*?"<>|]+/g, '__');
}

function writeTempFile(dir, filePath, content) {
  const tempPath = path.join(dir, sanitizeTempName(filePath));
  fs.mkdirSync(path.dirname(tempPath), { recursive: true });
  fs.writeFileSync(tempPath, content);
  return tempPath;
}

function baseBlob(cwd, file) {
  return runBufferSync(cwd, ['show', 'HEAD:' + file]);
}

function diffPairs(cwd, selectedPaths) {
  const selected = new Set((selectedPaths || []).map((p) => {
    if (p && typeof p === 'object') return String(p.path || '').replace(/\\/g, '/');
    return String(p || '').replace(/\\/g, '/');
  }).filter(Boolean));
  const all = reviewFiles(cwd);
  const files = selected.size ? all.filter((f) => selected.has(String(f.path).replace(/\\/g, '/'))) : all;
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'yuyu-code-diff-'));
  return files.flatMap((f) => {
    if (f.nestedRepo) {
      const nestedPairs = diffPairs(path.resolve(cwd, f.path), []);
      if (nestedPairs.length) {
        return nestedPairs.map((pair) => ({
          ...pair,
          path: path.join(f.path, pair.path).replace(/\\/g, '/'),
          oldPath: pair.oldPath ? path.join(f.path, pair.oldPath).replace(/\\/g, '/') : pair.oldPath,
          repoPath: path.resolve(cwd, f.path),
          nestedRepo: true,
        }));
      }
      return [{ ...f, openOnly: true, repoPath: path.resolve(cwd, f.path), left: '', right: path.resolve(cwd, f.path), workPath: path.resolve(cwd, f.path) }];
    }
    if (f.directory) {
      return [{ ...f, openOnly: true, repoPath: cwd, left: '', right: path.resolve(cwd, f.path), workPath: path.resolve(cwd, f.path) }];
    }
    const oldFile = f.oldPath || f.path;
    const workPath = path.resolve(cwd, f.path);
    let left = '';
    let right = workPath;
    if (f.kind === 'added') {
      left = writeTempFile(tempDir, f.path + '.base', Buffer.alloc(0));
    } else {
      const blob = baseBlob(cwd, oldFile);
      left = writeTempFile(tempDir, oldFile + '.base', blob.ok ? blob.stdout : Buffer.alloc(0));
    }
    if (f.kind === 'deleted') {
      right = writeTempFile(tempDir, f.path + '.deleted', Buffer.alloc(0));
    }
    return { ...f, left, right, workPath };
  });
}

function diff(cwd, baseBranch, branch) {
  const d = runSync(cwd, ['diff', 'HEAD']);
  if (d.ok && d.stdout) return d.stdout;
  const staged = runSync(cwd, ['diff', '--cached', 'HEAD']);
  if (staged.ok && staged.stdout) return staged.stdout;
  const d2 = runSync(cwd, ['diff', baseBranch + '...' + branch]);
  return d2.ok ? d2.stdout : (d2.stderr || '');
}

function acceptChanges() {
  if (!review) throw new Error('暂无待评审的改动');
  const { cwd, baseBranch, branch, mode } = review;
  if (mode !== 'branch') {
    const files = reviewFiles(cwd);
    review = null;
    return {
      ok: true,
      baseBranch,
      accepted: true,
      mode,
      files: files.length,
      output: '已接受当前工作区中的 agent 改动；改动保持为未提交状态，可继续由你在 VS Code 中提交。',
    };
  }
  const commit = runSync(cwd, ['commit', '--allow-empty', '-m', 'agent changes（评审通过）']);
  const co = runSync(cwd, ['checkout', baseBranch]);
  const m = runSync(cwd, ['merge', '--no-ff', branch, '-m', 'accept agent changes']);
  runSync(cwd, ['branch', '-d', branch]); // 已合并则删除
  review = null;
  return { ok: commit.ok && co.ok && m.ok, baseBranch, merged: m.ok, output: m.stdout || m.stderr || commit.stderr };
}

function rejectChanges() {
  if (!review) throw new Error('暂无待评审的改动');
  const { cwd, baseBranch, branch, baseCommit, mode } = review;
  if (baseCommit) runSync(cwd, ['reset', '--hard', baseCommit]); // 丢弃改动
  runSync(cwd, ['clean', '-fd']);
  if (mode === 'branch') {
    runSync(cwd, ['checkout', baseBranch]);
    runSync(cwd, ['branch', '-D', branch]);
  }
  review = null;
  return { ok: true, baseBranch, rejected: true, mode };
}

module.exports = { currentReview, restoreReview, beginReview, prepareReviewChanges, absorbReviewNestedGitRepos, changedFiles, reviewFiles, diffPairs, diff, acceptChanges, rejectChanges };
