# Windows Service lifecycle (development provisioning)

**Phase 4 update:** Agent WebSocket now requires Ed25519 proof before normal
traffic. Service startup still accepts legacy identity, but authenticated
connectivity requires the explicit Phase 2 migration. See
[Agent authentication](agent-authentication.md) for current sequencing and rollout.
Historical protocol/scope statements below describe the original Service phase.

This is development/admin tooling for an authorized managed lab. It is not a release installer.
Only thesis-agent is changed. The existing REST and WebSocket contracts are retained.

Current results: [verification report](windows-service-verification.md).
Exact remaining manual procedures: [Windows acceptance](windows-service-acceptance.md).

## Architecture and launch modes

main selects console, administrative provisioning, or the native SCM host.
internal/agent owns one shared runtime. internal/apppaths resolves explicit paths;
internal/retry supplies equal-jitter exponential backoff. The existing service/
providers, client/, download/, and antivirus/ implementations remain in use.

- `go run .`: attached console, Ctrl+C/SIGTERM where supported, interactive
  enrollment when needed, and both terminal and rotating file logs.
- `--provision`: elevated, interactive enrollment using machine paths, then exit.
- `--migrate-identity ABSOLUTE_PATH`: only with provisioning; copy existing bytes.
- `--migrate-private-key-protection`: separate elevated operation; stopped Service,
  existing protected runtime identity and original decrypting user context required.
- `--verify-private-key`: stopped-Service maintenance check; no network or file changes
  to identity. See [private-key protection](private-key-protection.md).
- `--service`: SCM only; no stdin or terminal. Normal SCM auto-detection also works.
- `--service-info`: read-only JSON containing names/paths for development tooling.
- `--configure-service`: elevated development helper used by the script, allowed
  only from the installed Program Files executable. It configures SCM, not files.

The previous automatic DetachConsole call is removed from the launch path.
Its platform helpers are retained as contributor code, but no current mode calls
FreeConsole. Existing InitLogging file support is extended, not replaced.

## Paths and configuration

The centralized name is `ThesisAgentDev`. Windows Known Folder APIs resolve roots
without assuming the system drive is C:.

| Item | Machine location |
| --- | --- |
| Executable | %ProgramFiles%\ThesisAgentDev\thesis-agent.exe |
| Identity | %ProgramData%\ThesisAgentDev\agent_config.json |
| Enrollment history | %ProgramData%\ThesisAgentDev\enrollment_state.json |
| Configuration | %ProgramData%\ThesisAgentDev\.env |
| Logs | %ProgramData%\ThesisAgentDev\logs\agent.log |
| Downloads | %ProgramData%\ThesisAgentDev\data\downloads |
| Runtime lock | %ProgramData%\ThesisAgentDev\.runtime.lock |

Console uses its working directory for the existing .env, identity, logs and
data/downloads convention. Service/provisioning never loads SCM's working-directory
.env. Relative Service log/download overrides resolve against the machine runtime
root. Service logs must remain below logs/, and downloads below a child of data/;
this prevents a download/log destination from overwriting critical identity/config.

Precedence: process environment (SCM inherits its system environment) >
the selected .env > existing defaults. Administrative provisioning and SCM can
inherit different environments; use the persistent .env and check conflicting
system variables. Service environment changes may require restarting Windows.

Preserved settings include WS_SERVER_URL, AGENT_API_URL, AGENT_LOG_PATH,
DOWNLOAD_DIRECTORY, MAX_CONCURRENT_DOWNLOADS, DOWNLOAD_QUEUE_SIZE,
MAX_DOWNLOAD_SIZE_BYTES, ALLOW_LOCAL_HTTP_DOWNLOADS, and POWER_MODE.
Power defaults to real shutdown as before. Use POWER_MODE=mock on development
machines while exercising connection/lifecycle behavior.

## Identity and enrollment

Identity and enrollment are separate. A file existing does not prove a successful
registration. Identity validation checks JSON, UUID, Ed25519 algorithm, public-key
base64/size, and nonempty base64 ciphertext. It does not decrypt DPAPI, authenticate
ciphertext, or prove the two key fields form a pair.

Phase 2 keeps that structural startup path. Legacy identities continue to run
existing features with a migration notice; Service startup never decrypts or
silently migrates keys. Unknown protection versions fail closed.
The separate `service.LoadPrivateKey` requires machine protection and validates
the decrypted key against its seed and stored public key.
New Service provisioning uses `dpapi-machine-v1` in the existing protected directory.
New console identities retain user-scope DPAPI; they must be explicitly migrated
after import before future Service private-key use.

Existing partial/corrupt identities fail closed, without automatic replacement.
Only an explicitly interactive first provisioning may create a missing identity.
Service cannot create an identity, request tokens, or silently repair keys.

Migration takes an explicit absolute source path during provisioning. The script
requires either an existing source, an existing destination, or an explicit
-NewIdentity choice for a genuinely new installation. It does not automatically
select the tracked repository identity. Migration validates both existing files,
copies source bytes (including unknown JSON fields), retains the source, and
never overwrites the destination. A different identity/key at the destination
fails. An equal identity already there remains authoritative.

Complete new files are flushed and published in the same directory. Identity
creation/migration uses an exclusive hard link publication on NTFS to prevent a
race overwriting an existing destination. Enrollment state uses atomic replacement.
An OS byte/file lock prevents concurrent runtimes or provisioning commands from
writing the same installation; it releases on process exit/crash. The empty lock
file can remain on disk and is not an indication that the process is running.

The marker records Agent ID, SHA-256 public-key fingerprint, configured API, and
historical verification. It is protected by the runtime directory ACL. It is not
authentication and never bypasses the REST existence check.

| State / response | Service behavior |
| --- | --- |
| Missing or corrupt identity | Stop with provisioning/config error; no new identity |
| Identity valid, marker absent/mismatched | Unknown; verify through existing endpoint |
| /exists returns 204 | Persist matching marker; connect WebSocket |
| /exists returns 404, even with old marker | Mark unverified; stop; administrator provisioning needed |
| Network failure, 408, 429, 5xx | Stay alive and retry; no token prompt |
| Other HTTP errors | Stop with configuration error; no blind retry |
| Interactive /register success | Require returned ID to match; persist marker |

A registration request whose response is lost may already have succeeded:
retry provisioning with the same identity so /exists decides. Never delete the
identity or create a replacement to work around an API error.

DPAPI code/scope is unchanged. Existing ciphertext is copied as opaque data.
New provisioning encrypts under the provisioning account, exactly as the current
implementation did. LocalSystem is not claimed to be able to decrypt ciphertext
created by that account. Current normal runtime does not decrypt it. A later
signature/security feature must address that account migration explicitly.

## Build and first installation

Prerequisites: Windows on NTFS, Go matching go.mod, Windows PowerShell 5.1,
an Administrator terminal for provisioning, and existing REST/WS infrastructure
using the same Agent database. The server needs its existing public_key migration.

Build from the repository (not elevated):

```powershell
go test ./...
go vet ./...
go build -o build/thesis-agent-dev.exe .
```

Prepare a reviewed service .env in a private location outside source control.
Example development settings (replace addresses for your infrastructure):

```dotenv
AGENT_API_URL=http://localhost:8080
WS_SERVER_URL=ws://localhost:8081/ws
POWER_MODE=mock
```

Do not put enrollment tokens in this file or command arguments.
The existing token prompt is used only when /exists returns 404.

From an elevated **Windows PowerShell 5.1** terminal in the repository:

```powershell
# Preserve this installation's existing identity.
.\scripts\dev-service.ps1 -Action Install `
  -ConfigPath 'D:\PrivateConfig\service.env' `
  -IdentityPath 'D:\ExistingAgent\agent_config.json'

# Alternative: only for a genuinely new installation without an old identity.
.\scripts\dev-service.ps1 -Action Install `
  -ConfigPath 'D:\PrivateConfig\service.env' -NewIdentity
```

Choose one command, not both. Replace the example paths with your real paths.
Do not copy the repository's tracked identity to another computer.

Install creates dedicated directories with ACLs, copies the development binary
and selected config, migrates/enrolls interactively, installs an Application Event
Log source, configures SCM, then starts the service. It refuses non-elevated use,
unexpected directory owners, reparse points, another executable under the same
service name, and a running service during repair/update. A failure retains
identity; inspect the error and rerun after correction. This is not a transactional
installer: a failed setup may leave protected directories or a stopped registration.

## Start, stop, status, repair and removal

```powershell
.\scripts\dev-service.ps1 -Action Status
.\scripts\dev-service.ps1 -Action Stop
.\scripts\dev-service.ps1 -Action Start
.\scripts\dev-service.ps1 -Action Restart
```

The script reads metadata from build/thesis-agent-dev.exe by default. Keep that
development build, or supply -Executable with the installed executable path.
Status shows account, startup mode, process ID, recovery configuration, and DACL.
Running means local runtime initialized; check the log for enrollment/connectivity.

For update/repair: Stop, rebuild, and run Install again. Existing persistent
identity/config are reused; ConfigPath is optional after initial installation.
Supplying IdentityPath again checks for conflict. Do not use NewIdentity on an
existing installation.

```powershell
.\scripts\dev-service.ps1 -Action Remove
```

Remove stops and unregisters only this Service. It deliberately preserves program
files, runtime data, keys, logs, and Event Log source for repair/reinstall. No
recursive data deletion or repository-history cleanup is performed.

## Windows permissions and service account

The development Service runs in its own LocalSystem process. LocalSystem is highly
privileged and chosen to preserve existing authorized process, Defender and power
operations; revisit least privilege in the Security phase.

| Object | SYSTEM / Administrators | Standard Users |
| --- | --- | --- |
| Program directory/files | Full control | Read and execute |
| Runtime directory/state/logs/data | Full control | No granted access |
| Service object | Full control | Query/config-read/status/interrogate only |

ACLs are positive allow entries; there are no broad deny entries. Inheritance is
controlled and directory/file ownership is Administrators. SCM/process identity
provides ordinary protection against unprivileged process termination; this is
not a protected process, driver, Task Manager restriction, or Administrator lockout.
Validate actual effective access and inherited machine policies manually.

The scope is normal local Windows user operations. This does not add Agent
authentication, change Dashboard authorization, or protect against administrators,
offline disk access, kernel exploits, or security products.

## Lifecycle, recovery and retry

SCM transitions StartPending -> Running -> StopPending -> Stopped.
Running is reported after local initialization, before waiting for infrastructure.
StartPending never waits indefinitely for enrollment/API/WS availability.

API and WebSocket use independent equal-jitter backoff windows: 1, 2, 4, 8, 16,
30 seconds, with actual waits in [window/2, window]. The cap includes jitter.
WebSocket backoff resets only after a connection lasting at least 30 seconds;
a successful TCP upgrade immediately rejected by the server does not reset it.
No local listener or per-Agent server port is introduced.

Stop/Shutdown cancels contexts and reconnect waits, closes sockets, joins connection
workers, cancels downloads/scans, and cancels accepted delayed power operations.
Network disconnect alone retains accepted power work as before. HTTP idle
connections are closed. Native providers may block inside OS calls; the SCM host
has a final 20-second shutdown bound, reports a diagnostic, and exits the dedicated
service process if cooperative shutdown cannot complete. That is a fallback,
not the normal stop path. Local initialization also has a 30-second host bound.

Recovery: first unexpected crash restarts after 5 seconds; second after 30 seconds;
subsequent failures take no action. Failure count reset is 24 hours without failures.
Non-crash failures do not trigger recovery. An intentional Stop, invalid identity,
404, or configuration failure therefore does not enter an automatic restart loop.
After repairing such an error, an Administrator must Start the service.

These settings use native SCM recovery semantics:
[Microsoft failure-action flags](https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_failure_actions_flag)
and [Service access rights](https://learn.microsoft.com/en-us/windows/win32/services/service-security-and-access-rights).

## Logs and troubleshooting

Both modes retain file logging. Console additionally mirrors to stderr.
Default retention is the current file plus five backups, 5 MiB each (about 30 MiB).
Rotation occurs on write. Logs record lifecycle, enrollment, connection and retry
events. JSON payloads and remote close reasons are not logged; HTTP enrollment
errors report status codes without response bodies or credentials.

Inspect:

```powershell
Get-Content -LiteralPath "$env:ProgramData\ThesisAgentDev\logs\agent.log" -Tail 80
Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='ThesisAgentDev'} -MaxEvents 10
```

- Running with API retry: check AGENT_API_URL, network, server and database.
- Running with WS retry: check WS_SERVER_URL and whether REST/WS use the same DB.
- A 404 with a marker: server no longer has that ID; use administrative provisioning
  with the existing identity. Do not regenerate it.
- Invalid identity/conflict: retain both files and investigate as Administrator.
- Missing log: startup may have failed before file logging; inspect Application
  events, Service status, protected paths and ACLs.
- Missing screen in Service: expected Session 0 limitation; runtime stays alive.
- Service cannot start after update: check binary path, file ACL and startup error.
- State already in use: stop the other runtime/provisioner; never delete a live lock.
- A third crash stays stopped: expected bounded recovery; inspect and repair.
- Environment differs between provisioning and SCM: review system variables and
  persistent .env; never rely on the administrator shell's temporary environment.

## Screen limitation

Service intentionally supplies no interactive capture callback. Screen start
requests log the Session 0 limitation and do not crash or emit a new wire message.
Console retains CaptureScreenJPEG and binary JPEG streaming. A Session Worker,
desktop/session redesign, and screen availability before login are deferred.

## Manual Windows acceptance checklist

All rows below are **NOT YET VERIFIED MANUALLY** in this implementation session.
The session was not elevated. Unit tests simulating SCM channels are not evidence
of effective Windows SCM/ACL protection.

| Test | Procedure and expected result |
| --- | --- |
| A Console regression | In an isolated console installation, go run .; verify terminal + file logs, existing features, console screen and Ctrl+C. |
| B Install | Elevated Install; verify StartMode Auto, LocalSystem, correct quoted path, Running and durable logs. |
| C Auto Start | Reboot after successful provisioning; do not launch Agent or log in first. Observe server/log timestamps proving startup before login. |
| D Infrastructure unavailable at boot | Stop REST/WS, reboot the provisioned test PC; Service remains Running with retry logs. Restore infrastructure; enrollment/WS recover automatically. |
| E WS restart | Restart WS while connected; Agent stays alive and reconnects with varied bounded backoff. |
| F Identity | Record only agent_id and hashes of the identity file; restart Service and reboot; compare. Do not print private-key fields. |
| G Service ACL | As Standard User attempt Services Stop, Stop-Service, sc.exe stop/config/delete. All mutations must be denied; service stays visible/queryable. |
| H Process ACL | As Standard User attempt Task Manager End task and Stop-Process on the Service PID. Expect Access denied. |
| I NTFS ACL | As Standard User attempt replace/rename/move/delete executable and modify/delete identity/config/enrollment state. Expect Access denied. |
| J Admin control | Elevated Stop/Start/Restart work. After Stop, wait longer than recovery delays and confirm it stays stopped. |
| K Crash recovery | On a disposable development PC, as Administrator find the exact Service PID, force-terminate only that process, and verify restart at configured delays. Third crash remains stopped. |

Do not use the crash test on a production lab PC or kill unrelated processes.
Use sc.exe explicitly in PowerShell (sc can be an alias).

Useful manual commands:

```powershell
sc.exe qc ThesisAgentDev
sc.exe queryex ThesisAgentDev
sc.exe sdshow ThesisAgentDev
sc.exe qfailure ThesisAgentDev
sc.exe qfailureflag ThesisAgentDev

# Standard User rejection tests:
Stop-Service -Name ThesisAgentDev
sc.exe config ThesisAgentDev start= disabled
sc.exe delete ThesisAgentDev
# Use the actual PID observed with queryex, not a guessed value:
# Stop-Process -Id <service-pid> -Force
```

Do not claim Auto Start, reboot, ACL denial or crash recovery VERIFIED until those
checks have actually run under the appropriate accounts.

## Deferred work and existing repository artifacts

No Session Worker, new security/enrollment protocol, TLS/WSS redesign, updater,
final installer, multi-machine provisioning or 10-machine acceptance testing is
included. Agent signature authentication and
least-privilege service-account review are deferred.

Security Phase 1 removes .env, agent_config.json and data/downloads artifacts
from the Git index while preserving their local bytes. History is not rewritten.
Use .env.example for configuration and administrative provisioning for each
machine's unique identity; never copy one machine's identity to another.

The REST enrollment integration test now uses an isolated test schema and the
current agent_id/public_key payload. It requires a disposable PostgreSQL
TEST_DATABASE_URL; it must never run against an operational database.
