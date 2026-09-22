package service

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryMigrationFiles struct {
	data    map[string][]byte
	onRead  func(string) error
	onWrite func(string, []byte, bool) error
	writes  int
}

func (m *memoryMigrationFiles) read(path string) ([]byte, error) {
	if m.onRead != nil {
		if err := m.onRead(path); err != nil {
			return nil, err
		}
	}
	data, ok := m.data[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return bytes.Clone(data), nil
}
func (m *memoryMigrationFiles) write(path string, data []byte, exclusive bool) error {
	m.writes++
	if m.onWrite != nil {
		if err := m.onWrite(path, data, exclusive); err != nil {
			return err
		}
	}
	if _, exists := m.data[path]; exclusive && exists {
		return os.ErrExist
	}
	m.data[path] = bytes.Clone(data)
	return nil
}

func keyFixture(t *testing.T) (AgentConfig, []byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clear(key) })
	cfg := testIdentity()
	cfg.PublicKey = base64.StdEncoding.EncodeToString(pub)
	cfg.EncryptedPrivateKey = base64.StdEncoding.EncodeToString([]byte("legacy-test-blob"))
	return cfg, key
}

func identityJSON(t *testing.T, cfg AgentConfig) []byte {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func migrationFixture(t *testing.T) (*memoryMigrationFiles, privateKeyCodec, AgentConfig, []byte) {
	t.Helper()
	cfg, key := keyFixture(t)
	data := identityJSON(t, cfg)
	data = append(data[:len(data)-1], []byte(",\"contributor_field\":{\"preserve\":true}}\n")...)
	enrollment, _ := json.Marshal(enrollmentFor(cfg, "https://test.invalid", true))
	files := &memoryMigrationFiles{data: map[string][]byte{"identity": data, "enrollment": enrollment}}
	// These are opaque stand-ins, not cryptography. Real Windows DPAPI tests
	// use the same migration engine with actual protect/unprotect operations.
	codec := privateKeyCodec{
		protect: func(got []byte) ([]byte, error) {
			if !bytes.Equal(got, key) {
				return nil, errors.New("wrong key")
			}
			return []byte("machine-test-blob"), nil
		},
		unprotect: func(blob []byte) ([]byte, error) {
			if string(blob) != "legacy-test-blob" && string(blob) != "machine-test-blob" {
				return nil, errors.New("bad blob")
			}
			return bytes.Clone(key), nil
		},
	}
	return files, codec, cfg, key
}

func TestPrivateKeyConsistency(t *testing.T) {
	cfg, key := keyFixture(t)
	cfg.PrivateKeyProtection = MachinePrivateKeyProtection
	for _, name := range []string{"valid", "bad-length", "bad-seed", "bad-suffix", "wrong-public", "corrupt-ciphertext", "unknown-version", "legacy"} {
		t.Run(name, func(t *testing.T) {
			current := cfg
			plain := bytes.Clone(key)
			decrypted := false
			switch name {
			case "bad-length":
				plain = plain[:ed25519.PrivateKeySize-1]
			case "bad-seed":
				plain[0] ^= 1
			case "bad-suffix":
				plain[ed25519.SeedSize] ^= 1
			case "wrong-public":
				current.PublicKey = base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
			case "unknown-version":
				current.PrivateKeyProtection = "dpapi-future-v99"
			case "legacy":
				current.PrivateKeyProtection = ""
			}
			got, err := privateKeyFromIdentity(identityJSON(t, current), true, func([]byte) ([]byte, error) {
				decrypted = true
				if name == "corrupt-ciphertext" {
					return nil, errors.New("DPAPI failure")
				}
				return plain, nil
			})
			defer clear(got)
			if name == "valid" {
				if err != nil || !bytes.Equal(got, key) {
					t.Fatal("valid key rejected")
				}
			} else {
				if err == nil || got != nil {
					t.Fatal("invalid private key accepted")
				}
				if (name == "legacy" || name == "unknown-version") && decrypted {
					t.Fatal("unsupported protection was decrypted")
				}
				if decrypted && name != "corrupt-ciphertext" && !bytes.Equal(plain, make([]byte, len(plain))) {
					t.Fatal("rejected plaintext not cleared")
				}
			}
		})
	}
}

func TestPrivateKeyMigrationPreservesIdentityAndEnrollment(t *testing.T) {
	files, codec, originalConfig, key := migrationFixture(t)
	original := bytes.Clone(files.data["identity"])
	enrollment := bytes.Clone(files.data["enrollment"])
	result, err := migratePrivateKeyProtection("identity", "enrollment", files, codec)
	if err != nil || result.AlreadyMigrated {
		t.Fatalf("migration: %v", err)
	}
	if !bytes.Equal(files.data[result.BackupPath], original) {
		t.Fatal("backup changed original bytes")
	}
	cfg, err := DecodeIdentity(files.data["identity"])
	if err != nil || cfg.AgentID != originalConfig.AgentID || cfg.Algorithm != originalConfig.Algorithm || cfg.PublicKey != originalConfig.PublicKey ||
		cfg.PrivateKeyProtection != MachinePrivateKeyProtection || cfg.EncryptedPrivateKey == originalConfig.EncryptedPrivateKey {
		t.Fatal("identity invariants failed")
	}
	got, err := privateKeyFromIdentity(files.data["identity"], true, codec.unprotect)
	if err != nil || !bytes.Equal(got, key) {
		t.Fatal("private key changed")
	}
	clear(got)
	var fields map[string]json.RawMessage
	json.Unmarshal(files.data["identity"], &fields)
	var extra map[string]bool
	if json.Unmarshal(fields["contributor_field"], &extra) != nil || !extra["preserve"] {
		t.Fatal("unknown contributor field lost")
	}
	if !bytes.Equal(enrollment, files.data["enrollment"]) {
		t.Fatal("enrollment state changed")
	}
	before := bytes.Clone(files.data["identity"])
	writes := files.writes
	result, err = migratePrivateKeyProtection("identity", "enrollment", files, codec)
	if err != nil || !result.AlreadyMigrated || files.writes != writes || !bytes.Equal(before, files.data["identity"]) {
		t.Fatal("repeat migration rewrote identity")
	}
}

func TestPrivateKeyMigrationFailuresPreserveOriginal(t *testing.T) {
	for _, name := range []string{"decrypt", "protect", "new-ciphertext", "backup-write", "backup-conflict", "backup-readback", "enrollment-binding", "replace", "ambiguous-replace", "final-decrypt", "final-read"} {
		t.Run(name, func(t *testing.T) {
			files, codec, _, _ := migrationFixture(t)
			original := bytes.Clone(files.data["identity"])
			enrollment := bytes.Clone(files.data["enrollment"])
			failure := errors.New("injected failure")
			switch name {
			case "decrypt":
				codec.unprotect = func([]byte) ([]byte, error) { return nil, failure }
			case "protect":
				codec.protect = func([]byte) ([]byte, error) { return nil, failure }
			case "new-ciphertext":
				codec.protect = func([]byte) ([]byte, error) { return []byte("invalid-new-blob"), nil }
			case "backup-conflict":
				files.data["identity"+privateKeyBackupSuffix] = []byte("existing-backup")
			case "enrollment-binding":
				files.data["enrollment"] = []byte("{}")
			case "backup-write":
				files.onWrite = func(_ string, _ []byte, exclusive bool) error {
					if exclusive {
						return failure
					}
					return nil
				}
			case "backup-readback":
				files.onRead = func(path string) error {
					if strings.HasSuffix(path, privateKeyBackupSuffix) && files.writes > 0 {
						return failure
					}
					return nil
				}
			case "replace", "ambiguous-replace":
				failed := false
				files.onWrite = func(path string, data []byte, exclusive bool) error {
					if !exclusive && !failed {
						failed = true
						if name == "ambiguous-replace" {
							files.data[path] = bytes.Clone(data)
						}
						return failure
					}
					return nil
				}
			case "final-decrypt":
				real := codec.unprotect
				calls := 0
				codec.unprotect = func(data []byte) ([]byte, error) {
					calls++
					if calls == 3 {
						return nil, failure
					}
					return real(data)
				}
			case "final-read":
				failed := false
				files.onRead = func(path string) error {
					if path == "identity" && files.writes == 2 && !failed {
						failed = true
						return failure
					}
					return nil
				}
			}
			_, err := migratePrivateKeyProtection("identity", "enrollment", files, codec)
			if err == nil {
				t.Fatal("failure accepted")
			}
			if !bytes.Equal(files.data["identity"], original) {
				t.Fatal("failed migration changed original identity")
			}
			if name != "enrollment-binding" && !bytes.Equal(files.data["enrollment"], enrollment) {
				t.Fatal("enrollment changed")
			}
			if name == "backup-conflict" && string(files.data["identity"+privateKeyBackupSuffix]) != "existing-backup" {
				t.Fatal("existing backup overwritten")
			}
		})
	}
}

func TestMigrationReportsUnverifiedRollback(t *testing.T) {
	files, codec, _, _ := migrationFixture(t)
	original := bytes.Clone(files.data["identity"])
	real := codec.unprotect
	calls := 0
	codec.unprotect = func(data []byte) ([]byte, error) {
		calls++
		if calls == 3 {
			return nil, errors.New("verification failure")
		}
		return real(data)
	}
	files.onWrite = func(_ string, _ []byte, exclusive bool) error {
		if !exclusive && files.writes >= 3 {
			return errors.New("disk unavailable")
		}
		return nil
	}
	_, err := migratePrivateKeyProtection("identity", "enrollment", files, codec)
	if err == nil || !strings.Contains(err.Error(), "rollback could not be verified") {
		t.Fatal("rollback failure misreported")
	}
	if !bytes.Equal(original, files.data["identity"+privateKeyBackupSuffix]) {
		t.Fatal("recovery backup lost")
	}
	if _, err := DecodeIdentity(files.data["identity"]); err != nil {
		t.Fatal("partial final identity")
	}
}

func TestMigrationReusesOnlyIdenticalBackupAndAllowsMissingMarker(t *testing.T) {
	files, codec, _, _ := migrationFixture(t)
	original := bytes.Clone(files.data["identity"])
	files.data["identity"+privateKeyBackupSuffix] = bytes.Clone(original)
	delete(files.data, "enrollment")
	if _, err := migratePrivateKeyProtection("identity", "enrollment", files, codec); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(files.data["identity"+privateKeyBackupSuffix], original) || files.writes != 1 {
		t.Fatal("identical backup was rewritten")
	}
	if _, exists := files.data["enrollment"]; exists {
		t.Fatal("migration created enrollment state")
	}
}

func TestMigrationRejectsPrivatePublicMismatchBeforeBackup(t *testing.T) {
	files, codec, cfg, _ := migrationFixture(t)
	cfg.PublicKey = base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	files.data["identity"] = identityJSON(t, cfg)
	original := bytes.Clone(files.data["identity"])
	if _, err := migratePrivateKeyProtection("identity", "enrollment", files, codec); err == nil {
		t.Fatal("mismatched key pair accepted")
	}
	if files.writes != 0 || !bytes.Equal(files.data["identity"], original) {
		t.Fatal("mismatched identity was modified")
	}
}

func TestOversizedMigrationCandidateFailsBeforeReplacement(t *testing.T) {
	files, codec, _, _ := migrationFixture(t)
	var fields map[string]json.RawMessage
	json.Unmarshal(files.data["identity"], &fields)
	// Compact input is bounded, but indenting a contributor field expands it.
	fields["large_extra"] = json.RawMessage("[" + strings.Repeat("0,", 180000) + "0]")
	original, err := json.Marshal(fields)
	if err != nil || len(original) > 1<<20 {
		t.Fatal("invalid test fixture size")
	}
	files.data["identity"] = original
	if _, err := migratePrivateKeyProtection("identity", "enrollment", files, codec); err == nil {
		t.Fatal("oversized candidate accepted")
	}
	if !bytes.Equal(files.data["identity"], original) {
		t.Fatal("oversized candidate replaced original")
	}
}

func TestMigrationInterruptionLeavesCompleteIdentityAndBackup(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		files, codec, _, _ := migrationFixture(t)
		original := bytes.Clone(files.data["identity"])
		files.onWrite = func(path string, data []byte, exclusive bool) error {
			if !exclusive {
				if afterCommit {
					files.data[path] = bytes.Clone(data)
				}
				panic("simulated interruption")
			}
			return nil
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Error("interruption not reached")
				}
			}()
			_, _ = migratePrivateKeyProtection("identity", "enrollment", files, codec)
		}()
		if !bytes.Equal(original, files.data["identity"+privateKeyBackupSuffix]) {
			t.Fatal("backup unavailable")
		}
		if _, err := DecodeIdentity(files.data["identity"]); err != nil {
			t.Fatal("incomplete identity after interruption")
		}
		if !afterCommit && !bytes.Equal(original, files.data["identity"]) {
			t.Fatal("pre-commit interruption changed original")
		}
	}
}

func TestLegacyLoaderAndUnknownVersionNeverRewrite(t *testing.T) {
	cfg, _ := keyFixture(t)
	for _, version := range []string{"", "unknown"} {
		cfg.PrivateKeyProtection = version
		path := filepath.Join(t.TempDir(), "identity.json")
		before := writeTestIdentity(t, path, cfg)
		if _, err := LoadPrivateKey(path); err == nil {
			t.Fatal("unsupported Service key protection accepted")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("loader changed identity")
		}
		if version == "unknown" {
			if _, err := LoadIdentity(path, true); err == nil {
				t.Fatal("unknown protection accepted")
			}
		} else {
			got, err := LoadServiceIdentity(path, false)
			if err != nil || got != cfg {
				t.Fatal("legacy Service startup rejected")
			}
		}
	}
}
