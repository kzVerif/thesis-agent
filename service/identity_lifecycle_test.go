package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func testIdentity() AgentConfig {
	return AgentConfig{AgentID: "11111111-1111-4111-8111-111111111111", Algorithm: "Ed25519",
		PublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), EncryptedPrivateKey: base64.StdEncoding.EncodeToString([]byte("opaque-test-ciphertext"))}
}

func writeTestIdentity(t *testing.T, path string, cfg AgentConfig) []byte {
	t.Helper()
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMigrationPreservesAllBytesAndConflict(t *testing.T) {
	source, dest := filepath.Join(t.TempDir(), "source.json"), filepath.Join(t.TempDir(), "target.json")
	b, _ := json.Marshal(testIdentity())
	b = append(b[:len(b)-1], []byte(", \"future_field\": {\"keep\": true}}\n")...)
	if err := os.WriteFile(source, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateIdentity(source, dest); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, b) {
		t.Fatal("migration changed identity bytes")
	}
	if err := MigrateIdentity(source, dest); err != nil {
		t.Fatal("repeat migration:", err)
	}
	conflict := testIdentity()
	conflict.AgentID = "22222222-2222-4222-8222-222222222222"
	writeTestIdentity(t, source, conflict)
	if err := MigrateIdentity(source, dest); err == nil {
		t.Fatal("conflict accepted")
	}
	got, _ = os.ReadFile(dest)
	if !bytes.Equal(got, b) {
		t.Fatal("conflict overwrote destination")
	}
}

func TestPartialIdentityNeverRepaired(t *testing.T) {
	valid := testIdentity()
	variants := []AgentConfig{valid, valid, valid, valid}
	variants[0].PublicKey = ""
	variants[1].EncryptedPrivateKey = ""
	variants[2].PublicKey = "invalid-base64"
	variants[3].AgentID = "invalid"
	for _, cfg := range variants {
		path := filepath.Join(t.TempDir(), "identity.json")
		before := writeTestIdentity(t, path, cfg)
		if _, err := LoadIdentity(path, true); err == nil {
			t.Fatal("partial identity accepted")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("partial identity overwritten")
		}
	}
	for _, bad := range []string{"{", "null", "[]", "{}"} {
		path := filepath.Join(t.TempDir(), "identity.json")
		os.WriteFile(path, []byte(bad), 0600)
		if _, err := LoadIdentity(path, true); err == nil {
			t.Fatal("malformed identity accepted")
		}
		after, _ := os.ReadFile(path)
		if string(after) != bad {
			t.Fatal("malformed identity overwritten")
		}
	}
}

func TestMissingUnattendedIdentityDoesNotCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.json")
	if _, err := LoadIdentity(path, false); err == nil {
		t.Fatal("missing unattended identity accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("identity was created")
	}
}

func TestInvalidDestinationIsNotOverwritten(t *testing.T) {
	source, dest := filepath.Join(t.TempDir(), "source.json"), filepath.Join(t.TempDir(), "target.json")
	writeTestIdentity(t, source, testIdentity())
	os.WriteFile(dest, []byte("broken"), 0600)
	if err := MigrateIdentity(source, dest); err == nil {
		t.Fatal("invalid destination accepted")
	}
	b, _ := os.ReadFile(dest)
	if string(b) != "broken" {
		t.Fatal("destination overwritten")
	}
}
