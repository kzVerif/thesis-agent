//go:build windows

package service

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestRotationWithMetadataHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	w, err := newRotatingLog(path, 64, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, p := range []string{path + ".1", path + ".2"} {
		if err := os.WriteFile(p, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{path, path + ".1", path + ".2"} {
		name, _ := windows.UTF16PtrFromString(p)
		h, err := windows.CreateFile(name, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(h)
	}
	if err := w.rotateFiles(); err != nil {
		t.Fatal(err)
	}
}
