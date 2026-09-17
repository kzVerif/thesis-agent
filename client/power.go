package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// PowerController accepts a shutdown request and reports the active execution mode.
// Phase 2 supports both mock and real implementations without changing the wire shape.
type PowerController interface {
	Shutdown(context.Context) error
	Mode() string
}

// ConfigurePower must be called before Run, like ConfigureDownloads.
func (c *Client) ConfigurePower(controller PowerController) {
	c.powerController = controller
}

// powerCommand is separate from streamCommand: shutdown is a one-shot action.
type powerCommand struct {
	RequestID string
}

type powerCommandResult struct {
	Type      string `json:"type"`
	Action    string `json:"action"`
	RequestID string `json:"request_id"`
	Success   bool   `json:"success"`
	Mode      string `json:"mode"`
	Message   string `json:"message"`
}

func parsePowerCommand(data []byte) (powerCommand, bool) {
	var message struct {
		Type      string `json:"type"`
		Action    string `json:"action"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(data, &message); err != nil || message.Type != "power" || message.Action != "shutdown" {
		return powerCommand{}, false
	}
	// uuid.Validate also accepts URNs, braces and unhyphenated UUIDs. The wire
	// contract uses the 36-byte, hyphenated form; preserve its original casing.
	if len(message.RequestID) != 36 || uuid.Validate(message.RequestID) != nil {
		return powerCommand{}, false
	}
	return powerCommand{RequestID: message.RequestID}, true
}

func powerMode(controller PowerController) string {
	if controller == nil {
		// Preserve the existing failure envelope so the server can consume it.
		return "mock"
	}
	if controller.Mode() == "real" {
		return "real"
	}
	return "mock"
}

func (c *Client) handlePowerCommand(ctx context.Context, conn *safeConnection, command powerCommand) {
	if ctx.Err() != nil {
		return
	}
	result := powerCommandResult{
		Type:      "power",
		Action:    "shutdown_result",
		RequestID: command.RequestID,
		Mode:      powerMode(c.powerController),
	}
	switch {
	case c.powerController == nil:
		result.Message = "power controller unavailable"
	default:
		if err := c.powerController.Shutdown(ctx); err != nil {
			// Fixed messages keep internal errors private and stay below 4096 bytes.
			result.Message = "shutdown command failed"
			fmt.Println("Power shutdown request failed:", err)
		} else {
			result.Success = true
			result.Message = "shutdown command accepted"
		}
	}
	// Bind results to the requesting connection; never replay across reconnects.
	if err := writeJSON(ctx, conn, result); err != nil {
		fmt.Println("Send power shutdown result failed:", err)
	}
}
