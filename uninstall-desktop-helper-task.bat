@echo off
setlocal EnableExtensions

powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
    "Unregister-ScheduledTask -TaskName 'ThesisAgentDesktop' -Confirm:$false -ErrorAction SilentlyContinue"

echo Desktop Helper logon task removed.
pause
