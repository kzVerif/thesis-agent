//go:build windows

package protectedpath

import (
	"context"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAppendHandlePreservesExistingBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "append-fixture.log")
	if err := os.WriteFile(path, []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := openPinned(path, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.FILE_APPEND_DATA, windows.OPEN_ALWAYS, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("second\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first\nsecond\n" {
		t.Fatalf("append changed existing bytes: %q %v", data, err)
	}
}

func TestOnAccessPathEscapeAndReparseBoundary(t *testing.T) {
	root := t.TempDir()
	b := &Boundary{Root: root}
	for _, path := range []string{filepath.Join(filepath.Dir(root), "outside.log"), filepath.Join(root, "file:stream"), filepath.Join(root, "alias.")} {
		if f, err := b.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
			f.Close()
			t.Fatal("unsafe file path accepted")
		}
		called := false
		if err := b.WithFiles([]string{path}, func() error { called = true; return nil }); err == nil || called {
			t.Fatal("unsafe mutation accepted")
		}
	}
	target := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	b.Root = link
	if err := b.EnsureDirectory(link); err == nil || !strings.Contains(err.Error(), "reparse_point_detected") {
		t.Fatal("reparse container accepted", err)
	}
	if f, err := b.OpenFile(filepath.Join(link, "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
		f.Close()
		t.Fatal("followed reparse container")
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 0 {
		t.Fatal("created file through reparse")
	}
}

func TestProtectedDynamicIOAndIgnoredHistoricalChildren(t *testing.T) {
	paths := NewTrustedRuntimeFixtureForTest(t)
	b := &Boundary{Root: paths.Root}
	for _, dir := range []string{filepath.Dir(paths.Log), paths.Downloads} {
		if err := b.EnsureDirectory(dir); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{paths.Log + ".1", filepath.Join(paths.Downloads, "historical.bin")} {
		if err := os.WriteFile(path, []byte("historical fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		SetFixtureDACLForTest(t, path, "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;GR;;;BU)")
	}
	unlock, err := ensureStartup(context.Background(), paths, nil)
	if err != nil {
		t.Fatal("historical children blocked startup", err)
	}
	unlock()
	f, err := b.OpenFile(paths.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("first\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	f, err = b.OpenFile(paths.Log, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("second\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	data, err := os.ReadFile(paths.Log)
	if err != nil || string(data) != "first\nsecond\n" {
		t.Fatal("append semantics changed", err)
	}
	part, err := b.CreateTemp(paths.Downloads, "fixture-*.part")
	if err != nil {
		t.Fatal(err)
	}
	name := part.Name()
	if _, err = part.WriteString("payload"); err != nil {
		t.Fatal(err)
	}
	part.Close()
	part, err = b.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(part)
	part.Close()
	if err != nil || string(data) != "payload" {
		t.Fatal(err)
	}
	dest := filepath.Join(paths.Downloads, "new.bin")
	if err = b.WithFiles([]string{name, dest}, func() error { return os.Rename(name, dest) }); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.Log + ".1", filepath.Join(paths.Downloads, "historical.bin")} {
		if f, err = b.OpenFile(path, os.O_RDONLY, 0); err == nil {
			f.Close()
			t.Fatal("unsafe historical file opened")
		}
		called := false
		if err = b.WithFiles([]string{path}, func() error { called = true; return os.Remove(path) }); err == nil || called {
			t.Fatal("unsafe historical file deleted")
		}
	}
	link := filepath.Join(paths.Downloads, "link.bin")
	if err = os.Symlink(dest, link); err != nil {
		t.Fatal(err)
	}
	if f, err = b.OpenFile(link, os.O_RDONLY, 0); err == nil {
		f.Close()
		t.Fatal("existing reparse target opened")
	}
}

func TestStrictScopeRefusesUnsafeContainerAndKnownBackup(t *testing.T) {
	for _, scenario := range []string{"logs", "data", "downloads", "backup", "downloads-reparse"} {
		t.Run(scenario, func(t *testing.T) {
			paths := NewTrustedRuntimeFixtureForTest(t)
			for _, p := range []string{filepath.Dir(paths.Log), paths.Downloads} {
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			target := map[string]string{"logs": filepath.Dir(paths.Log), "data": filepath.Dir(paths.Downloads), "downloads": paths.Downloads, "backup": paths.IdentityBackup(), "downloads-reparse": paths.Downloads}[scenario]
			want := "untrusted_ace"
			if scenario == "downloads-reparse" {
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
				want = "reparse_point_detected"
			} else {
				flags := "OICI"
				if scenario == "backup" {
					flags = ""
					if err := os.WriteFile(target, []byte("opaque fixture backup"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				SetFixtureDACLForTest(t, target, "O:BAG:BAD:P(A;"+flags+";FA;;;SY)(A;"+flags+";FA;;;BA)(A;"+flags+";GR;;;BU)")
			}
			var logs []Diagnostic
			unlock, err := ensureStartup(context.Background(), paths, func(d Diagnostic) { logs = append(logs, d) })
			if err == nil {
				unlock()
				t.Fatal("unsafe strict object accepted")
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatal(err)
			}
			for _, d := range logs {
				if d.Reason == "repair_started" {
					t.Fatal("unsafe preflight mutated ACL")
				}
			}
		})
	}
}
