package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"ws-agent/internal/statefile"
)

type enrollmentState struct {
	AgentID         string `json:"agent_id"`
	PublicKeySHA256 string `json:"public_key_sha256"`
	API             string `json:"api"`
	Verified        bool   `json:"verified"`
}

func enrollmentFor(cfg AgentConfig, api string, verified bool) enrollmentState {
	pub, _ := base64.StdEncoding.DecodeString(cfg.PublicKey)
	h := sha256.Sum256(pub)
	return enrollmentState{cfg.AgentID, hex.EncodeToString(h[:]), strings.TrimRight(api, "/"), verified}
}

func ReadEnrollment(path string, cfg AgentConfig, api string) (bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var state enrollmentState
	if len(b) > 1<<20 || json.Unmarshal(b, &state) != nil {
		return false, fmt.Errorf("enrollment state is malformed; administrator repair required")
	}
	return state == enrollmentFor(cfg, api, true), nil
}

func WriteEnrollment(path string, cfg AgentConfig, api string, verified bool) error {
	b, err := json.MarshalIndent(enrollmentFor(cfg, api, verified), "", "  ")
	if err != nil {
		return err
	}
	return statefile.Write(path, b, false)
}
