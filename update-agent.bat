@echo off
setlocal EnableExtensions

rem Update the installed ThesisAgentDev Service and Desktop Helper.
rem Place the prebuilt executables in the build folder before running:
rem   build\thesis-agent.exe
rem   build\thesis-agent-desktop.exe
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

set "AGENT_EXE=%~dp0build\thesis-agent.exe"
set "HELPER_EXE=%~dp0build\thesis-agent-desktop.exe"

if not exist "%AGENT_EXE%" (
    echo ERROR: Missing new Service executable:
    echo   %AGENT_EXE%
    pause
    exit /b 1
)

if not exist "%HELPER_EXE%" (
    echo ERROR: Missing new Desktop Helper executable:
    echo   %HELPER_EXE%
    pause
    exit /b 1
)

echo [1/4] Stopping Windows Service...
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
    "if (Get-Service -Name 'ThesisAgentDev' -ErrorAction SilentlyContinue) { Stop-Service -Name 'ThesisAgentDev' -ErrorAction Stop }"
if not "%errorlevel%"=="0" (
    echo ERROR: Could not stop ThesisAgentDev.
    pause
    exit /b 1
)

echo [2/4] Stopping Desktop Helper if it is running...
taskkill /IM thesis-agent-desktop.exe /F >nul 2>&1

echo [3/4] Installing the updated Service while preserving identity and config...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\dev-service.ps1" ^
    -Action Install ^
    -Executable "%AGENT_EXE%"
if not "%errorlevel%"=="0" (
    echo ERROR: Service update failed. Inspect the Agent log and run Status.
    pause
    exit /b 1
)

echo [4/4] Registering and starting the Desktop Helper task...
set "DESKTOP_HELPER_LAUNCHER=%~dp0run-desktop-helper-hidden.vbs"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
    "$launcher = $env:DESKTOP_HELPER_LAUNCHER; $action = New-ScheduledTaskAction -Execute (Join-Path $env:SystemRoot 'System32\wscript.exe') -Argument ([char]34 + $launcher + [char]34); $trigger = New-ScheduledTaskTrigger -AtLogOn -User ($env:USERDOMAIN + '\' + $env:USERNAME); Register-ScheduledTask -TaskName 'ThesisAgentDesktop' -Action $action -Trigger $trigger -RunLevel Limited -Force"
if not "%errorlevel%"=="0" (
    echo ERROR: Could not register the Desktop Helper logon task.
    pause
    exit /b 1
)
schtasks.exe /Run /TN "ThesisAgentDesktop" >nul 2>&1

echo.
echo Update completed.
echo Existing Agent identity and runtime configuration were preserved.
echo If the Desktop Helper task is not registered, run:
echo   install-desktop-helper-task.bat
pause
exit /b 0
