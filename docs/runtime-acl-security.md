# Phase 5B.1: runtime ACL inspection, repair and diagnostics

Baseline: `81488afd80f17d6aca6ad5dcdff5233020796daf` (`capture helper`).
Only the Agent repository is changed. Lab/LocalSystem acceptance is **NOT VERIFIED**.

## Scope and policy

Unattended repair runs only for `Options.Service`. Console and provisioning retain
their previous locking/installer behavior. `ValidateFile` / `ValidateDirectory`
remain read-only; the private-key loader, DPAPI implementation, identity generation,
enrollment and transport protocols are unchanged.

The production security entry point accepts only the complete paths returned by
`apppaths.Machine()` using Windows Known Folders. Its fixed startup scope is:

- Runtime root.
- Existing `agent_config.json`, `enrollment_state.json`, `.env`, `.runtime.lock`.
- Existing `agent_config.json.dpapi-user-v1.bak`, resolved by `Paths.IdentityBackup()`
  using the suffix shared with private-key migration.
- Directory objects `logs`, `data`, and `data/downloads`.

There is no directory enumeration in the startup planner or Windows adapter.
Historical downloads, `.part` files, rotated logs, `.state-*.tmp` files and arbitrary
nested descendants are not startup targets. Missing containers retain lazy creation:
existing parents are checked at startup, then the exact directory is created and
validated under a trusted parent when used.
Nothing in Program Files or an ancestor is repaired. A missing runtime root is
refused; missing identity/config/enrollment files are not created by repair.

Trusted owners/principals are SYSTEM and BUILTIN\Administrators, identified by SID.
Canonical ACLs contain exactly their two Full Control allow ACEs. Directories use
OI/CI inheritance, without inherit-only/no-propagate flags. The root DACL must be
protected and explicit. Children can have equivalent inherited trusted ACEs or
protected explicit ACEs, matching the installer and normal file creation. Repair
writes protected explicit DACLs and preserves the existing trusted owner and SACL.

| Classification | Examples | Action |
| --- | --- | --- |
| Canonical | Trusted owner, exact SYSTEM/Admin full control and correct flags | Validate; no DACL write |
| Repairable | Missing/restricted SYSTEM or Admin ACE; wrong inheritance flags; unprotected root containing only trusted ACEs; duplicate trusted ACEs | Acquire runtime lock, re-inspect, repair DACL, validate again |
| Unsafe | Untrusted owner or any foreign allow ACE; absent/null/unreadable DACL; unknown/deny/callback/object ACEs | Refuse, preserve contents and ACLs, require manual security review/recovery |
| Unsafe object/path | Reparse/junction/symlink at target or ancestor; unexpected type; path mismatch/escape; multiple hard links | Refuse without following or repairing it |

Foreign ACEs are conservatively refused even if inherit-only or zero-mask. The
implementation does not attempt effective-access analysis of arbitrary ACLs.
An empty non-null DACL with trusted owner is classifiable as repairable, but repair
still requires sufficient OS access to inspect the selected objects and lock runtime.
No take-ownership or backup/restore privilege is enabled to bypass unreadability.

Unsafe means the expected boundary cannot be established; it does not prove an
intrusion. Removing an unsafe ACE cannot undo potential past key disclosure.
Neither Service startup nor administrator Repair has a force/override option.
Do not use reinstallation to conceal an unsafe-state diagnosis.

## Startup order and concurrency

1. Resolve and compare all expected machine paths.
2. Pin existing ancestors from the volume root downward, checking type, reparse
   attributes and resolved handle path. Ancestors are never mutated.
3. Open runtime objects with `OPEN_REPARSE_POINT`, `BACKUP_SEMANTICS`,
   `MAXIMUM_ALLOWED`, and no DELETE sharing. Inspect metadata through those handles;
   open only the fixed critical/container paths. Preflight all selected objects
   before any ACL write so parent repair cannot hide an unsafe critical child.
   No historical child is opened or listed.
4. Acquire the existing `.runtime.lock` byte lock using the pinned lock handle.
   If absent, create only this coordination file using `CREATE_NEW` and an explicit
   protected SYSTEM/Admin ACL. Never truncate or recreate an existing lock file.
   A concurrent runtime/provisioner, sharing conflict or inaccessible lock causes
   refusal before ACL repair.
5. Repeat preflight under the lock. Immediately before each mutation inspect the
   parent and target; repair through the same handle; inspect the result. Finally
   validate every pinned object again. Any repair/post-validation error aborts.
6. Release inspection handles, retain the runtime byte lock and ancestor pins.
   Load `.env`, initialize logging and replay diagnostics, then execute existing
   identity/enrollment/private-key/network/capture flow. Release the lock on exit.

`SetSecurityInfo` with `MAXIMUM_ALLOWED` does not propagate ACEs to existing children;
historical children are not implicitly rewritten by container repair. This is
covered by a real Windows temporary-file test and follows the
[Microsoft API contract](https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-setsecurityinfo).
No content handle is read/written during repair; only security metadata and the
coordination lock are used. Inherited access for *future* files remains SYSTEM/Admin.

Startup work is independent of descendant count: the planner visits nine paths,
checking cancellation between them. The recursive count/depth limits and
`inspection_limit_exceeded` startup reason have been removed. Service startup and
administrative Repair use exactly the same bounded scope.

## Dynamic objects at their use boundary

We intentionally do not prove every historical download/log child secure before
startup. Critical secret-bearing state remains strict; container ACLs prevent
standard users from ordinarily creating/changing children. A dynamic object is
validated when a feature actually uses it, without repairing its ACL automatically.

`internal/protectedpath/access_windows.go` implements the Service-only `Boundary`.
It checks the requested path against runtime root, pins ancestors/exact parent
directories, rejects reparses, aliases/ADS/path escape, and requires canonical
trusted owner/DACL. Console and provisioning retain their existing I/O behavior.

- **Logging:** `InitProtectedLoggingAt` validates/creates the exact log directory.
  The active log opens with `OPEN_REPARSE_POINT`, no DELETE sharing and append access;
  the same handle is inspected before content writes. Opening it does not inspect
  rotated logs. Only when rotation is needed, validate the active path and fixed
  retention slots (normally `.1` through `.5`) before any rename/delete. Size,
  retention and rotation order are unchanged. Other historical logs are ignored.
- **Downloads:** validate/create the configured directory when configuring the
  manager. Each job checks its exact destination and parent before HTTP work.
  Exclusively create a new random `.part` in a pinned trusted parent and validate
  its handle before writing. Historical `.part` files are never resumed or scanned.
  Checksum reopening validates the exact handle. Publish checks source/destination
  immediately before the existing atomic rename/replace; cleanup checks only the
  job's `.part`. Network protocol, URL rules, size and SHA-256 checks are unchanged.
- **State:** critical state and the known key backup remain in the startup gate.
  `internal/statefile` still creates exclusive random temporary files under the
  protected root and publishes atomically, without opening historical `.state-*.tmp`
  files. Private-key loading/migration semantics are unchanged; migration and
  startup now share the authoritative backup suffix.

An unsafe historical file can remain unnoticed until used. Its presence alone does
not block startup, and startup success does not establish that it is safe. This is
the intentional security/availability tradeoff. No deep-audit command is added.

### Remaining TOCTOU limitations

No DELETE sharing prevents ordinary rename/replacement of objects while inspected.
Open-reparse handling, ancestor pins and final-path comparisons prevent ordinary
path redirection from turning repair into a write outside the runtime tree.
Hard-linked files are refused because another path would share their descriptor.

This is not an atomic filesystem transaction or protection against a hostile
Administrator/SYSTEM/kernel actor. A privileged process can change descriptors,
content or directory entries, including adding children, without honoring the
runtime lock. Handles do not freeze security descriptors or revoke previously
granted handles. Inspection handles are released before normal runtime file I/O
to preserve atomic state replacement/log rotation. Normal loaders continue using
their existing validation. Dynamic open/append/checksum use the same handle as
validation. Rename/delete checks allow DELETE sharing to permit the operation,
while checked containers remain pinned; this is not atomic check-and-rename against
a privileged concurrent writer. Prior exposure that leaves no observable evidence is
not detectable. Partially completed safe repairs are not rolled back after a
later failure; startup remains stopped, contents remain untouched, and no
potentially less secure ACL is restored.

## Diagnostics and logging

Diagnostics contain `scope=critical`, `scope=container` or `scope=on_access`,
quoted object paths, classification, reason, action, owner
SID and (for a foreign ACE) principal SID/access mask. They contain no file contents,
public/private key material, ciphertext or enrollment tokens. Reason codes include:

| Codes | Meaning |
| --- | --- |
| `acl_canonical` | Trusted ACL verified |
| `inheritance_drift`, `missing_system_full_control`, `missing_admin_full_control`, `noncanonical_trusted_aces` | Safely repairable metadata drift |
| `untrusted_owner`, `untrusted_ace` | Foreign ownership/access; manual review required |
| `reparse_point_detected`, `unexpected_object_type`, `path_escape`, `hard_link_detected` | Unsafe object/path |
| `security_descriptor_unreadable` | Cannot open/inspect, null/absent DACL or unsupported/ambiguous descriptor |
| `repair_started`, `repair_success`, `repair_failed`, `post_repair_validation_failed` | Mutation and verification results |
| `runtime_lock_failed`, `parent_validation_failed`, `path_resolution_failed` | Startup safety prerequisite failed |
| `unsafe_acl_fail_closed`, `startup_failed` | Startup refused |

Before the file logger is ready, diagnostics go to the existing `ThesisAgentDev`
Windows Application Event source (error event 1, informational event 2). Successful
startup replays buffered diagnostics, including repair results, into the configured
`agent.log`. Later download diagnostics go to that file. Logger I/O rejection uses
Event Log directly once initialized, avoiding recursive writes through the same
logger mutex or rejected path. Unsafe startup does not attempt to
write inside the rejected tree. The host also retains its existing startup failure
event. Event Log failures are best-effort and never convert a rejection to success.
If both file logging and Event Log are unavailable, the process still fails closed;
there may be no durable detailed event. No Event source is installed at startup.

## Administrator Repair

Use elevated **Windows PowerShell 5.1** on the Lab PC, with a reviewed Phase 5B.1
binary. Resolve names and paths using `--service-info`. After explicitly stopping
the owned Service and waiting for Stopped:

```powershell
.\scripts\dev-service.ps1 -Action Repair -Executable .\build\thesis-agent.exe
```

Repair verifies expected Program Files/ProgramData metadata, rejects reparse paths,
and validates an installed, stopped, dedicated LocalSystem Service with the exact
expected executable/arguments. The Go `--repair-runtime-acl` maintenance mode repeats
the ownership/state check and invokes the same nine-path engine as startup. It never
repairs historical downloads, rotated logs or arbitrary runtime descendants. It accepts no
target-path argument or force flag and cannot combine with other modes.

The lifecycle is **Option B**: refuse while running; no automatic stop/start/restart.
The runtime lock also excludes a cooperating provisioner/runtime that starts after
the SCM query. This avoids stopping unrelated processes and makes maintenance
explicit. The Service remains stopped after both success and failure.

Output goes to the administrative console and Application Event Log. Success is
also visible as canonical security diagnostics on the next normal Service startup.
Repair never provisions, prompts for a token, registers/re-enrolls, migrates DPAPI,
copies a binary/config, creates an identity or invokes a network endpoint. It rejects
`ConfigPath`, `IdentityPath` and `NewIdentity`. Failure returns nonzero and requests
manual security review/recovery; unsafe findings never trigger an ACL reset.

`-Action Install` and all BAT files retain their existing semantics, including
`update-agent.bat` using prebuilt Agent and Desktop Helper binaries. This phase
does not add legacy-key migration to `install-existing-agent.bat`.

Desktop Capture remains Service → Named Pipe → interactive Desktop Helper → JPEG
→ Service-owned WebSocket. The Helper receives no server credentials. See
[Desktop Capture Helper](desktop-capture-helper.md).

## Verification evidence and limits

Tests cover descriptor classification, unknown/unsafe ACE rejection, read-only
inspection, repair failures/post-validation failures/idempotency, actual Windows
DACL writes with content hashes and propagation checks, pinned object replacement,
hard links, symlinks, foreign root refusal, machine-path rejection, startup ordering,
Console/provisioning isolation, logging routing and Service ownership matching.

`TestLargeDownloadsDirectoryDoesNotPreventStartupSecurity` uses virtual metadata
with 0, 20,000 and 50,000 download children, unsafe rotated logs and nested unknown
objects. It invokes the production fixed planner with a metadata visitor and asserts
the same nine paths are checked once, without descendant or active-log access.
This proves no traversal by the planner; it is not a 50,000-file disk benchmark or
LocalSystem deployment test. Critical/container unsafe entries still fail preflight.
Native Windows append handles and reparse/escape boundary refusals are also tested.

Temporary caller-owned Windows fixtures use retained handles to restore their
original DACL during cleanup. Trusted owner substitution for descriptor testing is
in memory only; the production inspector still rejects those caller-owned fixtures.
No production safety policy is loosened to make tests pass.

The elevated fixed-scope/dynamic-I/O fixtures and canonical/repaired `service.LoadPrivateKey`
integration test are present but **SKIPPED / NOT VERIFIED** with this session's
unelevated token. They use only generated temporary fixture identities and DPAPI;
they do not read/write the actual ProgramData installation. Existing DPAPI/key
validation tests continue to run independently.

Race detector is **NOT VERIFIED**: this environment has `CGO_ENABLED=0` and no
discovered GCC/Clang toolchain. Actual SCM startup, LocalSystem access, effective
Standard User denial, Event Viewer delivery/fallback, restart/reboot, live
installer/update and Desktop JPEG delivery are **NOT VERIFIED**. Use the
[Lab acceptance procedure](windows-service-acceptance.md#phase-5b1-acl-acceptance-on-one-lab-pc).
