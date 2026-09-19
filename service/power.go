package service

import (
	"context"
	"log"
)

// MockPowerController only logs requests; it never changes OS power state.
type MockPowerController struct{}

func (MockPowerController) Mode() string { return "mock" }

func (MockPowerController) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log.Printf("mock power shutdown requested")
	return nil
}
