@echo off
setlocal EnableExtensions DisableDelayedExpansion
rem Paths are relative to this BAT. No manual editing is required.
set "TOOL=%~dp0..\..\scripts\manage.ps1"
if not exist "%TOOL%" (
    echo ERROR: Package is incomplete. Copy the entire install folder.
    if /i not "%~1"=="--check" pause
    exit /b 1
)
set "CHECK_OPTION="
if /i "%~1"=="--check" set "CHECK_OPTION=-Check"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" -Action Status %CHECK_OPTION%
set "RESULT=%errorlevel%"
if /i not "%~1"=="--check" pause
exit /b %RESULT%
