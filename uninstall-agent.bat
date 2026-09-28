@echo off
setlocal EnableExtensions DisableDelayedExpansion
rem Open normally in the user account that installed Desktop Helper.
set "TOOL=%~dp0install\scripts\manage.ps1"
if not exist "%TOOL%" (
    echo ERROR: Keep this BAT beside the complete install folder.
    set "RESULT=1"
    goto :done
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" -Action Remove -Check
if errorlevel 1 goto :failed
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" -Action HelperRemove -Check
if errorlevel 1 goto :failed
if /i "%~1"=="--check" (
    echo Uninstall package check completed. No system changes were made.
    set "RESULT=0"
    goto :done
)
echo Removing installed Agent config, identity, enrollment, logs and downloads.
echo [1/2] Removing Agent Service and its installed files...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" -Action Remove
if errorlevel 1 goto :failed
echo [2/2] Removing Desktop Helper for the current user...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%TOOL%" -Action HelperRemove
if errorlevel 1 goto :failed
echo Uninstall completed. Reinstallation requires a new identity enrollment.
set "RESULT=0"
goto :done
:failed
echo ERROR: Uninstall incomplete. Review the error above and retry.
set "RESULT=1"
:done
if /i not "%~1"=="--check" pause
exit /b %RESULT%
