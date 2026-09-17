package service

import (
	"context"
	"fmt"
)

// MockPowerController only logs requests; it never changes OS power state.
type MockPowerController struct{}

func (MockPowerController) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Println("[MOCK POWER] shutdown requested")
	return nil
}
