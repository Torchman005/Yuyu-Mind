<#
  stop-all.ps1 —— 停止 Yuyu Mind 桌宠及其依赖服务

  按进程匹配并结束：
    - GPT-SoVITS（python.exe 命令行含 api_v2.py）
    - SenseVoice（python.exe 命令行含 funasr_server）
    - Yuyu 应用本体（进程名 Yuyu-Mind* 或 wails）

  用法：
    .\stop-all.ps1
#>
$ErrorActionPreference = 'SilentlyContinue'

$count = 0

# ---- 停止 GPT-SoVITS ----
Get-CimInstance Win32_Process -Filter "Name = 'python.exe'" |
    Where-Object { $_.CommandLine -like '*api_v2.py*' } |
    ForEach-Object {
        Write-Host "[.] 停止 GPT-SoVITS（PID $($_.ProcessId)）" -ForegroundColor Cyan
        Stop-Process -Id $_.ProcessId -Force
        $count++
    }

# ---- 停止 SenseVoice ----
Get-CimInstance Win32_Process |
    Where-Object { $_.CommandLine -like '*funasr-server*' -or $_.CommandLine -like '*funasr_server*' -or $_.CommandLine -like '*SenseVoiceSmall*' } |
    ForEach-Object {
        Write-Host "[.] 停止 SenseVoice（PID $($_.ProcessId)）" -ForegroundColor Cyan
        Stop-Process -Id $_.ProcessId -Force
        $count++
    }

# ---- 停止 Yuyu 应用本体（wails dev 或打包 exe）----
Get-Process |
    Where-Object { $_.ProcessName -like 'Yuyu-Mind*' -or $_.ProcessName -eq 'wails' } |
    ForEach-Object {
        Write-Host "[.] 停止 $($_.ProcessName)（PID $($_.Id)）" -ForegroundColor Cyan
        Stop-Process -Id $_.Id -Force
        $count++
    }

if ($count -eq 0) {
    Write-Host '[i] 没有发现运行中的相关进程' -ForegroundColor Yellow
} else {
    Write-Host "[+] 已停止 $count 个进程" -ForegroundColor Green
}
