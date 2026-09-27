@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "TOOL=%~dp0..\scripts\build.ps1"
if not exist "%TOOL%" (
    echo ERROR: Package is incomplete. Copy the entire install folder.
    if /i not "%~1"=="--check" pause
    exit /b 1
)
set "CHECK_OPTION="
if /i "%~1"=="--check" set "CHECK_OPTION=-Check"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" %CHECK_OPTION%
set "RESULT=%errorlevel%"
if /i not "%~1"=="--check" pause
exit /b %RESULT%
