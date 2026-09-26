package service

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"ws-agent/internal/apppaths"
	"ws-agent/internal/protectedpath"
	"ws-agent/internal/runlock"
	"ws-agent/internal/statefile"
)

const privateKeyBackupSuffix = apppaths.LegacyPrivateKeyBackupSuffix

type PrivateKeyMigrationResult struct {
	AlreadyMigrated bool
	BackupPath      string
}

// MigratePrivateKeyProtection operates only on the protected runtime copy.
// The CLI additionally requires elevation and a stopped SCM Service.
// No enrollment request, key generation or permission change occurs here.
func MigratePrivateKeyProtection(identityPath, enrollmentPath string) (PrivateKeyMigrationResult, error) {
	paths, err := apppaths.Machine()
	if err != nil {
		return PrivateKeyMigrationResult{}, err
	}
	if !strings.EqualFold(filepath.Clean(identityPath), paths.Identity) ||
		!strings.EqualFold(filepath.Clean(enrollmentPath), paths.Enrollment) {
		return PrivateKeyMigrationResult{}, fmt.Errorf("private-key migration only operates on the designated protected Service runtime identity")
	}
	root := filepath.Dir(identityPath)
	if filepath.Dir(enrollmentPath) != root {
		return PrivateKeyMigrationResult{}, fmt.Errorf("identity and enrollment must use the same protected runtime directory")
	}
	if err := protectedpath.ValidateDirectory(root); err != nil {
		return PrivateKeyMigrationResult{}, err
	}
	unlock, err := runlock.Acquire(filepath.Join(root, ".runtime.lock"))
	if err != nil {
		return PrivateKeyMigrationResult{}, fmt.Errorf("Service/runtime must be stopped before private-key migration")
	}
	defer unlock()
	return migratePrivateKeyProtection(identityPath, enrollmentPath, diskMigrationIO{}, privateKeyCodec{
		protect: protectPrivateKeyForMachine, unprotect: unprotectPrivateKey,
	})
}

// Dependencies isolate fault-injection tests from live state and real DPAPI.
// Production always uses the protected filesystem and Windows DPAPI above.
type migrationIO interface {
	read(string) ([]byte, error)
	write(string, []byte, bool) error
}

type privateKeyCodec struct {
	protect   func([]byte) ([]byte, error)
	unprotect func([]byte) ([]byte, error)
}

type diskMigrationIO struct{}

func (diskMigrationIO) read(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, fmt.Errorf("identity/backup/enrollment must be a bounded regular file")
	}
	if err := protectedpath.ValidateFile(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (diskMigrationIO) write(path string, data []byte, exclusive bool) error {
	if err := protectedpath.ValidateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return statefile.Write(path, data, exclusive)
}

func migratePrivateKeyProtection(path, enrollmentPath string, files migrationIO, codec privateKeyCodec) (PrivateKeyMigrationResult, error) {
	result := PrivateKeyMigrationResult{}
	original, err := files.read(path)
	if err != nil {
		return result, fmt.Errorf("cannot read protected identity; no migration performed")
	}
	cfg, err := DecodeIdentity(original)
	if err != nil {
		return result, err
	}
	key, err := privateKeyFromIdentity(original, false, codec.unprotect)
	if err != nil {
		return result, err
	}
	defer clear(key)
	enrollment, err := files.read(enrollmentPath)
	enrollmentMissing := os.IsNotExist(err)
	if err != nil && !enrollmentMissing {
		return result, fmt.Errorf("cannot read enrollment state; original identity unchanged")
	}
	if !enrollmentMissing {
		if err := validateEnrollmentKeyBinding(enrollment, cfg); err != nil {
			return result, err
		}
	}
	if cfg.PrivateKeyProtection == MachinePrivateKeyProtection {
		result.AlreadyMigrated = true
		return result, nil
	}

	backup := path + privateKeyBackupSuffix
	existing, err := files.read(backup)
	if os.IsNotExist(err) {
		if err := files.write(backup, original, true); err != nil {
			return result, fmt.Errorf("cannot create exclusive protected backup; original identity unchanged")
		}
	} else if err != nil || !bytes.Equal(existing, original) {
		return result, fmt.Errorf("existing backup differs or is unreadable; original identity unchanged")
	}
	check, err := files.read(backup)
	if err != nil || !bytes.Equal(check, original) {
		return result, fmt.Errorf("backup verification failed; original identity unchanged")
	}
	result.BackupPath = backup
	ciphertext, err := codec.protect(key)
	if err != nil {
		return result, fmt.Errorf("machine protection failed; original identity unchanged; backup retained")
	}
	if len(ciphertext) == 0 {
		return result, fmt.Errorf("machine protection returned empty ciphertext; original identity unchanged")
	}
	// RawMessage retains all contributor/unknown fields. Only these two values
	// are replaced; the backup retains the exact original JSON bytes.
	var fields map[string]json.RawMessage
	if json.Unmarshal(original, &fields) != nil {
		return result, fmt.Errorf("cannot prepare identity; original identity unchanged")
	}
	fields["encrypted_private_key"], _ = json.Marshal(base64.StdEncoding.EncodeToString(ciphertext))
	fields["private_key_protection"], _ = json.Marshal(MachinePrivateKeyProtection)
	candidate, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return result, fmt.Errorf("cannot encode identity; original identity unchanged")
	}
	if err := verifyMigratedIdentity(candidate, cfg, key, codec.unprotect); err != nil {
		return result, fmt.Errorf("new protection verification failed; original identity unchanged; backup retained")
	}
	current, err := files.read(path)
	if err != nil || !bytes.Equal(current, original) {
		return result, fmt.Errorf("identity changed or became unreadable before replacement; migration refused")
	}
	currentEnrollment, err := files.read(enrollmentPath)
	if (enrollmentMissing && !os.IsNotExist(err)) ||
		(!enrollmentMissing && (err != nil || !bytes.Equal(currentEnrollment, enrollment))) {
		return result, fmt.Errorf("enrollment state changed before replacement; original identity not replaced")
	}
	if err := files.write(path, candidate, false); err != nil {
		return result, rollbackPrivateKey(files, path, original)
	}
	published, err := files.read(path)
	if err != nil || !bytes.Equal(published, candidate) {
		return result, rollbackPrivateKey(files, path, original)
	}
	if err := verifyMigratedIdentity(published, cfg, key, codec.unprotect); err != nil {
		return result, rollbackPrivateKey(files, path, original)
	}
	return result, nil
}

func verifyMigratedIdentity(data []byte, original AgentConfig, key []byte, unprotect func([]byte) ([]byte, error)) error {
	if len(data) > 1<<20 {
		return fmt.Errorf("migrated identity exceeds the supported size")
	}
	cfg, err := DecodeIdentity(data)
	if err != nil {
		return err
	}
	if cfg.AgentID != original.AgentID || cfg.Algorithm != original.Algorithm || cfg.PublicKey != original.PublicKey {
		return fmt.Errorf("migration changed identity")
	}
	check, err := privateKeyFromIdentity(data, true, unprotect)
	if err != nil {
		return err
	}
	defer clear(check)
	if subtle.ConstantTimeCompare(key, check) != 1 {
		return fmt.Errorf("migration changed private key")
	}
	return nil
}

func rollbackPrivateKey(files migrationIO, path string, original []byte) error {
	current, err := files.read(path)
	if err == nil && bytes.Equal(current, original) {
		return fmt.Errorf("migration failed; original identity unchanged and verified; backup retained")
	}
	// An OS write can fail ambiguously. Always verify bytes even if rollback
	// itself returns an error; never claim restoration merely from a return code.
	_ = files.write(path, original, false)
	current, err = files.read(path)
	if err != nil || !bytes.Equal(current, original) {
		return fmt.Errorf("migration failed; rollback could not be verified; protected backup retained at %s for manual recovery", path+privateKeyBackupSuffix)
	}
	return fmt.Errorf("migration failed; original identity restored and verified; backup retained")
}

func validateEnrollmentKeyBinding(data []byte, cfg AgentConfig) error {
	var state enrollmentState
	if len(data) > 1<<20 || json.Unmarshal(data, &state) != nil {
		return fmt.Errorf("enrollment state is malformed; original identity unchanged")
	}
	public, _ := base64.StdEncoding.DecodeString(cfg.PublicKey)
	hash := sha256.Sum256(public)
	if state.AgentID != cfg.AgentID || state.PublicKeySHA256 != hex.EncodeToString(hash[:]) {
		return fmt.Errorf("enrollment identity/public-key binding does not match; original identity unchanged")
	}
	return nil
}
