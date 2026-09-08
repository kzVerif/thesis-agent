package service

import (
	"encoding/json"
	"os"

	"github.com/google/uuid"
)

const configFile = "agent_config.json"

type AgentConfig struct {
	AgentID string `json:"agent_id"`
}

func GetOrCreateAgentID() (string, error) {
	// Reuse the persisted ID so the server recognizes this agent after restart.
	if data, err := os.ReadFile(configFile); err == nil {
		var config AgentConfig
		if err := json.Unmarshal(data, &config); err == nil && config.AgentID != "" {
			return config.AgentID, nil
		}
	}

	newID := uuid.NewString()
	data, err := json.MarshalIndent(AgentConfig{AgentID: newID}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configFile, data, 0600); err != nil {
		return "", err
	}

	return newID, nil
}
