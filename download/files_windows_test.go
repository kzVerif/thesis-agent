//go:build windows

package download

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"ws-agent/internal/protectedpath"
)

func TestProtectedDownloadFileOperationsRefuseEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "fixture.part")
	if err := os.WriteFile(outside, []byte("retain fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: Config{Directory: root, Boundary: &protectedpath.Boundary{Root: root}}}
	if _, err := m.fileSHA256(context.Background(), outside); err == nil {
		t.Fatal("checksum opened outside boundary")
	}
	if err := m.removePart(outside); err == nil {
		t.Fatal("cleanup deleted outside boundary")
	}
	if err := m.publish(outside, filepath.Join(root, "file.bin")); err == nil {
		t.Fatal("publish escaped boundary")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "retain fixture" {
		t.Fatal("outside fixture changed", err)
	}
}

func TestProtectedDownloadManagerRefusesReparseContainer(t *testing.T) {
	target := t.TempDir()
	root := filepath.Join(t.TempDir(), "download-link")
	if err := os.Symlink(target, root); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	m, err := NewManager(Config{Directory: root, Boundary: &protectedpath.Boundary{Root: root}}, "fixture-agent", func(any) error { return nil })
	if err == nil {
		m.Close()
		t.Fatal("download manager followed reparse container")
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 0 {
		t.Fatal("download manager changed target")
	}
}
