@echo off
setlocal EnableExtensions

rem One-click installer for a new Windows Agent machine.
rem Expected files beside this BAT:
rem   thesis-agent.exe
rem   service.env
rem The BAT must be run from an elevated Administrator context.

cd /d "%~dp0"

net session >nul 2>&1
if not "%errorlevel%"=="0" (
    echo ERROR: Run this file as Administrator.
    pause
    exit /b 1
)

set "AGENT_EXE=%~dp0build/thesis-agent.exe"
set "CONFIG_FILE=%~dp0build/service.env"
set "INSTALL_SCRIPT=%~dp0scripts\dev-service.ps1"

if not exist "%AGENT_EXE%" (
    echo ERROR: Missing %AGENT_EXE%
    pause
    exit /b 1
)

if not exist "%CONFIG_FILE%" (
    echo ERROR: Missing %CONFIG_FILE%
    pause
    exit /b 1
)

if not exist "%INSTALL_SCRIPT%" (
    echo ERROR: Missing %INSTALL_SCRIPT%
    pause
    exit /b 1
)

echo Installing a NEW Agent identity and Windows Service...
echo Existing identity files are never replaced by this command.
echo.

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%INSTALL_SCRIPT%" ^
    -Action Install ^
    -Executable "%AGENT_EXE%" ^
    -ConfigPath "%CONFIG_FILE%" ^
    -NewIdentity

if not "%errorlevel%"=="0" (
    echo.
    echo ERROR: Installation failed. Review the message above and the Agent log.
    pause
    exit /b %errorlevel%
)

echo.
echo Installation completed. Check Service status and Agent log before use.
pause
exit /b 0
