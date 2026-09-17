//go:build !windows

package service

import (
	"context"
	"fmt"
	"time"
)

// WindowsPowerController exists on non-Windows platforms only so the project
// can still compile cross-platform. Real shutdown remains Windows-only.
type WindowsPowerController struct {
	Delay time.Duration
}

func (*WindowsPowerController) Mode() string { return "real" }

func (*WindowsPowerController) Shutdown(context.Context) error {
	return fmt.Errorf("real shutdown is only supported on Windows")
}
