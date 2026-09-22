//go:build windows

package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirstProvisionCreatesStableDPAPIIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent_config.json")
	first, err := LoadIdentity(path, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadIdentity(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("identity changed")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeIdentity(b); err != nil {
		t.Fatal(err)
	}
	// Deliberately do not decrypt DPAPI; this phase treats ciphertext as opaque.
}
