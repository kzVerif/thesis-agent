#requires -Version 5.1
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)]
    [ValidateSet('InstallNew','InstallExisting','Update','Start','Stop','Restart','Status','Remove','HelperInstall','HelperStart','HelperStop','HelperRemove')]
    [string]$Action,
    [switch]$Check,
    [switch]$Elevated
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$packageRoot = Split-Path -Parent $PSScriptRoot
$agentExe = Join-Path $packageRoot 'build\agent\thesis-agent.exe'
$helperExe = Join-Path $packageRoot 'build\desktop-helper\thesis-agent-desktop.exe'
$configFile = Join-Path $packageRoot 'config\service.env'
$serviceScript = Join-Path $PSScriptRoot 'dev-service.ps1'
$launcherSource = Join-Path $PSScriptRoot 'run-desktop-helper-hidden.vbs'
$taskName = 'ThesisAgentDesktop'
$helperRoot = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) $taskName
$helperInstalled = Join-Path $helperRoot 'thesis-agent-desktop.exe'
$helperLauncher = Join-Path $helperRoot 'run-desktop-helper-hidden.vbs'

function Require-File([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "Required file is missing: $Path" }
}
function Assert-NoLinks([string]$Path) {
    $candidate = [IO.Path]::GetFullPath($Path)
    while ($candidate) {
        if (Test-Path -LiteralPath $candidate) {
            if (((Get-Item -LiteralPath $candidate -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "Refusing linked installation path: $candidate"
            }
        }
        $parent = [IO.Directory]::GetParent($candidate)
        if ($null -eq $parent) { break }
        $candidate = $parent.FullName
    }
}
function Stop-Helper {
    $task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($null -ne $task) {
        # Do not change another user's task from this per-user installer.
        $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
        $user = [Security.Principal.WindowsIdentity]::GetCurrent().Name
        if ($task.Principal.UserId -notin @($sid, $user, $env:USERNAME)) { throw 'Desktop task belongs to another user. Run this BAT in that user session.' }
        Stop-ScheduledTask -InputObject $task
    }
    $session = [Diagnostics.Process]::GetCurrentProcess().SessionId
    foreach ($process in Get-Process -Name 'thesis-agent-desktop' -ErrorAction SilentlyContinue) {
        if ($process.SessionId -eq $session) {
            Stop-Process -Id $process.Id -ErrorAction Stop
            $process.WaitForExit(10000) | Out-Null
        }
    }
}
function Install-Helper {
    Require-File $helperExe
    Require-File $launcherSource
    Assert-NoLinks $helperRoot
    Assert-NoLinks $helperInstalled
    Assert-NoLinks $helperLauncher
    Stop-Helper
    [void][IO.Directory]::CreateDirectory($helperRoot)
    Copy-Item -LiteralPath $helperExe -Destination $helperInstalled -Force
    Copy-Item -LiteralPath $launcherSource -Destination $helperLauncher -Force
    $user = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    $taskAction = New-ScheduledTaskAction -Execute (Join-Path $env:SystemRoot 'System32\wscript.exe') -Argument ('"' + $helperLauncher + '"') -WorkingDirectory $helperRoot
    $trigger = New-ScheduledTaskTrigger -AtLogOn -User $user
    $principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Limited
    $settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
    Register-ScheduledTask -TaskName $taskName -Action $taskAction -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
    Start-ScheduledTask -TaskName $taskName
    Write-Host 'Desktop Helper installed and started for the current user.'
}

try {
    if ($Action -like 'Helper*') {
        if ($Action -in @('HelperInstall','HelperStart')) { Require-File $helperExe; Require-File $launcherSource }
    } else {
        Require-File $agentExe
        Require-File $serviceScript
        if ($Action -in @('InstallNew','InstallExisting')) { Require-File $configFile }
        if ($Action -eq 'Update') { Require-File $helperExe; Require-File $launcherSource }
    }
    if ($Check) {
        Write-Host "PASS: $Action package files and paths are available. No system changes were made."
        exit 0
    }
    if ($Action -like 'Helper*') {
        switch ($Action) {
            'HelperInstall' { Install-Helper }
            'HelperStart' {
                if (-not (Test-Path -LiteralPath $helperInstalled) -or -not (Test-Path -LiteralPath $helperLauncher)) { Install-Helper }
                else {
                    Stop-Helper
                    Start-Process -FilePath (Join-Path $env:SystemRoot 'System32\wscript.exe') -ArgumentList ('"' + $helperLauncher + '"') -WindowStyle Hidden
                    Write-Host 'Desktop Helper started in the current desktop session.'
                }
            }
            'HelperStop' { Stop-Helper; Write-Host 'Desktop Helper stopped for this session.' }
            'HelperRemove' {
                Stop-Helper
                $task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
                if ($null -ne $task) { Unregister-ScheduledTask -InputObject $task -Confirm:$false }
                Write-Host 'Desktop Helper logon task removed. Local binaries are retained.'
            }
        }
        exit 0
    }

    $principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        if ($Elevated) { throw 'Administrator permission was not granted.' }
        # Elevate only Service work; keep desktop task installation in the caller's
        # original user/session, even when UAC uses a different admin account.
        $arguments = '-NoProfile -ExecutionPolicy Bypass -File "' + $PSCommandPath + '" -Action ' + $Action + ' -Elevated'
        $child = Start-Process -FilePath (Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe') -ArgumentList $arguments -Verb RunAs -Wait -PassThru
        if ($child.ExitCode -ne 0) { throw "Service action failed (exit $($child.ExitCode))." }
        if ($Action -eq 'Update') { Install-Helper }
        exit 0
    }

    $serviceAction = $Action
    $options = @{ Executable = $agentExe }
    switch ($Action) {
        'InstallNew' { $serviceAction = 'Install'; $options.ConfigPath = $configFile; $options.NewIdentity = $true }
        'InstallExisting' {
            $serviceAction = 'Install'
            $metadataText = & $agentExe --service-info
            if ($LASTEXITCODE -ne 0) { throw 'Unable to read Service metadata.' }
            $metadata = $metadataText | ConvertFrom-Json
            if (-not (Test-Path -LiteralPath $metadata.paths.identity)) { throw 'No installed machine identity. Use install-new-agent.bat for a new machine; use dev-service.ps1 -IdentityPath for explicit identity migration.' }
            # Reinstall keeps the installed configuration whenever it exists.
            if (-not (Test-Path -LiteralPath $metadata.paths.config)) { $options.ConfigPath = $configFile }
            & $serviceScript -Action Stop -Executable $agentExe
        }
        'Update' {
            $serviceAction = 'Install'
            & $serviceScript -Action Stop -Executable $agentExe
        }
    }
    & $serviceScript -Action $serviceAction @options
    if ($Action -eq 'Update' -and -not $Elevated) { Install-Helper }
    Write-Host "Completed: $Action"
    exit 0
} catch {
    Write-Host ('ERROR: ' + $_.Exception.Message) -ForegroundColor Red
    if ($Elevated) { [void](Read-Host 'Press Enter to close') }
    exit 1
}
