package client

import (
	"context"
	"encoding/json"
	"ws-agent/antivirus"
)

func (c *Client) handleVirusScan(ctx context.Context, data []byte) bool {
	var envelope struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &envelope) != nil || normalizeCommand(envelope.Type) != "virus_scan" {
		return false
	}
	var command antivirus.Command
	if err := json.Unmarshal(data, &command); err != nil {
		c.scanManager.Reject(command, "malformed virus_scan command")
		return true
	}
	c.scanManager.Submit(ctx, command)
	return true
}
