# Windows Service implementation and verification report

Date: 2026-09-21. This supersedes the unfinished verification status in
[implementation-handoff.md](implementation-handoff.md).

## 1. Current status

Implementation review, remaining fixes, formatting, and the available automated
checks are complete against the current working tree. **Real Windows Service
acceptance remains pending; this is not a production verification claim.**

No Service was installed. The terminal is not elevated, and the machine's
PowerShell execution policy blocked the development .ps1 invocation. Race tests
remain unverified because no compatible C compiler was found.

All changes remain uncommitted in thesis-agent. No commit/push, identity
replacement, execution-policy change, reboot, account switch or real shutdown
was performed. Other repository working trees are clean.

## 2. Completion since the handoff

- Formatted only Go files reported by gofmt: shared runtime, native Service
  configuration and Windows power controller.
- Fixed the stale script name in the runtime error and the enrollment guide's
  outdated detach-console description.
- Moved CLI positional-argument validation ahead of the privileged configure
  branch, so malformed invocations fail before attempting configuration.
- Fixed cancellation ordering: Client.Run now initiates Client.Close immediately
  on runtime cancellation, even while a connection provider is still unwinding.
  This cancels pending power timers/manager I/O promptly. Explicit Close also
  cancels Run, preventing it from reconnecting after shutdown.
- Added a regression test with a deliberately blocked provider. Both parent
  cancellation and explicit Close must close the mock power controller before
  the provider is released, then let Run exit.
- Reviewed script scoping, elevation checks, owner/reparse guards, ACL
  inheritance, Service executable quoting, recovery settings and retained-state
  removal. No additional script change was needed from static review.

## 3. Final architecture

main.go selects attached console, administrative provisioning, metadata/configure
tooling or native SCM hosting. Both console and SCM call internal/agent.Run.
SCM owns StartPending/Running/StopPending transitions; shared runtime owns
configuration, logging, identity/enrollment checks and the existing Client/providers.

Local initialization completes before SCM Running. Network/API/WS readiness
is reported separately. Service never reads an enrollment token from stdin:
administrative provisioning reuses the existing REST enrollment flow first.
Unknown enrollment history does not mean missing identity; server 204 verifies,
404 requires provisioning, and temporary API failures retry unattended.

Persistent paths come from Windows Known Folder APIs. Identity publication is
exclusive; migration preserves source bytes without replacing a conflicting
destination. DPAPI code, algorithm and ciphertext format are unchanged.
The machine runtime directory has an OS-owned lock. Existing user-bound
ciphertext is not decrypted by the current runtime.

Connection retry uses exponential equal jitter with a 30-second ceiling, reset
after a connection lasts at least 30 seconds after initial-message delivery.
Context cancellation interrupts retry. Connection workers, downloads, scanning
and pending power timers participate in cleanup. Local startup is bounded at
30 seconds and SCM shutdown at 20 seconds, with diagnostics for timeout paths.

Console stays attached and retains screen capture, terminal and file logging.
Service mode uses file logging and no interactive screen callback. File logs
rotate at 5 MiB with five backups. No Session Worker was implemented.

See [windows-service.md](windows-service.md) for setup, lifecycle and limitations.

## 4. Commands and actual results

Environment: go version go1.26.5 windows/amd64; GOOS=windows, GOARCH=amd64,
CGO_ENABLED=0, CC=gcc. Get-Command found neither gcc nor clang.

| Command/check | Result against current source |
| --- | --- |
| gofmt inspection of all modified/untracked Go files; gofmt -w only reported files and later edited main/client files | PASS; final gofmt -l produced no paths |
| go test ./... | PASS, exit 0; changed client tests 2.587s and runtime tests 1.699s; other unchanged packages may use Go's test cache |
| go vet ./... | PASS, exit 0 |
| go build -o build/thesis-agent-dev.exe . | PASS, exit 0; native Windows amd64 build |
| git diff --check | PASS, exit 0; Git reported LF/CRLF conversion warnings, no whitespace errors |
| PowerShell Parser.ParseFile for scripts/dev-service.ps1 | PASS, exit 0; zero syntax errors |
| Service and runtime-directory SDDL parsing | PASS; syntax/mask inspection only, not an effective-access test |
| .\build\thesis-agent-dev.exe --service-info | PASS, exit 0; metadata matched expected Known Folder paths |
| Absolute invocation of the same binary with CWD changed to the temp directory | PASS; identical metadata |
| Binary --service from this console | PASS rejection check; expected exit 1 with SCM-only diagnostic |
| Binary --provision from this non-elevated session | PASS rejection check; expected exit 1 before runtime/provisioning |
| Binary --configure-service unexpected | PASS rejection check; expected exit 1 for positional arguments |
| .\scripts\dev-service.ps1 -Action Status | BLOCKED/failed invocation: execution policy rejected the file before the script ran; not evidence of the script's administrator guard |
| sc.exe query ThesisAgentDev | Exit 1060: Service is not installed; read-only query only |
| go build ./... with process-scoped GOOS=linux, then restore GOOS | PASS, exit 0; cross-platform compile only |
| go test -race ./... | NOT VERIFIED — required Windows C compiler unavailable; not attempted again this turn |

The combined metadata/guard command ended with exit 1 only because its final
script invocation was blocked. Its preceding metadata comparisons and binary
rejection assertions completed successfully. No check is reported as a successful
Service install or effective ACL test.

Reproduction of the parser check:

    $tokens = $null
    $parseErrors = $null
    [void][System.Management.Automation.Language.Parser]::ParseFile(
        (Join-Path (Get-Location) 'scripts\dev-service.ps1'),
        [ref]$tokens, [ref]$parseErrors)
    if ($parseErrors.Count -gt 0) { $parseErrors | Format-List; exit 1 }

The rebuilt development binary SHA-256 is:

    20A7060150764FCAF119B5F71F9B7E540C5BDC31AE680A119BBBED90E8BCEBDB

It is an ignored build artifact, not a committed release.

## 5. Verified binary metadata

| Field | Actual value on this computer |
| --- | --- |
| Service name | ThesisAgentDev |
| Display name | Thesis Agent (Development) |
| Program directory | C:\Program Files\ThesisAgentDev |
| Executable destination | C:\Program Files\ThesisAgentDev\thesis-agent.exe |
| Runtime root | C:\ProgramData\ThesisAgentDev |
| Configuration | C:\ProgramData\ThesisAgentDev\.env |
| Identity | C:\ProgramData\ThesisAgentDev\agent_config.json |
| Enrollment state | C:\ProgramData\ThesisAgentDev\enrollment_state.json |
| Log | C:\ProgramData\ThesisAgentDev\logs\agent.log |
| Downloads | C:\ProgramData\ThesisAgentDev\data\downloads |

These came from the newly built binary and were compared between repository
and temp working directories. No production machine paths were created by this
metadata check.

## 6. Service verification boundary

Actually executed: simulated SCM Stop/Shutdown/Interrogate tests, shared-runtime
API-outage readiness/cancellation tests, local REST/WS protocol tests, lifecycle
worker tests, identity/marker/logging/retry/locking tests, CLI guard checks,
metadata checks, script parser/SDDL inspection and a read-only Service query.

| Feature | Implementation status | Actual Windows acceptance |
| --- | --- | --- |
| Auto Start / before login | Automatic LocalSystem configuration present | NOT YET VERIFIED MANUALLY |
| Service Start/Stop/Restart | Native host and admin tooling present | NOT YET VERIFIED MANUALLY |
| Standard User stop/disable/delete prevention | Service DACL present and parsed | NOT YET VERIFIED MANUALLY |
| Standard User process/file tamper prevention | LocalSystem and NTFS ACL provisioning present | NOT YET VERIFIED MANUALLY |
| Administrator control | Retained by Service/NTFS ACL and management script | NOT YET VERIFIED MANUALLY |
| Agent ID stability | Byte preservation and no-regeneration unit tests pass | Migration/restart/reboot NOT YET VERIFIED MANUALLY |
| Crash recovery | 5s, 30s, then no action; non-crash recovery disabled | NOT YET VERIFIED MANUALLY |
| API outage at boot / WS server restart | Retry/cancellation tests pass locally | NOT YET VERIFIED MANUALLY |
| Console feature regression / Service screen limitation | Existing tests and local Service screen test pass | Real desktop/server workflow NOT YET VERIFIED MANUALLY |
| Multiple physical agents on one server port | Existing keyed connection model retained | NOT YET VERIFIED MANUALLY |
| Development removal | Scoped stop/unregister; retained data | NOT YET VERIFIED MANUALLY |

Service SDDL parses to SYSTEM/Administrators GenericAll and Builtin Users
0x2008D (query/interrogate/enumerate/read-control). It grants Users no Start,
Stop, change-config, delete, write-owner or write-DACL rights. This matches the
intended access model; only real account tests can establish effective behavior
([Microsoft Service rights](https://learn.microsoft.com/en-us/windows/win32/services/service-security-and-access-rights)).

## 7. Compatibility audit of current local HEADs

No protocol change is required by this implementation. The audit used these
local checked-out revisions; no remote refresh was performed during this turn.

| Repository | HEAD |
| --- | --- |
| thesis-agent baseline | c3d2b17da580b0d652bed2ecef5f627944c68cc6 plus working tree |
| thesis-web-socket | 6abe7c3f4a6534bdc13f25ea2a5d5478aeac3836 |
| thesis-rat-server | 85358b5279701d16b6786e5b86b9385b6c163651 |
| thesis-rat-dashboard | 5013e6f709611d0a78aa55102618ba65bc0fe4ea |

- REST: thesis-rat-server/main.go registers POST /api/agents/register and
  GET /api/agents/:id/exists before session middleware. service/agents.go:
  AgentExists returns 204 or 404; service/agent_registration.go: RegisterAgent
  accepts the existing token/agent_id/public_key/host/MAC/OS payload and returns
  201 with id. Agent service/registration.go reuses these endpoints/fields.
- WS initial message: Agent model.SystemInfo remains the same five JSON fields.
  Server internal/wsserver/server.go: HandleWebSocket reads model.AgentInfo,
  looks up registration.ID in the database, and registers the connection.
- JSON types/actions: existing parsing/models for performance, process, power,
  downloads and antivirus remain compatible. The changes manage lifetimes/logs;
  they do not introduce an envelope, signature exchange or new message types.
- Screen: Agent client/screen.go still writes binary JPEG in console mode.
  Server readMessages and subscriptions.go: WriteScreen retain the existing
  frontend JSON header plus binary frame. Dashboard ScreensClient.tsx consumes
  that same header and ArrayBuffer. Service emits no new availability message;
  its screen tile may remain Waiting, as the accepted scope permits.
- Ports/multiple clients: Agent default remains ws://localhost:8081/ws; server
  main.go maps /ws and /ws/frontend to the existing handlers on its shared
  listener. Registry is keyed by Agent ID, so distinct machines need distinct
  identities, not different server ports. Duplicate IDs replace an old session.
- Dashboard: AgentDetailDashboard.tsx still consumes existing process/performance/
  agent_status messages; lib/power-control.ts and lib/virus-scan.ts expectations
  are unchanged. Agent has no new direct Dashboard integration.

This is a source compatibility audit plus local Agent tests, not a live four-
repository/database integration run. The REST token integration fixture still
references ../schema.sql; it was not modified or executed.

## 8. Files added

    internal/agent/runtime.go
    internal/agent/runtime_test.go
    internal/apppaths/paths.go
    internal/apppaths/paths_windows.go
    internal/apppaths/paths_other.go
    internal/apppaths/paths_test.go
    internal/retry/backoff.go
    internal/retry/backoff_test.go
    internal/runlock/lock_windows.go
    internal/runlock/lock_unix.go
    internal/runlock/lock_other.go
    internal/runlock/lock_test.go
    internal/statefile/file.go
    internal/statefile/replace_windows.go
    internal/statefile/replace_other.go
    internal/servicehost/host.go
    internal/servicehost/host_windows.go
    internal/servicehost/host_other.go
    internal/servicehost/configure_windows.go
    internal/servicehost/host_windows_test.go
    service/enrollment_state.go
    service/enrollment_state_test.go
    service/identity_lifecycle_test.go
    service/identity_windows_test.go
    service/logging_test.go
    service/power_lifecycle_windows_test.go
    client/lifecycle_test.go
    download/lifecycle_test.go
    antivirus/lifecycle_test.go
    scripts/dev-service.ps1
    docs/implementation-handoff.md
    docs/windows-service.md
    docs/windows-service-acceptance.md
    docs/windows-service-verification.md

## 9. Tracked files modified

    .gitignore
    main.go
    readme.md
    config/config.go
    docs/agent-enrollment.md
    client/client.go
    client/heartbeat.go
    client/message.go
    client/screen.go
    service/agent_id.go
    service/registration.go
    service/logging.go
    service/power_windows.go
    service/power_other.go
    download/manager.go
    download/manager_test.go
    antivirus/scan.go
    antivirus/defender_windows.go

Tracked .env, agent_config.json, DPAPI implementation files, go.mod/go.sum and
existing download artifacts were not changed or removed. Existing ignored/
tracked secret cleanup and repository-history rewriting are out of scope.

## 10. Known limits and exact remaining steps

Use [windows-service-acceptance.md](windows-service-acceptance.md) for the exact
commands/account context, expected results, fingerprint checks, file-access
probes, reboot observations, outage recovery and controlled-crash procedures.

The next operator needs an elevated Windows PowerShell 5.1 session with permission
to run the reviewed script, the correct existing identity/config, and test REST/
WS infrastructure. Complete install/admin control, then Standard User ACL tests,
then scheduled reboot/start-before-login, identity stability, infrastructure
outage and controlled crash checks. Record actual results separately.

Race testing needs a compatible Windows C compiler; none was installed here.
Screen in Session 0, cross-account DPAPI decryption, key-based authentication,
TLS/security protocol redesign, final installer/updater and classroom-scale
acceptance remain outside this implementation. LocalSystem has broad privileges.
Stopping can cancel a pending power timer; it cannot undo an OS shutdown that
has already been accepted by Windows.

No new blocker requiring protocol changes, another repository's production
code, identity replacement, DPAPI redesign or Session Worker was found.
