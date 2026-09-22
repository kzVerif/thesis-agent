//go:build windows || unix

package runlock

import (
	"path/filepath"
	"testing"
)

func TestSingleRuntimeAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".runtime.lock")
	release, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(path)
	if err == nil {
		second()
		release()
		t.Fatal("second runtime acquired state")
	}
	release()
	third, err := Acquire(path)
	if err != nil {
		t.Fatal("lock not released:", err)
	}
	third()
}
