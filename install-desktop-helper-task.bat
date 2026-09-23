@echo off
setlocal EnableExtensions

rem Register the Desktop Helper for the current user at logon.

set "HELPER_EXE=%~dp0build\thesis-agent-desktop.exe"
set "HELPER_LAUNCHER=%~dp0run-desktop-helper-hidden.vbs"
if not exist "%HELPER_EXE%" (
    echo ERROR: Missing %HELPER_EXE%
    echo Place the prebuilt executable in the build folder first.
    pause
    exit /b 1
)
if not exist "%HELPER_LAUNCHER%" (
    echo ERROR: Missing %HELPER_LAUNCHER%
    pause
    exit /b 1
)

set "DESKTOP_HELPER_LAUNCHER=%HELPER_LAUNCHER%"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
    "$launcher = $env:DESKTOP_HELPER_LAUNCHER; $action = New-ScheduledTaskAction -Execute (Join-Path $env:SystemRoot 'System32\wscript.exe') -Argument ([char]34 + $launcher + [char]34); $trigger = New-ScheduledTaskTrigger -AtLogOn -User ($env:USERDOMAIN + '\' + $env:USERNAME); Register-ScheduledTask -TaskName 'ThesisAgentDesktop' -Action $action -Trigger $trigger -RunLevel Limited -Force"

if not "%errorlevel%"=="0" (
    echo ERROR: Could not register the logon task.
    pause
    exit /b 1
)

echo Desktop Helper will start automatically when this user logs on.
pause
