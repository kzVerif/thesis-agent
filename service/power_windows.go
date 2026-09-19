//go:build windows

package service

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync/atomic"
	"time"
)

// WindowsPowerController schedules a local Windows shutdown. It deliberately
// avoids /f. The short in-process delay gives the Agent time to send its
// shutdown_result before Windows begins shutting down.
type WindowsPowerController struct {
	Delay     time.Duration
	scheduled atomic.Bool
}

func (*WindowsPowerController) Mode() string { return "real" }

func (controller *WindowsPowerController) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	shutdownPath, err := exec.LookPath("shutdown.exe")
	if err != nil {
		return fmt.Errorf("shutdown.exe unavailable: %w", err)
	}
	if !controller.scheduled.CompareAndSwap(false, true) {
		return fmt.Errorf("shutdown is already scheduled")
	}

	delay := controller.Delay
	if delay <= 0 {
		delay = 3 * time.Second
	}
	log.Printf("real power shutdown scheduled in %s", delay)

	time.AfterFunc(delay, func() {
		// /t 0 avoids Windows implicitly enabling /f for positive shutdown.exe
		// timeouts. This lets normal applications participate in shutdown instead
		// of deliberately force-closing them.
		cmd := exec.Command(
			shutdownPath,
			"/s",
			"/t", "0",
			"/d", "p:0:0",
			"/c", "Thesis RAT remote shutdown",
		)
		if err := cmd.Run(); err != nil {
			log.Printf("real power shutdown command failed: %v", err)
			controller.scheduled.Store(false)
		}
	})
	return nil
}
