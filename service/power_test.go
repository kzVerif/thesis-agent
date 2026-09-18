package service

import (
	"context"
	"errors"
	"testing"
)

func TestMockPowerController(t *testing.T) {
	controller := MockPowerController{}
	if err := controller.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := controller.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request: %v", err)
	}
}
