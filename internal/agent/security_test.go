package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"ws-agent/internal/apppaths"
	"ws-agent/internal/protectedpath"
)

func TestServiceSecurityBeforeLockConfigLogsAndIdentity(t *testing.T) {
	paths, before := runtimeFixture(t)
	if err := os.WriteFile(paths.Config, []byte("THIS_IS_NOT_A_VALID_ENV_LINE"), 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("security refused fixture")
	called := false
	err := runWithSecurity(context.Background(), Options{Service: true}, paths, nil, func(_ context.Context, p apppaths.Paths, emit protectedpath.Reporter) (func(), error) {
		called = true
		if p != paths {
			t.Fatal("wrong security paths")
		}
		emit(protectedpath.Diagnostic{Reason: "untrusted_ace", Action: "fail_closed"})
		return nil, sentinel
	}, nil)
	if !called || !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(paths.Root, ".runtime.lock"), paths.Log, paths.Enrollment} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("opened runtime state before security: %s", p)
		}
	}
	after, _ := os.ReadFile(paths.Identity)
	if string(after) != string(before) {
		t.Fatal("identity changed")
	}
}

func TestConsoleAndProvisionDoNotSelfHeal(t *testing.T) {
	for _, options := range []Options{{}, {Provision: true}} {
		t.Run(map[bool]string{false: "console", true: "provision"}[options.Provision], func(t *testing.T) {
			paths, before := runtimeFixture(t)
			t.Setenv("AGENT_LOG_PATH", paths.Identity) // stop before networking or provisioning
			called := false
			err := runWithSecurity(context.Background(), options, paths, nil, func(context.Context, apppaths.Paths, protectedpath.Reporter) (func(), error) {
				called = true
				return nil, errors.New("unexpected")
			}, nil)
			if err == nil || called {
				t.Fatalf("self-healing invoked outside Service: %v", err)
			}
			if _, err := os.Stat(filepath.Join(paths.Root, ".runtime.lock")); err != nil {
				t.Fatal("existing runtime locking behavior changed", err)
			}
			after, _ := os.ReadFile(paths.Identity)
			if string(after) != string(before) {
				t.Fatal("identity changed")
			}
		})
	}
}

func TestStartupDiagnosticsFallbackReplayAndFailure(t *testing.T) {
	var events, files []string
	d := &startupDiagnostics{event: func(s string, _ bool) { events = append(events, s) }, file: func(s string) { files = append(files, s) }}
	d.report(protectedpath.Diagnostic{Reason: "repair_success", Action: "verified"})
	if len(events) != 1 || len(files) != 0 {
		t.Fatal("early event not routed to Event Log")
	}
	d.fileReady()
	if len(files) != 1 || files[0] != events[0] {
		t.Fatal("repair result not replayed")
	}
	d.report(protectedpath.Diagnostic{Reason: "acl_canonical", Action: "verified"})
	if len(events) != 1 || len(files) != 2 {
		t.Fatal("file-ready diagnostic incorrectly routed")
	}
	// Event Log failure is best-effort and cannot affect the security decision.
	d = &startupDiagnostics{event: func(string, bool) {}, file: func(string) { t.Fatal("unsafe early file logging") }}
	d.report(protectedpath.Diagnostic{Reason: "untrusted_owner", Action: "fail_closed"})
	if len(d.pending) != 1 || !strings.Contains(d.pending[0], "untrusted_owner") {
		t.Fatal("failure lost")
	}
}

func TestLoggerBoundaryFailureNeverRecursesIntoFileLogger(t *testing.T) {
	var events, files int
	d := &startupDiagnostics{ready: true, event: func(string, bool) { events++ }, file: func(string) { files++; t.Fatal("logger failure recursively used file logger") }}
	d.logBoundaryReport(protectedpath.Diagnostic{Scope: "on_access", Reason: "untrusted_ace", Action: "fail_closed"})
	if events != 1 || files != 0 {
		t.Fatal("unsafe logger diagnostic lost")
	}
}
