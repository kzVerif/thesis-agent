//go:build windows

package service

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestWindowsMachineDPAPIAndPrivateKeyConsistency(t *testing.T) {
	cfg, key := keyFixture(t)
	blob, err := protectPrivateKeyForMachine(key)
	if err != nil {
		t.Fatal("machine DPAPI protection failed")
	}
	cfg.PrivateKeyProtection = MachinePrivateKeyProtection
	cfg.EncryptedPrivateKey = base64.StdEncoding.EncodeToString(blob)
	got, err := privateKeyFromIdentity(identityJSON(t, cfg), true, unprotectPrivateKey)
	if err != nil || !bytes.Equal(got, key) {
		t.Fatal("actual machine DPAPI round trip failed")
	}
	clear(got)
	cfg.EncryptedPrivateKey = base64.StdEncoding.EncodeToString([]byte("invalid-dpapi-blob"))
	if _, err := privateKeyFromIdentity(identityJSON(t, cfg), true, unprotectPrivateKey); err == nil {
		t.Fatal("corrupt DPAPI blob accepted")
	}
	short, err := protectPrivateKeyForMachine(key[:31])
	if err != nil {
		t.Fatal(err)
	}
	cfg.EncryptedPrivateKey = base64.StdEncoding.EncodeToString(short)
	if _, err := privateKeyFromIdentity(identityJSON(t, cfg), true, unprotectPrivateKey); err == nil {
		t.Fatal("valid DPAPI with invalid private-key length accepted")
	}
}

func TestWindowsActualDPAPIMigrationInMemory(t *testing.T) {
	files, _, cfg, key := migrationFixture(t)
	legacy, err := protectPrivateKey(key)
	if err != nil {
		t.Fatal("user DPAPI protection failed")
	}
	cfg.EncryptedPrivateKey = base64.StdEncoding.EncodeToString(legacy)
	files.data["identity"] = identityJSON(t, cfg)
	original := bytes.Clone(files.data["identity"])
	result, err := migratePrivateKeyProtection("identity", "enrollment", files, privateKeyCodec{protectPrivateKeyForMachine, unprotectPrivateKey})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(files.data[result.BackupPath], original) {
		t.Fatal("original encrypted backup changed")
	}
	got, err := privateKeyFromIdentity(files.data["identity"], true, unprotectPrivateKey)
	if err != nil || !bytes.Equal(got, key) {
		t.Fatal("actual DPAPI migration changed private key")
	}
	clear(got)
}

func TestNewMachineKeyGeneration(t *testing.T) {
	cfg := AgentConfig{AgentID: "11111111-1111-4111-8111-111111111111"}
	if err := addKeyPairWithProtection(&cfg, true); err != nil {
		t.Fatal(err)
	}
	if cfg.PrivateKeyProtection != MachinePrivateKeyProtection {
		t.Fatal("new machine key lacks metadata")
	}
	key, err := privateKeyFromIdentity(identityJSON(t, cfg), true, unprotectPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	clear(key)
}
