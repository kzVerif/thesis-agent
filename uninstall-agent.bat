@echo off
setlocal EnableExtensions

rem Uninstaller for the ThesisAgentDev Windows Service.
rem Run this BAT as Administrator.

cd /d "%~dp0"

net session >nul 2>&1
if not "%errorlevel%"=="0" (
    echo ERROR: Run this file as Administrator.
    pause
    exit /b 1
)

set "REMOVE_SCRIPT=%~dp0scripts\dev-service.ps1"
set "INSTALLED_EXE=%ProgramFiles%\ThesisAgentDev\thesis-agent.exe"
set "BUILD_EXE=%~dp0build\thesis-agent.exe"

if not exist "%REMOVE_SCRIPT%" (
    echo ERROR: Missing %REMOVE_SCRIPT%
    pause
    exit /b 1
)

rem Prefer the protected installed executable because it contains the service metadata.
if exist "%INSTALLED_EXE%" (
    set "AGENT_EXE=%INSTALLED_EXE%"
) else if exist "%BUILD_EXE%" (
    set "AGENT_EXE=%BUILD_EXE%"
) else (
    echo ERROR: Could not find the Agent executable.
    echo Checked:
    echo   %INSTALLED_EXE%
    echo   %BUILD_EXE%
    pause
    exit /b 1
)

echo.
echo Removing the ThesisAgentDev Windows Service...
echo Executable: %AGENT_EXE%
echo.

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%REMOVE_SCRIPT%" ^
    -Action Remove ^
    -Executable "%AGENT_EXE%"

if not "%errorlevel%"=="0" (
    echo.
    echo ERROR: Service removal failed. No runtime data was deleted.
    pause
    exit /b %errorlevel%
)

echo.
echo Service registration removed.
echo Runtime files, identity, configuration and logs are still preserved.
echo.
choice /C YN /N /M "Delete all local Agent files and identity now? [Y/N] "
if errorlevel 2 goto :done

echo.
echo Deleting local Agent files...
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
    "Remove-Item -LiteralPath (Join-Path $env:ProgramFiles 'ThesisAgentDev') -Recurse -Force -ErrorAction SilentlyContinue; Remove-Item -LiteralPath (Join-Path $env:ProgramData 'ThesisAgentDev') -Recurse -Force -ErrorAction SilentlyContinue"

if not "%errorlevel%"=="0" (
    echo WARNING: Some local files could not be deleted.
    pause
    exit /b 1
)

echo Local Agent files and identity deleted.

:done
echo.
echo Uninstallation completed.
echo Note: This does not remove the Agent record from the remote server.
pause
exit /b 0
