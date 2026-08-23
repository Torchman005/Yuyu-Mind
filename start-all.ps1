<#
  start-all.ps1 —— 一键启动 Yuyu Mind 桌宠及依赖服务

  会读取 config.json，按配置自动判断是否启动：
    - GPT-SoVITS（当 speech.provider == "gpt_sovits"）
    - SenseVoice（当 asr.model 已配置且 asr.base_url 为本地地址）

  用法：
    .\start-all.ps1             # 构建前端 + 启动所有服务 + wails dev
    .\start-all.ps1 -SkipBuild  # 跳过前端构建（已构建过时用）
#>
param(
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'

# ==================== 可修改配置 ====================
$ProjectRoot    = 'D:\itJinYu_toolkit\AI-pet\Yuyu-Mind2'
$GptSovitsRoot  = 'D:\itJinYu_toolkit\GPT-SoVITS-v2pro-20250604\GPT-SoVITS-v2pro-20250604'
$CondaExe       = 'conda'   # 若 conda 不在 PATH，改成绝对路径，如 'C:\Users\23182\miniconda3\Scripts\conda.exe'
$SenseVoiceEnv  = 'funasr'  # 你安装 FunASR 的 conda 环境名
$SenseVoiceModel = 'iic/SenseVoiceSmall'
# ====================================================

# ---- 定位 config.json ----
$configDir  = if ($env:YUYU_CONFIG_DIR) { $env:YUYU_CONFIG_DIR } else { Join-Path $env:APPDATA 'Yuyu-Mind' }
$configPath = Join-Path $configDir 'config.json'

$config = $null
if (Test-Path $configPath) {
    $config = Get-Content $configPath -Raw | ConvertFrom-Json
} else {
    Write-Host "[!] 未找到 config.json：$configPath" -ForegroundColor Yellow
    Write-Host '    将不启动 GPT-SoVITS / SenseVoice（按默认配置运行）'
}

# ---- 在新窗口启动一个服务（cmd /k 保持窗口） ----
function Start-InNewCmd([string]$Title, [string]$WorkDir, [string]$Cmd) {
    Write-Host "[+] 启动 $Title ..." -ForegroundColor Cyan
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = 'cmd.exe'
    $psi.Arguments = "/k title $Title & cd /d `"$WorkDir`" & $Cmd"
    $psi.UseShellExecute = $true
    [System.Diagnostics.Process]::Start($psi) | Out-Null
}

# ==================== 判断 GPT-SoVITS ====================
$useGpt  = $false
$gptPort = 9880
if ($config -and $config.speech.provider -eq 'gpt_sovits') {
    $useGpt  = $true
    $gptBase = [string]$config.speech.gpt_sovits.base_url
    if ($gptBase -match ':(\d+)') { $gptPort = $Matches[1] }
}

if ($useGpt) {
    $py = Join-Path $GptSovitsRoot 'runtime\python.exe'
    if (-not (Test-Path $py)) {
        Write-Host "[!] 未找到 GPT-SoVITS 的 runtime\python.exe（$GptSovitsRoot），跳过。请改脚本里的 GptSovitsRoot。" -ForegroundColor Red
    } else {
        Start-InNewCmd 'GPT-SoVITS' $GptSovitsRoot "runtime\python.exe api_v2.py -a 127.0.0.1 -p $gptPort -c GPT_SoVITS/configs/tts_infer.yaml"
    }
} else {
    Write-Host '[i] speech.provider != gpt_sovits，跳过 GPT-SoVITS' -ForegroundColor Yellow
}

# ==================== 判断 SenseVoice ====================
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
    Start-InNewCmd 'SenseVoice' $ProjectRoot "`"$CondaExe`" run -n $SenseVoiceEnv python -m funasr_server --model $SenseVoiceModel --port $asrPort"
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
