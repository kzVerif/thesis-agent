@echo off
setlocal EnableExtensions

rem Run the interactive Desktop Helper in the current user's desktop session.
rem Build it first with: go build -o build/thesis-agent-desktop.exe ./cmd/thesis-agent-desktop

set "HELPER_EXE=%~dp0build\thesis-agent-desktop.exe"
if not exist "%HELPER_EXE%" (
    echo ERROR: Missing %HELPER_EXE%
    echo Build it first with:
    echo   go build -o build/thesis-agent-desktop.exe ./cmd/thesis-agent-desktop
    pause
    exit /b 1
)

echo Starting Desktop Helper in the current user session...
echo Keep this window running while testing screen streaming.
"%HELPER_EXE%"
