@echo off
setlocal
chcp 65001 >nul
title Yuyu Mind Stop

REM Double-click entry: runs stop-all.ps1 (kills GPT-SoVITS / SenseVoice / Yuyu app).

if not exist "%~dp0stop-all.ps1" (
    echo [ERROR] stop-all.ps1 not found next to stop-all.bat.
    pause
    exit /b 1
)

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop-all.ps1" %*
echo.
pause
endlocal
