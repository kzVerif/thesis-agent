@echo off
setlocal EnableExtensions

rem Continue installation using the existing machine identity.
rem Expected files beside this BAT:
rem   thesis-agent.exe
rem   service.env

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
set "IDENTITY_FILE=%ProgramData%\ThesisAgentDev\agent_config.json"

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

if not exist "%IDENTITY_FILE%" (
    echo ERROR: Existing machine identity was not found:
    echo         %IDENTITY_FILE%
    echo Use install-new-agent.bat only for a genuinely new machine.
    pause
    exit /b 1
)

echo Continuing with the existing Agent identity...
echo The existing Agent ID and key will be preserved.
echo.

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%INSTALL_SCRIPT%" ^
    -Action Install ^
    -Executable "%AGENT_EXE%" ^
    -ConfigPath "%CONFIG_FILE%"

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
