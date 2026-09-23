@echo off
setlocal EnableExtensions

rem Build and update the installed ThesisAgentDev Service and Desktop Helper.
rem Run this BAT as Administrator from the repository.

cd /d "%~dp0"

net session >nul 2>&1
if not "%errorlevel%"=="0" (
    echo ERROR: Run this file as Administrator.
    pause
    exit /b 1
)

if not exist "%~dp0go.mod" (
    echo ERROR: This file must be run from the project repository.
    pause
    exit /b 1
)

if not exist "%~dp0build" mkdir "%~dp0build"

echo [1/6] Stopping Windows Service...
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
    "if (Get-Service -Name 'ThesisAgentDev' -ErrorAction SilentlyContinue) { Stop-Service -Name 'ThesisAgentDev' -ErrorAction Stop }"
if not "%errorlevel%"=="0" (
    echo ERROR: Could not stop ThesisAgentDev.
    pause
    exit /b 1
)

echo [2/6] Stopping Desktop Helper if it is running...
taskkill /IM thesis-agent-desktop.exe /F >nul 2>&1

echo [3/6] Building Service executable...
go build -o "%~dp0build\thesis-agent.exe" .
if not "%errorlevel%"=="0" (
    echo ERROR: Service build failed. The installed Service was not updated.
    pause
    exit /b 1
)

echo [4/6] Building Desktop Helper executable...
go build -o "%~dp0build\thesis-agent-desktop.exe" .\cmd\thesis-agent-desktop
if not "%errorlevel%"=="0" (
    echo ERROR: Desktop Helper build failed. The installed Service was not updated.
    pause
    exit /b 1
)

echo [5/6] Installing the updated Service while preserving identity and config...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\dev-service.ps1" ^
    -Action Install ^
    -Executable "%~dp0build\thesis-agent.exe"
if not "%errorlevel%"=="0" (
    echo ERROR: Service update failed. Inspect the Agent log and run Status.
    pause
    exit /b 1
)

echo [6/6] Starting the Desktop Helper task if it is registered...
schtasks.exe /Run /TN "ThesisAgentDesktop" >nul 2>&1

echo.
echo Update completed.
echo Existing Agent identity and runtime configuration were preserved.
echo If the Desktop Helper task is not registered, run:
echo   install-desktop-helper-task.bat
pause
exit /b 0
