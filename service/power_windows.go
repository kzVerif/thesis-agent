//go:build windows

package service

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"log"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// WindowsPowerController schedules a local Windows shutdown. It deliberately
// avoids /f. The short in-process delay gives the Agent time to send its
// shutdown_result before Windows begins shutting down.
type WindowsPowerController struct {
	Delay     time.Duration
	mu        sync.Mutex
	scheduled bool
	closed    bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func (*WindowsPowerController) Mode() string { return "real" }

func (controller *WindowsPowerController) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("shutdown.exe unavailable: %w", err)
	}
	shutdownPath := filepath.Join(systemDirectory, "shutdown.exe")
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.closed {
		return fmt.Errorf("power controller is stopping")
	}
	if controller.scheduled {
		return fmt.Errorf("shutdown is already scheduled")
	}
	controller.scheduled = true

	delay := controller.Delay
	if delay <= 0 {
		delay = 3 * time.Second
	}
	log.Printf("real power shutdown scheduled in %s", delay)

	// Accepted power operations survive a network disconnect, but never a
	// legitimate runtime Stop. Close owns cancellation and joins this goroutine.
	powerCtx, cancel := context.WithCancel(context.Background())
	controller.cancel = cancel
	controller.wg.Add(1)
	go func() {
		defer controller.wg.Done()
		defer cancel()
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-powerCtx.Done():
			return
		case <-timer.C:
		}
		// /t 0 avoids Windows implicitly enabling /f for positive shutdown.exe
		// timeouts. This lets normal applications participate in shutdown instead
		// of deliberately force-closing them.
		cmd := exec.CommandContext(powerCtx,
			shutdownPath,
			"/s",
			"/t", "0",
			"/d", "p:0:0",
			"/c", "Thesis RAT remote shutdown",
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd.WaitDelay = 3 * time.Second
		if err := cmd.Run(); err != nil {
			log.Printf("real power shutdown command failed or cancelled")
			controller.mu.Lock()
			controller.scheduled = false
			controller.mu.Unlock()
		}
	}()
	return nil
}

func (controller *WindowsPowerController) Close() {
	controller.mu.Lock()
	controller.closed = true
	if controller.cancel != nil {
		controller.cancel()
	}
	controller.mu.Unlock()
	controller.wg.Wait()
}
