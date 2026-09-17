# PHASE 1 AGENT HANDOFF

## A. Architecture implemented

```text
Client.Run → Client.runConnection → readJSONMessages
  → onMessage callback
  → parsePowerCommand
  → go Client.handlePowerCommand(runCtx, requesting safeConnection, command)
  → PowerController.Shutdown(context.Context)
  → powerCommandResult
  → writeJSON → safeConnection.write → writeMu → WebSocket text message
```

`main.go` constructs the client, injects the mock, configures downloads, and
passes the existing providers to `Run`. Registration still precedes the reader,
heartbeat and publishers; the existing three-second reconnect loop is unchanged.

The incoming callback retains virus-scan handling, checks Power, then retains
download handling. The reader also invokes the existing stream parser, so
`parseStreamCommand` now explicitly excludes the normalized `power` type before
checking legacy aliases. This prevents a Power message containing, for example,
`action: "kill_process"` or `command: "start_screen"` from triggering another
feature. Power has its own command/result types and is not a stream kind.

## B. Changed files

| File | Change and reason |
| --- | --- |
| `main.go` | Calls `ConfigurePower(service.MockPowerController{})` at startup. |
| `client/client.go` | Stores the injected `PowerController`; dispatches validated Power commands using the requesting connection and its context. |
| `client/message.go` | Excludes Power from stream alias matching to prevent cross-feature dispatch. No stream fields or kinds were added. |
| `client/power.go` (new) | Defines the controller interface, startup configuration method, Power parser, command/result types and handler. |
| `client/power_test.go` (new) | Tests parsing, exact result fields, safe errors, controller calls, mutex serialization, cancellation, connection affinity, and routing alongside existing features. |
| `service/power.go` (new) | Implements the mock, which only checks cancellation, logs, and returns. |
| `service/power_test.go` (new) | Tests mock success and cancellation. |
| `docs/power-shutdown.md` (new) | This handoff for the next repository and Phase 2. |
| `readme.md` | Links to this handoff. |

The pre-existing local `.env` modification was not changed by this task.

## C. Exact protocol implemented

Server → Agent:

```json
{
  "type": "power",
  "action": "shutdown",
  "request_id": "2FC4DEE1-73B3-425B-9D36-E7B167CC8B74"
}
```

Agent → Server on success:

```json
{
  "type": "power",
  "action": "shutdown_result",
  "request_id": "2FC4DEE1-73B3-425B-9D36-E7B167CC8B74",
  "success": true,
  "mode": "mock",
  "message": "shutdown command accepted"
}
```

Agent → Server on controller error:

```json
{
  "type": "power",
  "action": "shutdown_result",
  "request_id": "2FC4DEE1-73B3-425B-9D36-E7B167CC8B74",
  "success": false,
  "mode": "mock",
  "message": "shutdown command failed"
}
```

If no controller was injected, the failure message is
`power controller unavailable`. `success` is always a JSON boolean, including
`false`; no `omitempty` hides it. The result contains exactly these six fields,
with no `agent_id`. There are no changes to the supplied wire contract.

## D. Validation behavior

- `type` and `action` values must be exactly `power` and `shutdown`. They are not
  trimmed or case-normalized by the Power parser.
- `request_id` must be a string of 36 bytes validated by the existing
  `github.com/google/uuid` dependency. It accepts hyphenated hexadecimal UUIDs
  in either case; it adds no version/variant restriction. Whitespace, braces,
  URNs, compact UUIDs, missing/null/numeric IDs and invalid hexadecimal fail.
  The original string is returned without regeneration or canonicalization.
- Malformed JSON is ignored by the reader and also rejected by the parser.
  Valid JSON with invalid field types or unsupported actions is ignored.
  These messages neither invoke the controller nor produce a Power result;
  the connection continues reading subsequent messages.
- `restart`, `sleep`, `lock`, `shutdown_room` and unknown actions have no handler.
  There is no room-specific logic in the agent.
- Extra JSON fields are ignored by the Power parser, using standard Go JSON
  decoding. Stream-like extra fields cannot activate a stream for type Power.
- Outbound messages use fixed short ASCII text, all below 4,096 bytes. Controller
  error strings are logged locally, never copied into the protocol response.

## E. PowerController implementation

The consuming package defines `client.PowerController`:

```go
type PowerController interface {
    Shutdown(context.Context) error
}
```

`service.MockPowerController` implements it. An active request logs
`[MOCK POWER] shutdown requested` and returns `nil`. A cancelled context returns
its cancellation error. The handler skips work if its context is already
cancelled before invocation.

`Client.ConfigurePower` injects the dependency before `Run`, following the
existing startup configuration pattern of `ConfigureDownloads`. It must not be
called concurrently with `Run`. A missing controller produces a safe failure.

Phase 1 has **no real OS shutdown implementation**, executable invocation,
PowerShell shutdown, Windows shutdown API, reboot, sleep or logoff path.
Tests use fakes/the mock and never perform OS power actions.

For Phase 2, add `service/power_windows.go` implementing
`WindowsPowerController.Shutdown(context.Context)` and its tests, then change the
injection in `main.go`. If non-Windows builds remain supported, provide a
build-tagged unsupported-platform implementation in `service/power_other.go`
or equivalent platform wiring. None of those Phase 2 files exist in this change.
The interface, parser, request/result flow, server and dashboard need no changes
to swap controllers under the current contract. The current result mode remains
`mock`; changing its meaning/value requires an explicitly coordinated contract
decision, not an unannounced agent change.

## F. Concurrency / connection behavior

Each accepted command runs in a goroutine so a controller does not block the
WebSocket reader. It receives `runCtx`, cancelled when that connection ends,
and the exact `safeConnection` on which the request arrived.

Results use the existing `writeJSON` and `safeConnection.writeMu`, shared by
JSON publishers, process results, download/scan events and screen frames.
Heartbeat retains its existing WebSocket `Ping` path. There is no new raw write
or synchronization mechanism.

A write failure is logged. Power results do not use `sendDownloadEvent`, are not
added to `pendingResults`, and are not replayed on a replacement connection.
There is no durable request state, deduplication, room aggregation or agent-side
ten-second request timer. Concurrent requests can complete out of order and are
correlated using `request_id`.

## G. Test results

Baseline before changes:

```text
go test ./...
PASS
```

After implementation:

```text
gofmt -w main.go client/client.go client/message.go client/power.go client/power_test.go service/power.go service/power_test.go
PASS

go test ./...
PASS (all packages)

go vet ./...
PASS

go test -race ./...
NOT RUN: go: -race requires cgo; enable cgo by setting CGO_ENABLED=1
```

The environment reports `CGO_ENABLED=0` and `gcc` is not on PATH. Thus race
detection is unverified; the normal concurrency tests pass. The first sandboxed
`go vet` attempt could not access the Go build cache; the approved rerun passed.
There were no baseline test failures.

Tests cover valid/invalid parsing, preserving UUID case, unsupported actions,
safe controller failures (including a long error containing internal details),
missing controllers, exact six-field response/boolean serialization, one call
per command, repeated IDs, cancelled context, write-mutex use and failed-result
connection affinity. A local WebSocket integration test exercises registration,
Power, performance/process publishers, fake process kill, fake screen frames,
fake antivirus completion and download rejection together. Existing download
and antivirus suites also pass. No live Step 1 server or dashboard was contacted.

## H. Compatibility with WebSocket Server

**Compatible with the supplied Step 1 contract**:

- Receives `{type:"power", action:"shutdown", request_id:"<hyphenated UUID>"}`.
- Sends `{type:"power", action:"shutdown_result", request_id:"<same UUID>",
  success:<boolean>, mode:"mock", message:"<bounded safe text>"}`.
- Sends no `agent_id`; connection identity remains the server's source of truth.
- Preserves `request_id` exactly, including letter case.
- Always sends `mode: "mock"` on success and failure.

Compatibility is based on the provided contract and local wire tests, not an
end-to-end run against `thesis-web-socket`.

## I. Remaining risks / assumptions

- The dashboard should treat success as mock acceptance, not evidence that a
  computer has powered off. This agent remains connected and running.
- The server generates hyphenated UUIDs and owns authorization, pending
  requests, duplicate handling and timeout behavior. Repeating a valid command
  calls the controller again; the agent does not implement idempotency.
- Invalid requests are ignored without a rejection result. A malformed server
  request will therefore rely on server-side timeout/error handling.
- Disconnects can lose results. Nothing retries a Power result on another
  connection; the server must resolve its pending request appropriately.
- Controllers should honor cancellation and be safe for concurrent calls.
  Cancellation can race with a controller already accepting work; it cannot
  undo an accepted operation. Phase 2 must account for this and for giving the
  result a chance to leave before a real shutdown closes the connection.
- ConfigurePower is startup-only, not a runtime controller-switching API.
- Race-detector coverage still requires a suitable cgo toolchain.

## J. Deviations and design decisions

There is no wire-protocol deviation and no Phase 2 implementation.

`powerCommand` contains only `RequestID`: successful parsing already means
shutdown, so a redundant shutdown boolean is unnecessary. The interface lives
in `client`, its consumer, while the mock lives in `service`, matching the
existing separation between WebSocket handling and system services.

The reader's callback API was retained. Because that reader independently
checks stream aliases after the callback, a narrow guard was added to
`parseStreamCommand` to isolate Power. Other command parsing and feature
protocols were not refactored.

Invalid requests are silently ignored, and Power uses a connection-bound
handler rather than the download/antivirus reconnect queue. These choices keep
correlation on the requesting connection and leave request lifecycle policy to
the server. They require no new dashboard request or response fields.
