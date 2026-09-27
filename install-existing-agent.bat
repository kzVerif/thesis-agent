@echo off
setlocal EnableExtensions DisableDelayedExpansion
rem Install the Agent Service and Desktop Helper together for the current user.
rem Open normally; the Service step requests Administrator permission through UAC.
set "TOOL=%~dp0install\scripts\manage.ps1"
if not exist "%TOOL%" (
    echo ERROR: Keep this BAT beside the complete install folder.
    set "RESULT=1"
    goto :done
)
echo [1/3] Checking Agent and Desktop Helper package files...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" -Action InstallExisting -Check
if errorlevel 1 goto :failed
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" -Action HelperInstall -Check
if errorlevel 1 goto :failed
if /i "%~1"=="--check" (
    echo Package check completed. No system changes were made.
    set "RESULT=0"
    goto :done
)
echo.
echo [2/3] Updating or reinstalling Agent Service, preserving the existing identity and configuration...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" -Action InstallExisting
if errorlevel 1 goto :failed
echo.
echo [3/3] Installing and starting Desktop Helper for the current user...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" -Action HelperInstall
if errorlevel 1 goto :helper_failed
echo.
echo Installation completed: Agent Service and Desktop Helper are started.
echo Agent starts with Windows. Desktop Helper starts when this user logs on.
set "RESULT=0"
goto :done
:helper_failed
echo.
echo ERROR: Agent Service was installed, but Desktop Helper setup failed.
echo Retry install\desktop-helper\setup\install-desktop-helper-task.bat in this user session.
set "RESULT=1"
goto :done
:failed
echo.
echo ERROR: Setup stopped. Review the error above before retrying.
set "RESULT=1"
:done
if /i not "%~1"=="--check" pause
exit /b %RESULT%
