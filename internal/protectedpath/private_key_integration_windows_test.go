//go:build windows

package protectedpath_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"testing"
	"ws-agent/internal/protectedpath"
	"ws-agent/service"
)

func TestRepairedFixtureReachesAuthoritativePrivateKeyLoader(t *testing.T) {
	paths := protectedpath.NewTrustedRuntimeFixtureForTest(t)
	// Generates a disposable fixture identity through existing machine DPAPI code.
	// No real Agent identity, endpoint, enrollment token, or machine runtime is used.
	identity, err := service.LoadServiceIdentity(paths.Identity, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.WriteEnrollment(paths.Enrollment, identity, "https://fixture.invalid", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config, []byte("FIXTURE_ONLY=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hashes := map[string][32]byte{}
	for _, path := range []string{paths.Identity, paths.Enrollment, paths.Config} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		hashes[path] = sha256.Sum256(data)
	}
	var first []byte
	for _, drift := range []bool{false, true, false} {
		if drift {
			protectedpath.SetFixtureDACLForTest(t, paths.Identity, "O:BAG:BAD:P(A;;FA;;;BA)")
		}
		unlock, err := protectedpath.EnsureFixtureRuntimeForTest(context.Background(), paths.Root, nil)
		if err != nil {
			t.Fatal(err)
		}
		key, err := service.LoadPrivateKey(paths.Identity)
		if err != nil {
			unlock()
			t.Fatal(err)
		}
		if first == nil {
			first = append([]byte(nil), key...)
			defer clear(first)
		} else if !bytes.Equal(first, key) {
			clear(key)
			unlock()
			t.Fatal("private key changed")
		}
		clear(key)
		unlock()
		for path, before := range hashes {
			data, err := os.ReadFile(path)
			if err != nil || sha256.Sum256(data) != before {
				t.Fatal("repair changed fixture bytes", err)
			}
		}
	}
}
