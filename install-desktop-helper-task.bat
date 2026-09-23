@echo off
setlocal EnableExtensions

rem Register the Desktop Helper for the current user at logon.

set "HELPER_EXE=%~dp0build\thesis-agent-desktop.exe"
if not exist "%HELPER_EXE%" (
    echo ERROR: Missing %HELPER_EXE%
    echo Build it first with:
    echo   go build -o build/thesis-agent-desktop.exe ./cmd/thesis-agent-desktop
    pause
    exit /b 1
)

set "DESKTOP_HELPER_EXE=%HELPER_EXE%"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
    "$action = New-ScheduledTaskAction -Execute $env:DESKTOP_HELPER_EXE; $trigger = New-ScheduledTaskTrigger -AtLogOn -User ($env:USERDOMAIN + '\' + $env:USERNAME); Register-ScheduledTask -TaskName 'ThesisAgentDesktop' -Action $action -Trigger $trigger -RunLevel Limited -Force"

if not "%errorlevel%"=="0" (
    echo ERROR: Could not register the logon task.
    pause
    exit /b 1
)

echo Desktop Helper will start automatically when this user logs on.
pause
