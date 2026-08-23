@echo off
setlocal
chcp 65001 >nul
title Yuyu Mind Start

REM Double-click entry: runs start-all.ps1 (config parsing + service spawn + build + wails dev).
REM Usage: start-all.bat [-SkipBuild]

if not exist "%~dp0start-all.ps1" (
    echo [ERROR] start-all.ps1 not found next to start-all.bat.
    pause
    exit /b 1
)

echo [i] Launching start-all.ps1 ...
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-all.ps1" %*
set "RC=%ERRORLEVEL%"
if not "%RC%"=="0" (
    echo.
    echo [ERROR] start-all.ps1 exited with code %RC%
)
echo.
pause
endlocal
