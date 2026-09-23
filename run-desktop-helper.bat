@echo off
setlocal EnableExtensions

rem Run the interactive Desktop Helper in the current user's desktop session.
rem The helper is started without a visible Command Prompt window.

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

echo Starting Desktop Helper in the background...
wscript.exe "%HELPER_LAUNCHER%"
exit /b 0
