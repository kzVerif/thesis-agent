# Remaining Windows acceptance procedures

Status on 2026-09-21: **NOT YET VERIFIED MANUALLY**.
Run these procedures on the designated development PC/VM. The implementation
session did not install a Service, reboot, switch accounts, or terminate a process.
Use a VM snapshot for tests that deliberately try to disable/delete/kill the
Service: a defective ACL could let an attempted operation succeed.

## 1. Prepare and install as Administrator

Use Windows PowerShell 5.1 as Administrator. Review Get-ExecutionPolicy -List:
the current development machine blocked .ps1 execution before the script ran.
Have the administrator permit the reviewed script through the organization's
approved policy/signing process if needed. No execution policy is changed by
the provisioning tool or these instructions.

Build the reviewed source, then select the correct identity for this installation.
Do not use a different machine's identity or delete identity to fix enrollment.
Save an Administrator-only backup of the existing identity/config outside the
installation before migration; keep private-key fields out of terminal output.

Create a reviewed .env outside source control containing the correct endpoints
and POWER_MODE=mock. Put POWER_MODE in this persistent file, not merely in the
administrator shell environment; SCM has a different environment.

From the Agent repository, after replacing these two example source paths:

    .\scripts\dev-service.ps1 -Action Install -ConfigPath 'D:\PrivateConfig\service.env' -IdentityPath 'D:\ExistingAgent\agent_config.json'
    .\scripts\dev-service.ps1 -Action Status
    sc.exe qc ThesisAgentDev
    sc.exe queryex ThesisAgentDev
    sc.exe qfailure ThesisAgentDev
    sc.exe qfailureflag ThesisAgentDev
    sc.exe sdshow ThesisAgentDev

Only for a genuinely new installation, replace -IdentityPath with -NewIdentity.
Enrollment may prompt for the existing registration token if REST returns 404.
Do not put the token in arguments, .env, test records, or logs.

Expect Automatic, LocalSystem, an explicitly quoted installed executable path,
and Running. Then verify enrollment and WebSocket connection in the Agent log.
Running alone does not prove Online. Expect recovery delays 5s, 30s, then no
action, and non-crash recovery disabled. On an existing installation repaired
from different recovery settings, Microsoft documents that the failure-action
flag change takes effect at the next system start; confirm after the planned
reboot ([failure-action flags](https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_failure_actions_flag)).

## 2. Record paths, permissions and identity baseline

In that elevated PowerShell window:

    $meta = .\build\thesis-agent-dev.exe --service-info | ConvertFrom-Json
    icacls.exe $meta.paths.install
    icacls.exe $meta.executable
    icacls.exe $meta.paths.root
    icacls.exe $meta.paths.identity
    icacls.exe $meta.paths.config
    icacls.exe $meta.paths.enrollment
    Get-Content -LiteralPath $meta.paths.log -Tail 60

Expect SYSTEM and Administrators FullControl. Builtin Users may read/execute
the program tree, but may not modify it; the runtime tree is restricted to
SYSTEM/Administrators. Inspect owner and protected inheritance as well as ACEs.
This inspection is not a substitute for running under a Standard User account.

Record only ID and hashes, never the full identity JSON:

    $baseline = [pscustomobject]@{
        AgentID = (Get-Content -LiteralPath $meta.paths.identity -Raw | ConvertFrom-Json).agent_id
        IdentitySHA256 = (Get-FileHash -LiteralPath $meta.paths.identity -Algorithm SHA256).Hash
        ConfigSHA256 = (Get-FileHash -LiteralPath $meta.paths.config -Algorithm SHA256).Hash
    }
    $baseline | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $meta.paths.root 'acceptance-baseline.json')

For migration, compare Get-FileHash on the explicit source identity with the
installed identity. First migration into an empty destination must preserve all
bytes. An already-existing equal identity remains authoritative.

## 3. Administrator control and graceful Stop

    .\scripts\dev-service.ps1 -Action Stop
    Start-Sleep -Seconds 40
    Get-Service -Name ThesisAgentDev
    .\scripts\dev-service.ps1 -Action Start
    .\scripts\dev-service.ps1 -Action Restart
    Get-Content -LiteralPath $meta.paths.log -Tail 100

Expect Stop to remain Stopped after 40 seconds, Start/Restart to work, and normal
stop/resource-cleanup log entries. Exercise cancellation while a controlled
download or scan runs. Use mock power commands only. Do not claim a successful
Windows shutdown or real Defender service cancellation solely from mock tests.

## 4. Standard User Service/process protection

Switch accounts manually when ready. Use an actual Standard User, not merely
an unelevated Administrator. Do not elevate any of these attempts:

    sc.exe queryex ThesisAgentDev
    Stop-Service -Name ThesisAgentDev -ErrorAction Stop
    sc.exe stop ThesisAgentDev
    sc.exe config ThesisAgentDev start= disabled
    sc.exe delete ThesisAgentDev

Run attempts individually and record their results. Expect Access denied for
mutations and continued Service operation. Services UI -> ThesisAgentDev -> Stop
should be unavailable or denied. In Task Manager -> Details, identify this exact
Service process, attempt End task, and expect Access denied.

For the PowerShell process test, obtain the PID through the Service record,
confirm it is still that Service, and use it rather than a guessed PID:

    $agentService = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
    if ($null -eq $agentService -or $agentService.ProcessId -eq 0) { throw 'Service is not running' }
    Stop-Process -Id $agentService.ProcessId -Force -ErrorAction Stop

Expect Access denied. If an attempt unexpectedly succeeds, stop the acceptance
run and retain the failure evidence; do not label other protection checks PASS.

## 5. Standard User file protection without changing identity

To avoid destroying an identity if permissions are wrong, request write/delete
access without performing any write, truncate, rename or delete. An Administrator
should first stop the Service and verify the four target files exist, so sharing
violations cannot be mistaken for ACL denial. Then use the Standard User session.

The following calls use OPEN_EXISTING with no delete-on-close flag; dispose all
handles immediately. This checks granted access, not an actual deletion workflow
([CreateFileW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)).

    Add-Type @'
    using System;
    using System.Runtime.InteropServices;
    using Microsoft.Win32.SafeHandles;
    public static class AgentFileAccessProbe {
        [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
        public static extern SafeFileHandle CreateFileW(
            string path, uint access, uint share, IntPtr security,
            uint creation, uint flags, IntPtr template);
    }
    '@
    $programs = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'ThesisAgentDev'
    $runtime = Join-Path ([Environment]::GetFolderPath('CommonApplicationData')) 'ThesisAgentDev'
    $targets = @(
        (Join-Path $programs 'thesis-agent.exe'),
        (Join-Path $runtime 'agent_config.json'),
        (Join-Path $runtime '.env'),
        (Join-Path $runtime 'enrollment_state.json')
    )
    foreach ($target in $targets) {
        foreach ($access in @([uint32]0x40000000, [uint32]0x10000)) {
            $handle = [AgentFileAccessProbe]::CreateFileW($target, $access, 7, [IntPtr]::Zero, 3, 128, [IntPtr]::Zero)
            $win32Error = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
            $wasDenied = $handle.IsInvalid -and $win32Error -eq 5
            $handle.Dispose()
            if (-not $wasDenied) { throw "FAIL or inconclusive: path=$target access=$access error=$win32Error" }
            "PASS access denied: $target access=$access"
        }
    }

Expect error 5 (Access denied) for both GENERIC_WRITE and DELETE. Error 2
(missing file) or 32 (sharing violation) is inconclusive, not PASS.

For actual Explorer edit/replace/rename/delete acceptance, use a disposable VM
snapshot and the Administrator-only backup. Attempt those operations individually
against the installed binary, identity, .env and enrollment state as Standard
User. Expect denial. If anything succeeds, stop and restore the snapshot/backup;
never provision a replacement identity to conceal the failure.

An Administrator then starts the Service again and verifies identity hashes.

## 6. Reboot, Auto Start and identity stability

Choose the reboot time manually. Leave Windows at the login screen long enough
to observe the registered Agent becoming Online from another administration PC.
Record timestamps there, then log in and compare with the Service log and boot:

    (Get-CimInstance Win32_OperatingSystem).LastBootUpTime
    Get-Service -Name ThesisAgentDev
    Get-Content -LiteralPath $meta.paths.log -Tail 100
    $baseline = Get-Content -LiteralPath (Join-Path $meta.paths.root 'acceptance-baseline.json') -Raw | ConvertFrom-Json
    $currentID = (Get-Content -LiteralPath $meta.paths.identity -Raw | ConvertFrom-Json).agent_id
    if ($currentID -ne $baseline.AgentID) { throw 'Agent ID changed' }
    if ((Get-FileHash -LiteralPath $meta.paths.identity -Algorithm SHA256).Hash -ne $baseline.IdentitySHA256) { throw 'Identity bytes changed' }
    if ((Get-FileHash -LiteralPath $meta.paths.config -Algorithm SHA256).Hash -ne $baseline.ConfigSHA256) { throw 'Config changed' }

Reinitialize $meta using the installed executable's --service-info in a new
terminal if necessary. Agent log timestamps are UTC. A Service observed Running
after login is insufficient evidence of startup before login.

## 7. Infrastructure outage and reconnect

On dedicated test infrastructure, arrange REST unavailability, then manually
reboot the provisioned Agent machine. Expect locally Running plus retry logs,
no token prompt and unchanged identity. Restore REST: /exists must return 204
and the Agent should connect to WS automatically. A deliberate 404 instead
requires administrative provisioning with the same identity.

Restart the test WebSocket server while connected. Expect bounded varied retry
delays and reconnection using the same initial Agent ID. If the TCP/WS handshake
fails silently, dial/heartbeat timeouts also contribute to total recovery time.

For several machines, provision each with its own identity and the same WS
server URL/port. Start them together and verify distinct registry entries and
independent reconnects. Cloning one identity tests duplicate-session replacement,
not multi-Agent support.

## 8. Controlled crash recovery

Perform only on the designated disposable development machine as Administrator,
after successful provisioning. Verify the exact Service process before killing it:

    $agentService = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
    if ($null -eq $agentService -or $agentService.ProcessId -eq 0) { throw 'Service is not running' }
    $agentProcess = Get-Process -Id $agentService.ProcessId -ErrorAction Stop
    if ($agentProcess.Path -ine $meta.executable) { throw 'PID does not match installed Agent' }
    Stop-Process -Id $agentProcess.Id -Force -ErrorAction Stop
    Start-Sleep -Seconds 8
    sc.exe queryex ThesisAgentDev

Expect a new PID after the first configured 5s delay. On the second crash within
the same failure-count window, wait at least 35s for the 30s restart. A third
crash should remain stopped. Re-query and validate PID/path before each attempt;
never reuse an old PID. Failure count resets after 24 hours without failures,
so record previous crashes when interpreting the sequence.

Use Administrator Start after investigating the final stopped state. Re-run
the intentional Stop test separately; it must remain stopped.

## 9. Console regression and removal

Use an isolated console installation with its correct existing identity/config.
Run go run ., verify attached terminal plus file logging, Ctrl+C, and the existing
performance/process/download/scan/mock-power flows. Check screen JPEG delivery
in console mode. In Service mode, screen requests should leave the Agent alive;
the current Dashboard can remain Waiting because there is no availability message.

When finished with this development Service:

    .\scripts\dev-service.ps1 -Action Remove
    sc.exe query ThesisAgentDev

Expect Service-not-installed (1060) after removal, with identity/config/logs and
the Event Log source retained. No recursive data deletion is part of Remove.
Record the actual results of each scenario; this document is a procedure, not
evidence that any of these scenarios passed.
