#requires -Version 5.1
<#
Development/admin tooling only. Run with Windows PowerShell 5.1 as Administrator.
All names and paths come from the built Agent's --service-info metadata.
Removal unregisters the Service and preserves binaries, identity, logs and config.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)]
    [ValidateSet('Install','Start','Status','Stop','Restart','Remove')]
    [string]$Action,
    [string]$Executable = (Join-Path $PSScriptRoot '..\build\thesis-agent-dev.exe'),
    [string]$ConfigPath,
    [string]$IdentityPath,
    [switch]$NewIdentity
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Run this development Service tool from an elevated Administrator terminal.'
}
if ($PSVersionTable.PSEdition -ne 'Desktop') {
    throw 'Use Windows PowerShell 5.1 (powershell.exe), which supports creating directories with an ACL atomically.'
}
$sourceExecutable = (Resolve-Path -LiteralPath $Executable).ProviderPath
$metadataText = & $sourceExecutable --service-info
if ($LASTEXITCODE -ne 0) { throw 'Unable to read Agent Service metadata.' }
$metadata = $metadataText | ConvertFrom-Json
$serviceName = [string]$metadata.name
$installRoot = [IO.Path]::GetFullPath([string]$metadata.paths.install)
$runtimeRoot = [IO.Path]::GetFullPath([string]$metadata.paths.root)
$installedExecutable = [IO.Path]::GetFullPath([string]$metadata.executable)
$expectedCommand = '"' + $installedExecutable + '" --service'
$adminSid = 'S-1-5-32-544'
$systemSid = 'S-1-5-18'
$callerSid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value

function Assert-NoReparseAncestors([string]$Path) {
    $candidate = [IO.Path]::GetFullPath($Path)
    while ($candidate) {
        if (Test-Path -LiteralPath $candidate) {
            $item = Get-Item -LiteralPath $candidate -Force
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "Refusing reparse point: $candidate"
            }
        }
        $parent = [IO.Directory]::GetParent($candidate)
        if ($null -eq $parent) { break }
        $candidate = $parent.FullName
    }
}
function Assert-OwnedPath([string]$Path) {
    $owner = (Get-Acl -LiteralPath $Path).GetOwner([Security.Principal.SecurityIdentifier]).Value
    if ($owner -notin @($adminSid,$systemSid,$callerSid)) {
        throw "Existing path has an unexpected owner; inspect it manually before provisioning: $Path"
    }
}
function New-DirectoryAcl([bool]$UsersRead) {
    $sddl = 'O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)'
    if ($UsersRead) { $sddl += '(A;OICI;GRGX;;;BU)' }
    $acl = New-Object Security.AccessControl.DirectorySecurity
    $acl.SetSecurityDescriptorSddlForm($sddl)
    return $acl
}
function Protect-Tree([string]$Path,[bool]$UsersRead) {
    Assert-NoReparseAncestors $Path
    $acl = New-DirectoryAcl $UsersRead
    if (-not (Test-Path -LiteralPath $Path)) {
        # Windows PowerShell/.NET Framework overload creates with the final ACL.
        [void][IO.Directory]::CreateDirectory($Path,$acl)
    }
    $pending = New-Object 'Collections.Generic.Queue[string]'
    $pending.Enqueue($Path)
    while ($pending.Count -gt 0) {
        $current = $pending.Dequeue()
        $item = Get-Item -LiteralPath $current -Force
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Refusing reparse point in Agent tree: $current"
        }
        Assert-OwnedPath $current
        if ($item.PSIsContainer) {
            Set-Acl -LiteralPath $current -AclObject (New-DirectoryAcl $UsersRead)
            foreach ($child in Get-ChildItem -LiteralPath $current -Force) { $pending.Enqueue($child.FullName) }
        } else {
            $fileAcl = New-Object Security.AccessControl.FileSecurity
            $sddl = 'O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)'
            if ($UsersRead) { $sddl += '(A;;GRGX;;;BU)' }
            $fileAcl.SetSecurityDescriptorSddlForm($sddl)
            Set-Acl -LiteralPath $current -AclObject $fileAcl
        }
    }
}
function Get-OwnedService {
    $existing = Get-CimInstance Win32_Service -Filter "Name='$serviceName'"
    if ($null -ne $existing -and $existing.PathName -ine $expectedCommand) {
        throw 'The Service name belongs to a different executable; refusing to modify it.'
    }
    return $existing
}
function Wait-ServiceState([string]$State) {
    $controller = Get-Service -Name $serviceName
    try { $controller.WaitForStatus([ServiceProcess.ServiceControllerStatus]::$State,[TimeSpan]::FromSeconds(30)) }
    finally { $controller.Dispose() }
}
function Stop-OwnedService {
    $existing = Get-OwnedService
    if ($null -ne $existing -and $existing.State -ne 'Stopped') {
        Stop-Service -Name $serviceName
        Wait-ServiceState 'Stopped'
    }
}

# Metadata must point to this project's dedicated child folders, never a root.
$expectedInstall = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) $serviceName
$expectedRuntime = Join-Path ([Environment]::GetFolderPath('CommonApplicationData')) $serviceName
if ($installRoot -ine $expectedInstall -or $runtimeRoot -ine $expectedRuntime -or
    $installedExecutable -ine (Join-Path $installRoot 'thesis-agent.exe') -or $serviceName -notmatch '^[A-Za-z0-9]+$') {
    throw 'Agent metadata does not describe the expected dedicated machine directories.'
}
Assert-NoReparseAncestors $installRoot
Assert-NoReparseAncestors $runtimeRoot
$existing = Get-OwnedService

switch ($Action) {
    'Install' {
        if ($null -ne $existing -and $existing.State -ne 'Stopped') {
            throw 'Stop the existing development Service before repair/update.'
        }
        if ($NewIdentity -and $IdentityPath) { throw 'Choose either IdentityPath or NewIdentity.' }
        if ($NewIdentity -and (Test-Path -LiteralPath $metadata.paths.identity)) {
            throw 'NewIdentity refuses an existing persistent identity. Existing state was not removed.'
        }
        if (-not (Test-Path -LiteralPath $metadata.paths.identity) -and -not $IdentityPath -and -not $NewIdentity) {
            throw 'For initial provisioning specify IdentityPath to preserve an existing identity, or NewIdentity for a genuinely new installation.'
        }
        if (-not $ConfigPath -and -not (Test-Path -LiteralPath $metadata.paths.config)) {
            throw 'Initial installation requires ConfigPath pointing to your reviewed service .env configuration.'
        }
        $sourceConfig = $null
        if ($ConfigPath) { $sourceConfig = (Resolve-Path -LiteralPath $ConfigPath).ProviderPath }
        $sourceIdentity = $null
        if ($IdentityPath) { $sourceIdentity = (Resolve-Path -LiteralPath $IdentityPath).ProviderPath }
        Protect-Tree $installRoot $true
        Protect-Tree $runtimeRoot $false
        if ($sourceExecutable -ine $installedExecutable) {
            Copy-Item -LiteralPath $sourceExecutable -Destination $installedExecutable -Force
        }
        if ($sourceConfig -and $sourceConfig -ine [string]$metadata.paths.config) {
            Copy-Item -LiteralPath $sourceConfig -Destination $metadata.paths.config -Force
        }
        Protect-Tree $installRoot $true
        Protect-Tree $runtimeRoot $false
        $provisionArguments = @('--provision')
        if ($sourceIdentity) { $provisionArguments += @('--migrate-identity',$sourceIdentity) }
        # The existing token prompt remains interactive. No token is stored by this script.
        & $installedExecutable @provisionArguments
        if ($LASTEXITCODE -ne 0) { throw 'Provisioning failed. Identity is retained; Service was not started.' }
        Protect-Tree $runtimeRoot $false
        if (-not [Diagnostics.EventLog]::SourceExists($serviceName)) {
            New-EventLog -LogName Application -Source $serviceName
        }
        & $installedExecutable --configure-service
        if ($LASTEXITCODE -ne 0) { throw 'Service configuration failed. Inspect the error; Service was not started.' }
        Start-Service -Name $serviceName
        Wait-ServiceState 'Running'
        Write-Output 'Service is Running. Verify API/WebSocket connectivity in the protected Agent log.'
    }
    'Start' {
        if ($null -eq $existing) { throw 'Development Service is not installed.' }
        Start-Service -Name $serviceName
        Wait-ServiceState 'Running'
    }
    'Status' {
        if ($null -eq $existing) { Write-Output 'Development Service is not installed.' }
        else {
            $existing | Select-Object Name,State,StartMode,StartName,ProcessId,PathName
            & "$env:SystemRoot\System32\sc.exe" qfailure $serviceName
            & "$env:SystemRoot\System32\sc.exe" qfailureflag $serviceName
            & "$env:SystemRoot\System32\sc.exe" sdshow $serviceName
        }
        Write-Output ("Log: " + $metadata.paths.log)
    }
    'Stop' { Stop-OwnedService }
    'Restart' {
        if ($null -eq $existing) { throw 'Development Service is not installed.' }
        Stop-OwnedService
        Start-Service -Name $serviceName
        Wait-ServiceState 'Running'
    }
    'Remove' {
        Stop-OwnedService
        if ($null -ne $existing) {
            & "$env:SystemRoot\System32\sc.exe" delete $serviceName
            if ($LASTEXITCODE -ne 0) { throw 'Unable to remove development Service registration.' }
        }
        Write-Output 'Service registration removed. Binaries, config, identity, enrollment state, logs and Event Log source are retained for repair/reinstall.'
    }
}
