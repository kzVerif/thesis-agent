package service

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// Phase 1 validates structure without decrypting or replacing DPAPI ciphertext.
func TestIdentityValidationPreservesExistingBytes(t *testing.T) {
	cases := map[string]func(*AgentConfig){
		"missing-algorithm": func(c *AgentConfig) { c.Algorithm = "" },
		"wrong-algorithm":   func(c *AgentConfig) { c.Algorithm = "RSA" },
		"short-public-key": func(c *AgentConfig) {
			c.PublicKey = base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize-1))
		},
		"private-key-in-public-field": func(c *AgentConfig) {
			c.PublicKey = base64.StdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize))
		},
		"invalid-ciphertext-base64": func(c *AgentConfig) { c.EncryptedPrivateKey = "!" },
		"missing-id":                func(c *AgentConfig) { c.AgentID = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := testIdentity()
			mutate(&cfg)
			path := filepath.Join(t.TempDir(), "identity.json")
			before := writeTestIdentity(t, path, cfg)
			for _, allowCreate := range []bool{false, true} {
				if _, err := LoadIdentity(path, allowCreate); err == nil {
					t.Fatal("invalid identity accepted")
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("existing identity was changed")
				}
			}
		})
	}
	t.Run("valid-opaque-ciphertext", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "identity.json")
		cfg := testIdentity()
		before := writeTestIdentity(t, path, cfg)
		got, err := LoadIdentity(path, false)
		if err != nil || got != cfg {
			t.Fatalf("valid identity load: %v", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("valid identity was rewritten")
		}
	})
}
