//go:build windows

package service

import (
	"os"
	"path/filepath"
	"testing"
	"ws-agent/internal/protectedpath"
)

func TestProtectedLoggerRefusesReparseContainerBeforeWriting(t *testing.T) {
	target := t.TempDir()
	root := filepath.Join(t.TempDir(), "logs-link")
	if err := os.Symlink(target, root); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	b := &protectedpath.Boundary{Root: root}
	if closeLog, err := InitProtectedLoggingAt(filepath.Join(root, "agent.log"), b); err == nil {
		closeLog()
		t.Fatal("logger followed reparse root")
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 0 {
		t.Fatal("logger modified rejected tree")
	}
}

func TestProtectedRotationRejectsEscapeBeforeDeletingFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "agent.log")
	w, err := newRotatingLog(outside, 64, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, path := range []string{outside + ".1", outside + ".2"} {
		if err := os.WriteFile(path, []byte("retain fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w.boundary = &protectedpath.Boundary{Root: root}
	if err := w.rotate(); err == nil {
		t.Fatal("rotation ignored boundary")
	}
	for _, path := range []string{outside + ".1", outside + ".2"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "retain fixture" {
			t.Fatal("rotation mutated before preflight", err)
		}
	}
}
