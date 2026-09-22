package service

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"ws-agent/internal/protectedpath"
)

// LoadPrivateKey is the authoritative Service private-key loader. It never
// creates, migrates or enrolls an identity. Caller must clear the returned key
// after use and hold the existing runtime lock while using runtime state.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, cfg, err := readIdentity(path)
	if err != nil {
		return nil, err
	}
	if cfg.PrivateKeyProtection != MachinePrivateKeyProtection {
		return nil, fmt.Errorf("legacy key protection requires explicit --migrate-private-key-protection in the original user's context")
	}
	if err := protectedpath.ValidateFile(path); err != nil {
		return nil, err
	}
	return privateKeyFromIdentity(data, true, unprotectPrivateKey)
}

func privateKeyFromIdentity(data []byte, requireMachine bool, unprotect func([]byte) ([]byte, error)) (ed25519.PrivateKey, error) {
	cfg, err := DecodeIdentity(data)
	if err != nil {
		return nil, err
	}
	if requireMachine && cfg.PrivateKeyProtection != MachinePrivateKeyProtection {
		return nil, fmt.Errorf("legacy key protection requires explicit migration")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(cfg.EncryptedPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("encrypted private key is malformed")
	}
	key, err := unprotect(ciphertext)
	if err != nil {
		clear(key)
		return nil, fmt.Errorf("private key cannot be decrypted in current context; explicit migration requires the original user's context")
	}
	if err := validatePrivateKey(key, cfg.PublicKey); err != nil {
		clear(key)
		return nil, err
	}
	return ed25519.PrivateKey(key), nil
}

func validatePrivateKey(key []byte, publicKey string) error {
	if len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("decrypted Ed25519 private key has invalid length")
	}
	// Public() returns the stored suffix. Derive from the seed instead, and
	// check all 64 bytes so an inconsistent seed/suffix cannot pass validation.
	derived := ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])
	defer clear(derived)
	if subtle.ConstantTimeCompare(key, derived) != 1 {
		return fmt.Errorf("decrypted Ed25519 private key is internally inconsistent")
	}
	public, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(public) != ed25519.PublicKeySize ||
		subtle.ConstantTimeCompare(derived[ed25519.SeedSize:], public) != 1 {
		return fmt.Errorf("private key does not match stored public key")
	}
	return nil
}
