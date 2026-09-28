//go:build windows

package desktopcapture

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestMissingHelperWakeAndReconnect(t *testing.T) {
	now := time.Unix(1000, 0)
	wakes := 0
	available := false
	path := filepath.Join(t.TempDir(), "pipe-fixture")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	c := New()
	defer c.Close()
	c.now = func() time.Time { return now }
	c.wakeHelper = func() error { wakes++; return nil }
	c.openPipe = func() (*os.File, error) {
		if !available {
			return nil, &os.PathError{Op: "open", Path: pipeName, Err: windows.ERROR_FILE_NOT_FOUND}
		}
		return os.OpenFile(path, os.O_RDWR, 0)
	}
	for i := 0; i < 5; i++ {
		if err := c.ensurePipe(); err == nil {
			t.Fatal("missing pipe accepted")
		}
	}
	if wakes != 1 {
		t.Fatalf("wake attempts = %d", wakes)
	}
	available = true
	if err := c.ensurePipe(); err != nil {
		t.Fatal(err)
	}
	if wakes != 1 {
		t.Fatal("running helper was restarted")
	}
	// Model a disconnected Helper and a later frame request after cooldown.
	c.closePipe()
	available = false
	now = now.Add(15 * time.Second)
	if err := c.ensurePipe(); err == nil {
		t.Fatal("missing pipe accepted")
	}
	if wakes != 2 {
		t.Fatalf("wake attempts after exit = %d", wakes)
	}
}

func TestHelperWakeFailureIsThrottled(t *testing.T) {
	c := New()
	wakes := 0
	c.openPipe = func() (*os.File, error) { return nil, windows.ERROR_FILE_NOT_FOUND }
	c.now = func() time.Time { return time.Unix(1000, 0) }
	c.wakeHelper = func() error { wakes++; return errors.New("task disabled") }
	if err := c.ensurePipe(); err == nil || !strings.Contains(err.Error(), "task disabled") {
		t.Fatalf("error = %v", err)
	}
	_ = c.ensurePipe()
	if wakes != 1 {
		t.Fatalf("failed task started %d times", wakes)
	}
}

func TestOtherPipeErrorsDoNotWakeHelper(t *testing.T) {
	for _, failure := range []error{windows.ERROR_PIPE_BUSY, windows.ERROR_ACCESS_DENIED} {
		c := New()
		c.openPipe = func() (*os.File, error) { return nil, failure }
		c.wakeHelper = func() error { t.Fatal("unexpected launch"); return nil }
		if err := c.ensurePipe(); !errors.Is(err, failure) {
			t.Fatalf("error = %v", err)
		}
	}
}
