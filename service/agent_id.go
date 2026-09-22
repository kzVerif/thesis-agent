package service

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"ws-agent/internal/protectedpath"
	"ws-agent/internal/statefile"

	"github.com/google/uuid"
)

const configFile = "agent_config.json"

type AgentConfig struct {
	AgentID              string `json:"agent_id"`
	Algorithm            string `json:"algorithm,omitempty"`
	PublicKey            string `json:"public_key,omitempty"`
	EncryptedPrivateKey  string `json:"encrypted_private_key,omitempty"`
	PrivateKeyProtection string `json:"private_key_protection,omitempty"`
}

const keyAlgorithm = "Ed25519"
const MachinePrivateKeyProtection = "dpapi-machine-v1"

func LoadOrCreateAgentConfig() (AgentConfig, error) {
	return LoadIdentity(configFile, true)
}

// DecodeIdentity validates structure only. DPAPI ciphertext remains opaque.
func DecodeIdentity(data []byte) (AgentConfig, error) {
	var cfg AgentConfig
	if json.Unmarshal(data, &cfg) != nil {
		return cfg, fmt.Errorf("identity JSON is invalid; administrator repair required")
	}
	if len(cfg.AgentID) != 36 || uuid.Validate(cfg.AgentID) != nil {
		return cfg, fmt.Errorf("identity agent_id is invalid")
	}
	if cfg.Algorithm != keyAlgorithm {
		return cfg, fmt.Errorf("identity algorithm is missing or unsupported")
	}
	if cfg.PrivateKeyProtection != "" && cfg.PrivateKeyProtection != MachinePrivateKeyProtection {
		return cfg, fmt.Errorf("unsupported private-key protection version")
	}
	pub, err := base64.StdEncoding.DecodeString(cfg.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return cfg, fmt.Errorf("identity public_key is missing or malformed")
	}
	private, err := base64.StdEncoding.DecodeString(cfg.EncryptedPrivateKey)
	if err != nil || len(private) == 0 {
		return cfg, fmt.Errorf("identity encrypted_private_key is missing or malformed")
	}
	return cfg, nil
}

func readIdentity(path string) ([]byte, AgentConfig, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, AgentConfig{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, AgentConfig{}, fmt.Errorf("identity must be a regular file smaller than 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, AgentConfig{}, err
	}
	cfg, err := DecodeIdentity(data)
	return data, cfg, err
}

// Only interactive provisioning/development may create a missing identity.
func LoadIdentity(path string, allowCreate bool) (AgentConfig, error) {
	return loadIdentity(path, allowCreate, false)
}

// Existing identities remain unchanged. Only new Service identities use
// machine protection, after validating the existing protected directory.
func LoadServiceIdentity(path string, allowCreate bool) (AgentConfig, error) {
	return loadIdentity(path, allowCreate, true)
}

func loadIdentity(path string, allowCreate, machine bool) (AgentConfig, error) {
	_, cfg, err := readIdentity(path)
	if err == nil {
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return AgentConfig{}, err
	}
	if !allowCreate {
		return AgentConfig{}, fmt.Errorf("identity is missing; run administrative provisioning first")
	}
	if machine {
		if err := protectedpath.ValidateDirectory(filepath.Dir(path)); err != nil {
			return AgentConfig{}, err
		}
	}
	cfg = AgentConfig{AgentID: uuid.NewString(), Algorithm: keyAlgorithm}
	if err := addKeyPairWithProtection(&cfg, machine); err != nil {
		return AgentConfig{}, err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return AgentConfig{}, err
	}
	if err := statefile.Write(path, data, true); err != nil {
		return AgentConfig{}, fmt.Errorf("create identity without replacing existing state: %w", err)
	}
	return cfg, nil
}

// MigrateIdentity copies bytes, including unknown JSON fields, without changing
// keys or overwriting a destination. Only an explicit provisioning source is used.
func MigrateIdentity(source, destination string) error {
	data, src, err := readIdentity(source)
	if err != nil {
		return err
	}
	_, dst, err := readIdentity(destination)
	if err == nil {
		if src != dst {
			return fmt.Errorf("identity conflict: source and destination differ; no files changed")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := statefile.Write(destination, data, true); err != nil {
		return fmt.Errorf("identity migration refused: %w", err)
	}
	check, _, err := readIdentity(destination)
	if err != nil {
		return err
	}
	if !bytes.Equal(check, data) {
		return fmt.Errorf("identity migration verification failed")
	}
	return nil
}

func addKeyPair(config *AgentConfig) error {
	return addKeyPairWithProtection(config, false)
}

func addKeyPairWithProtection(config *AgentConfig, machine bool) error {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate %s key pair: %w", keyAlgorithm, err)
	}
	defer clear(privateKey)
	protect := protectPrivateKey
	if machine {
		protect = protectPrivateKeyForMachine
	}
	encrypted, err := protect(privateKey)
	if err != nil {
		return fmt.Errorf("protect private key with Windows DPAPI: %w", err)
	}
	config.Algorithm = keyAlgorithm
	config.PublicKey = base64.StdEncoding.EncodeToString(publicKey)
	config.EncryptedPrivateKey = base64.StdEncoding.EncodeToString(encrypted)
	if machine {
		config.PrivateKeyProtection = MachinePrivateKeyProtection
	} else {
		config.PrivateKeyProtection = ""
	}
	return nil
}

func saveAgentConfig(config AgentConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", configFile, err)
	}
	return statefile.Write(configFile, data, true)
}

func GetOrCreateAgentID() (string, error) {
	config, err := LoadOrCreateAgentConfig()
	if err != nil {
		return "", err
	}
	return config.AgentID, nil
}
