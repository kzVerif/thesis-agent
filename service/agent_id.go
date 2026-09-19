package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
)

const configFile = "agent_config.json"

type AgentConfig struct {
	AgentID             string `json:"agent_id"`
	Algorithm           string `json:"algorithm,omitempty"`
	PublicKey           string `json:"public_key,omitempty"`
	EncryptedPrivateKey string `json:"encrypted_private_key,omitempty"`
}

const keyAlgorithm = "Ed25519"

func LoadOrCreateAgentConfig() (AgentConfig, error) {
	if data, err := os.ReadFile(configFile); err == nil {
		var config AgentConfig
		if err := json.Unmarshal(data, &config); err != nil {
			return AgentConfig{}, fmt.Errorf("decode %s: %w", configFile, err)
		}
		if config.AgentID == "" {
			return AgentConfig{}, fmt.Errorf("%s has no agent_id", configFile)
		}
		if config.Algorithm == "" {
			config.Algorithm = keyAlgorithm
		}
		if config.PublicKey == "" || config.EncryptedPrivateKey == "" {
			if err := addKeyPair(&config); err != nil {
				return AgentConfig{}, err
			}
			if err := saveAgentConfig(config); err != nil {
				return AgentConfig{}, err
			}
		}
		return config, nil
	} else if !os.IsNotExist(err) {
		return AgentConfig{}, fmt.Errorf("read %s: %w", configFile, err)
	}

	config := AgentConfig{AgentID: uuid.NewString(), Algorithm: keyAlgorithm}
	if err := addKeyPair(&config); err != nil {
		return AgentConfig{}, err
	}
	if err := saveAgentConfig(config); err != nil {
		return AgentConfig{}, err
	}
	return config, nil
}

func addKeyPair(config *AgentConfig) error {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate %s key pair: %w", keyAlgorithm, err)
	}
	encrypted, err := protectPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("protect private key with Windows DPAPI: %w", err)
	}
	config.Algorithm = keyAlgorithm
	config.PublicKey = base64.StdEncoding.EncodeToString(publicKey)
	config.EncryptedPrivateKey = base64.StdEncoding.EncodeToString(encrypted)
	return nil
}

func saveAgentConfig(config AgentConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", configFile, err)
	}
	tmp := fmt.Sprintf("%s.%d.tmp", configFile, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write temporary agent config: %w", err)
	}
	if err := os.Rename(tmp, configFile); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", configFile, err)
	}
	return nil
}

func GetOrCreateAgentID() (string, error) {
	config, err := LoadOrCreateAgentConfig()
	if err != nil {
		return "", err
	}
	return config.AgentID, nil
}
