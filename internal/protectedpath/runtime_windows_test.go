//go:build windows

package protectedpath

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"ws-agent/internal/apppaths"
	"ws-agent/internal/runlock"

	"golang.org/x/sys/windows"
)

func descriptor(t *testing.T, s string) *windows.SECURITY_DESCRIPTOR {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return sd
}

func TestRuntimeClassification(t *testing.T) {
	for _, tc := range []struct {
		name, sddl    string
		dir, root     bool
		class, reason string
	}{
		{"root", "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", true, true, canonical, "acl_canonical"},
		{"file", "O:SYG:SYD:P(A;;FA;;;SY)(A;;FA;;;BA)", false, false, canonical, "acl_canonical"},
		{"inherited-child", "O:SYG:SYD:AI(A;ID;FA;;;SY)(A;ID;FA;;;BA)", false, false, canonical, "acl_canonical"},
		{"missing-system", "O:BAG:BAD:P(A;OICI;FA;;;BA)", true, true, repairable, "missing_system_full_control"},
		{"missing-admin", "O:SYG:SYD:P(A;OICI;FA;;;SY)", true, true, repairable, "missing_admin_full_control"},
		{"system-flags", "O:BAG:BAD:P(A;;FA;;;SY)(A;OICI;FA;;;BA)", true, true, repairable, "inheritance_drift"},
		{"admin-flags", "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OI;FA;;;BA)", true, true, repairable, "missing_admin_full_control"},
		{"inheritance", "O:BAG:BAD:(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", true, true, repairable, "inheritance_drift"},
		{"no-propagate", "O:BAG:BAD:P(A;OICINP;FA;;;SY)(A;OICI;FA;;;BA)", true, true, repairable, "inheritance_drift"},
		{"empty", "O:BAG:BAD:P", true, true, repairable, "missing_system_full_control"},
		{"owner", "O:BUG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)", false, false, unsafeState, "untrusted_owner"},
		{"null-dacl", "O:BAG:BAD:NO_ACCESS_CONTROL", true, true, unsafeState, "security_descriptor_unreadable"},
		{"absent-dacl", "O:BAG:BA", true, true, unsafeState, "security_descriptor_unreadable"},
		{"deny-ambiguous", "O:BAG:BAD:P(D;;GR;;;BU)(A;;FA;;;SY)(A;;FA;;;BA)", false, false, unsafeState, "security_descriptor_unreadable"},
		{"duplicate-trusted", "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;GR;;;BA)", false, false, repairable, "noncanonical_trusted_aces"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd := descriptor(t, tc.sddl)
			before := sd.String()
			a := classifyDescriptor(sd, tc.dir, tc.root)
			if a.class != tc.class || !slices.Contains(a.reasons, tc.reason) {
				t.Fatalf("assessment=%+v", a)
			}
			if sd.String() != before {
				t.Fatal("classification mutated descriptor")
			}
		})
	}
	for _, sid := range []string{"BU", "AU", "WD", "IU", "S-1-5-21-111-222-333-1234"} {
		for _, mask := range []string{"GR", "GW", "SD", "WD", "WO", "0x00000020", "0x00000000"} {
			a := classifyDescriptor(descriptor(t, "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;"+mask+";;;"+sid+")"), false, false)
			if a.class != unsafeState || a.reasons[0] != "untrusted_ace" || a.principal == "" {
				t.Fatalf("accepted foreign ACE %s/%s: %+v", sid, mask, a)
			}
		}
	}
	if a := classifyDescriptor(nil, false, false); a.class != unsafeState {
		t.Fatal(a)
	}
}

type fakeSecurityObject struct {
	before, after assessment
	writes        int
	fail          bool
}

func (f *fakeSecurityObject) inspect() assessment {
	if f.writes > 0 {
		return f.after
	}
	return f.before
}
func (f *fakeSecurityObject) repair() error {
	f.writes++
	if f.fail {
		return errors.New("injected")
	}
	return nil
}

func TestRepairEngineFailClosedAndIdempotent(t *testing.T) {
	ok := assessment{class: canonical, reasons: []string{"acl_canonical"}}
	drift := assessment{class: repairable, reasons: []string{"missing_admin_full_control"}}
	for _, tc := range []struct {
		name          string
		before, after assessment
		fail          bool
		writes        int
		reason        string
	}{
		{"canonical", ok, ok, false, 0, ""},
		{"repair", drift, ok, false, 1, ""},
		{"unsafe", unsafeAssessment("untrusted_ace"), ok, false, 0, "untrusted_ace"},
		{"unreadable", unsafeAssessment("security_descriptor_unreadable"), ok, false, 0, "security_descriptor_unreadable"},
		{"mutation-failure", drift, drift, true, 1, "repair_failed"},
		{"post-failure", drift, drift, false, 1, "post_repair_validation_failed"},
		{"post-unsafe", drift, unsafeAssessment("untrusted_owner"), false, 1, "post_repair_validation_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSecurityObject{before: tc.before, after: tc.after, fail: tc.fail}
			var logs []Diagnostic
			err := ensureObject(context.Background(), "fixture", f, func(d Diagnostic) { logs = append(logs, d) })
			if (err != nil) != (tc.reason != "") || err != nil && !strings.Contains(err.Error(), tc.reason) || f.writes != tc.writes {
				t.Fatalf("err=%v writes=%d", err, f.writes)
			}
			if err == nil {
				if err = ensureObject(context.Background(), "fixture", f, nil); err != nil || f.writes != tc.writes {
					t.Fatal("not idempotent")
				}
			}
			if tc.reason != "" && !slices.ContainsFunc(logs, func(d Diagnostic) bool { return d.Reason == tc.reason && d.Action == "fail_closed" }) {
				t.Fatal("missing diagnostic", logs)
			}
		})
	}
}

// Actual Windows metadata writes on temporary, caller-owned fixtures. Holding
// handles lets an unelevated test restore its original ACL after removing caller
// access. Owner substitution is IN MEMORY ONLY to unit-test DACL classification;
// production inspect still refuses this fixture's untrusted owner.
func TestWindowsDACLRepairPreservesBytesAndDoesNotPropagate(t *testing.T) {
	root := t.TempDir()
	contents := map[string][]byte{
		"agent_config.json":     []byte(`{"agent_id":"fixture","public_key":"opaque-test-public","encrypted_private_key":"opaque-test-ciphertext","private_key_protection":"dpapi-machine-v1"}`),
		"enrollment_state.json": []byte(`{"fixture":"enrollment"}`),
		".env":                  []byte("TEST_ONLY=fixture\n"),
		".runtime.lock":         {},
	}
	for name, data := range contents {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	open := func(path string, dir bool) *pinnedObject {
		t.Helper()
		f, err := openPinned(path, windows.MAXIMUM_ALLOWED, windows.OPEN_EXISTING, nil)
		if err != nil {
			t.Fatal(err)
		}
		o := &pinnedObject{file: f, path: path, directory: dir, root: dir}
		sd, err := windows.GetSecurityInfo(o.handle(), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			f.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			dacl, _, _ := sd.DACL()
			if err := windows.SetSecurityInfo(o.handle(), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
				t.Error(err)
			}
			f.Close()
		})
		return o
	}
	dir := open(root, true)
	var files []*pinnedObject
	for name := range contents {
		files = append(files, open(filepath.Join(root, name), false))
	}
	childBefore := map[string]string{}
	for _, o := range files {
		sd, err := windows.GetSecurityInfo(o.handle(), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		childBefore[o.path] = sd.String()
	}
	if err := dir.repair(); err != nil {
		t.Fatal(err)
	}
	for _, o := range files {
		sd, err := windows.GetSecurityInfo(o.handle(), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil || sd.String() != childBefore[o.path] {
			t.Fatal("parent repair propagated to child", err)
		}
	}
	for _, o := range append(files, dir) {
		if err := o.repair(); err != nil {
			t.Fatal(err)
		}
		sd, err := windows.GetSecurityInfo(o.handle(), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		absolute, err := sd.ToAbsolute()
		if err != nil {
			t.Fatal(err)
		}
		owner, _ := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
		if err := absolute.SetOwner(owner, false); err != nil {
			t.Fatal(err)
		}
		if a := classifyDescriptor(absolute, o.directory, o.root); a.class != canonical {
			t.Fatalf("repaired DACL not canonical: %+v", a)
		}
		before := sd.String()
		if err := o.repair(); err != nil {
			t.Fatal(err)
		}
		after, err := windows.GetSecurityInfo(o.handle(), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil || after.String() != before {
			t.Fatal("DACL not stable", err)
		}
		if !o.directory {
			data, err := io.ReadAll(o.file)
			if err != nil {
				t.Fatal(err)
			}
			if sha256.Sum256(data) != sha256.Sum256(contents[filepath.Base(o.path)]) {
				t.Fatal("content changed")
			}
		}
	}
}

func TestPinnedObjectRefusesTypeEscapeHardlinkAndReparse(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fixture")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := openPinned(path, windows.MAXIMUM_ALLOWED, windows.OPEN_EXISTING, nil)
	if err != nil {
		t.Fatal(err)
	}
	o := &pinnedObject{file: f, path: path, directory: true}
	if a := o.inspect(); a.reasons[0] != "unexpected_object_type" {
		t.Fatal(a)
	}
	o.directory = false
	o.path = filepath.Join(root, "elsewhere")
	if a := o.inspect(); a.reasons[0] != "path_escape" {
		t.Fatal(a)
	}
	if err := os.Rename(path, filepath.Join(root, "moved")); err == nil {
		t.Fatal("pinned object could be renamed")
	}
	f.Close()
	if err := os.Link(path, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	f, err = openPinned(path, windows.MAXIMUM_ALLOWED, windows.OPEN_EXISTING, nil)
	if err != nil {
		t.Fatal(err)
	}
	o.file = f
	o.path = path
	if a := o.inspect(); a.reasons[0] != "hard_link_detected" {
		t.Fatal(a)
	}
	f.Close()
	t.Run("symlink", func(t *testing.T) {
		link := filepath.Join(root, "link")
		if err := os.Symlink(path, link); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		f, err := openPinned(link, windows.MAXIMUM_ALLOWED, windows.OPEN_EXISTING, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		o := &pinnedObject{file: f, path: link}
		if a := o.inspect(); a.reasons[0] != "reparse_point_detected" {
			t.Fatal(a)
		}
	})
}

func TestEnsureRefusesNonMachinePathsBeforeOpening(t *testing.T) {
	paths, _ := apppaths.Console(t.TempDir())
	var reasons []string
	release, err := EnsureRuntimeSecurity(context.Background(), paths, func(d Diagnostic) { reasons = append(reasons, d.Reason) })
	if err == nil || release != nil || !slices.Contains(reasons, "path_escape") {
		t.Fatalf("err=%v reasons=%v", err, reasons)
	}
	entries, err := os.ReadDir(paths.Root)
	if err != nil || len(entries) != 0 {
		t.Fatal("refusal changed fixture")
	}
}

func TestReparseRootAndAncestorRefusedWithoutTraversal(t *testing.T) {
	container := t.TempDir()
	target := filepath.Join(container, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "agent_config.json")
	if err := os.WriteFile(marker, []byte("untouched fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(container, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	before, err := windows.GetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{link, filepath.Join(link, "nested")} {
		var events []Diagnostic
		unlock, err := ensureFixtureScope(context.Background(), root, func(d Diagnostic) { events = append(events, d) })
		if err == nil {
			unlock()
			t.Fatal("reparse traversal accepted")
		}
		if !strings.Contains(err.Error(), "reparse_point_detected") || !slices.ContainsFunc(events, func(d Diagnostic) bool { return d.Reason == "reparse_point_detected" }) {
			t.Fatal("missing exact reparse diagnostic", err)
		}
	}
	after, err := windows.GetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || before.String() != after.String() {
		t.Fatal("target ACL changed", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "untouched fixture" {
		t.Fatal("followed reparse target", err)
	}
}

func TestPinnedRuntimeLockExcludesExistingRuntimeAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".runtime.lock")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	active, err := runlock.Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := openPinned(path, windows.MAXIMUM_ALLOWED, windows.OPEN_EXISTING, nil)
	if err == nil {
		f.Close()
		active()
		t.Fatal("maintenance opened MAXIMUM_ALLOWED while an existing runtime owns the file")
	}
	active()
	f, err = openPinned(path, windows.MAXIMUM_ALLOWED, windows.OPEN_EXISTING, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := windows.Handle(f.Fd())
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if unlock, err := runlock.Acquire(path); err == nil {
		unlock()
		f.Close()
		t.Fatal("runtime entered during maintenance")
	}
	if err := windows.UnlockFileEx(h, 0, 1, 0, &windows.Overlapped{}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	unlocked, err := runlock.Acquire(path)
	if err != nil {
		t.Fatal("maintenance failed to release existing runtime lock", err)
	}
	unlocked()
}

func TestEnsureRefusesUntrustedOwnerWithoutAnyMutation(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("this fixture requires an unelevated caller owner")
	}
	root := t.TempDir()
	before, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ensureFixtureScope(context.Background(), root, nil)
	if err == nil || !strings.Contains(err.Error(), "untrusted_owner") {
		t.Fatal(err)
	}
	after, err := windows.GetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || before.String() != after.String() {
		t.Fatal("unsafe tree changed", err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("unsafe tree received new files")
	}
}

// Exported only in the test binary so the external integration test can combine
// this engine with service.LoadPrivateKey without an import cycle or a production
// API that permits arbitrary repair targets.
func NewTrustedRuntimeFixtureForTest(t *testing.T) apppaths.Paths {
	t.Helper()
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("NOT VERIFIED: trusted-owner end-to-end ACL fixture requires elevated test token")
	}
	paths, _ := apppaths.Console(t.TempDir())
	SetFixtureDACLForTest(t, paths.Root, "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	return paths
}

func SetFixtureDACLForTest(t *testing.T, path, sddl string) {
	t.Helper()
	sd := descriptor(t, sddl)
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		t.Fatal(err)
	}
	flags := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.OWNER_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, flags, owner, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
}

func EnsureFixtureRuntimeForTest(ctx context.Context, root string, emit Reporter) (func(), error) {
	return ensureFixtureScope(ctx, root, emit)
}

func TestTrustedStartupScopePreflightAndRepair(t *testing.T) {
	for _, scenario := range []string{"canonical", "system-missing", "admin-flags", "unsafe-child", "critical-directory", "new-lock"} {
		t.Run(scenario, func(t *testing.T) {
			paths := NewTrustedRuntimeFixtureForTest(t)
			for _, name := range []string{"agent_config.json", "enrollment_state.json", ".env", ".runtime.lock"} {
				if scenario == "new-lock" && name == ".runtime.lock" {
					continue
				}
				if err := os.WriteFile(filepath.Join(paths.Root, name), []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(paths.Root, "logs"), 0700); err != nil {
				t.Fatal(err)
			}
			if scenario != "canonical" && scenario != "new-lock" {
				SetFixtureDACLForTest(t, paths.Root, "O:BAG:BAD:P(A;OICI;FA;;;BA)")
			}
			if scenario == "admin-flags" {
				SetFixtureDACLForTest(t, paths.Identity, "O:BAG:BAD:P(A;;FA;;;SY)(A;IO;FA;;;BA)")
			}
			if scenario == "unsafe-child" {
				SetFixtureDACLForTest(t, paths.Identity, "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;GR;;;BU)")
			}
			if scenario == "critical-directory" {
				if err := os.Remove(paths.Identity); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(paths.Identity, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before, err := windows.GetNamedSecurityInfo(paths.Root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
			if err != nil {
				t.Fatal(err)
			}
			var logs []Diagnostic
			unlock, err := ensureFixtureScope(context.Background(), paths.Root, func(d Diagnostic) { logs = append(logs, d) })
			if scenario == "unsafe-child" || scenario == "critical-directory" {
				if err == nil {
					unlock()
					t.Fatal("unsafe tree accepted")
				}
				after, _ := windows.GetNamedSecurityInfo(paths.Root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
				if before.String() != after.String() || slices.ContainsFunc(logs, func(d Diagnostic) bool { return d.Reason == "repair_started" }) {
					t.Fatal("preflight mutated root before refusing unsafe child")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateFile(paths.Identity); err != nil {
				unlock()
				t.Fatal(err)
			}
			if second, err := ensureFixtureScope(context.Background(), paths.Root, nil); err == nil {
				second()
				unlock()
				t.Fatal("concurrent repair accepted")
			}
			unlock()
			logs = nil
			unlock, err = ensureFixtureScope(context.Background(), paths.Root, func(d Diagnostic) { logs = append(logs, d) })
			if err != nil {
				t.Fatal(err)
			}
			unlock()
			if slices.ContainsFunc(logs, func(d Diagnostic) bool { return d.Reason == "repair_started" }) {
				t.Fatal("second pass repaired canonical tree")
			}
		})
	}
}

func ensureFixtureScope(ctx context.Context, root string, emit Reporter) (func(), error) {
	paths, _ := apppaths.Console(root)
	return ensureStartup(ctx, paths, emit)
}
