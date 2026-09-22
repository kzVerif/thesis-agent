//go:build windows

package statefile

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestFailedWindowsReplacementPreservesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	original := []byte("{\"test\":\"original\"}")
	if err := Write(path, original, true); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err := Write(path, []byte("{\"test\":\"replacement\"}"), false); err == nil {
		t.Fatal("expected sharing violation")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(original) {
		t.Fatal("failed replacement altered original")
	}
	tmp, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".state-*.tmp"))
	if err != nil || len(tmp) != 0 {
		t.Fatal("failed write leaked temporary file")
	}
}
