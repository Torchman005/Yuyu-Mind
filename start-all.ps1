<#
  start-all.ps1 —— 一键启动 Yuyu Mind 桌宠及依赖服务

  自动读取 config.json（优先项目 configs\config.json，其次 %APPDATA%\Yuyu-Mind\config.json），
  按配置判断是否启动：
    - GPT-SoVITS（当 speech.provider == "gpt_sovits"，路径取 services.gpt_sovits_root）
    - SenseVoice（当 asr.model 已配置且 asr.base_url 为本地地址，conda 参数取 services.*）

  无需手动改本脚本：项目根、服务路径、conda 环境都从 config.json 读取。

  用法：
    .\start-all.ps1             # 构建前端 + 启动服务 + wails dev
    .\start-all.ps1 -SkipBuild  # 跳过前端构建
#>
param(
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'

# 项目根 = 本脚本所在目录（无需手动改）
$ProjectRoot = $PSScriptRoot

# ---- 定位 config.json（项目 configs/ 优先，其次 AppData） ----
$config = $null
$configPath = Join-Path $ProjectRoot 'configs\config.json'
if (-not (Test-Path $configPath)) {
    $configDir = if ($env:YUYU_CONFIG_DIR) { $env:YUYU_CONFIG_DIR } else { Join-Path $env:APPDATA 'Yuyu-Mind' }
    $configPath = Join-Path $configDir 'config.json'
}
if (Test-Path $configPath) {
    $config = Get-Content $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
    Write-Host "[i] 使用配置：$configPath" -ForegroundColor Cyan
} else {
    Write-Host "[!] 未找到 config.json（$configPath），将按默认配置运行" -ForegroundColor Yellow
}

# ---- 从配置读取本地服务路径（services.*），留空则用默认 ----
$gptSovitsRoot   = [string]$config.services.gpt_sovits_root
$condaExe        = [string]$config.services.conda_exe
$senseVoiceEnv   = [string]$config.services.sensevoice_env
$senseVoiceModel = [string]$config.services.sensevoice_model
if (-not $gptSovitsRoot)  { $gptSovitsRoot  = 'D:\itJinYu_toolkit\GPT-SoVITS-v2pro-20250604\GPT-SoVITS-v2pro-20250604' }
if (-not $condaExe)       { $condaExe       = 'conda' }
if (-not $senseVoiceEnv)  { $senseVoiceEnv  = 'funasr' }
if (-not $senseVoiceModel){ $senseVoiceModel = 'iic/SenseVoiceSmall' }

# ---- 在新窗口启动一个服务（cmd /k 保持窗口） ----
function Start-InNewCmd([string]$Title, [string]$WorkDir, [string]$Cmd) {
    Write-Host "[+] 启动 $Title ..." -ForegroundColor Cyan
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = 'cmd.exe'
    $psi.Arguments = "/k title $Title & cd /d `"$WorkDir`" & $Cmd"
    $psi.UseShellExecute = $true
    [System.Diagnostics.Process]::Start($psi) | Out-Null
}

# ==================== 判断 + 启动 GPT-SoVITS ====================
$useGpt  = $false
$gptPort = 9880
if ($config -and $config.speech.provider -eq 'gpt_sovits') {
    $useGpt  = $true
    $gptBase = [string]$config.speech.gpt_sovits.base_url
    if ($gptBase -match ':(\d+)') { $gptPort = $Matches[1] }
}

if ($useGpt) {
    $py = Join-Path $gptSovitsRoot 'runtime\python.exe'
    if (-not (Test-Path $py)) {
        Write-Host "[!] 未找到 GPT-SoVITS runtime\python.exe（$gptSovitsRoot）。请在 config.json 的 services.gpt_sovits_root 填写正确路径。" -ForegroundColor Red
    } else {
        Start-InNewCmd 'GPT-SoVITS' $gptSovitsRoot "runtime\python.exe api_v2.py -a 127.0.0.1 -p $gptPort -c GPT_SoVITS/configs/tts_infer.yaml"
    }
} else {
    Write-Host '[i] speech.provider != gpt_sovits，跳过 GPT-SoVITS' -ForegroundColor Yellow
}

# ==================== 判断 + 启动 SenseVoice ====================
$useSense = $false
$asrPort  = 10095
if ($config -and $config.asr.model) {
    $asrBase = [string]$config.asr.base_url
    if ($asrBase -match '127\.0\.0\.1|localhost') {
        $useSense = $true
        if ($asrBase -match ':(\d+)') { $asrPort = $Matches[1] }
    } else {
        Write-Host "[i] ASR 配置为云端（$asrBase），跳过本地 SenseVoice" -ForegroundColor Yellow
    }
}

if ($useSense) {
    Start-InNewCmd 'SenseVoice' $ProjectRoot "`"$condaExe`" run -n $senseVoiceEnv python -m funasr_server --model $senseVoiceModel --port $asrPort"
} else {
    Write-Host '[i] 未配置本地 ASR（asr.model 为空或为云端），跳过 SenseVoice' -ForegroundColor Yellow
}

# ==================== 前端构建环境变量 ====================
if ($useSense) {
    $env:VITE_ASR_PROVIDER = 'model'
    $env:VITE_ASR_LANGUAGE = 'auto'
    Write-Host '[i] 已配置本地 SenseVoice → 前端用 VITE_ASR_PROVIDER=model / VITE_ASR_LANGUAGE=auto' -ForegroundColor Cyan
} else {
    Remove-Item Env:\VITE_ASR_PROVIDER -ErrorAction SilentlyContinue
    Remove-Item Env:\VITE_ASR_LANGUAGE -ErrorAction SilentlyContinue
    Write-Host '[i] 未配置本地 ASR → 前端用默认浏览器识别' -ForegroundColor Yellow
}

# ==================== 构建前端 ====================
if (-not $SkipBuild) {
    Write-Host '[+] 构建前端...' -ForegroundColor Cyan
    Set-Location (Join-Path $ProjectRoot 'frontend')
    npm run build
    if ($LASTEXITCODE -ne 0) {
        Write-Host '[!] 前端构建失败' -ForegroundColor Red
        exit 1
    }
    Set-Location $ProjectRoot
} else {
    Write-Host '[i] 跳过前端构建（-SkipBuild）' -ForegroundColor Yellow
}

# ==================== 启动 Wails ====================
Write-Host '[+] 启动 Wails（wails dev）...' -ForegroundColor Cyan
Set-Location $ProjectRoot
wails dev
