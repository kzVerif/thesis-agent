//go:build windows

package desktopcapture

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// The installed task uses InteractiveToken and the installing user's identity.
// Asking Task Scheduler to run it does not launch a GUI under LocalSystem or
// supply credentials. A missing/disabled task is reported, never recreated here.
func startHelperTask() error {
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(systemDirectory, "schtasks.exe"), "/Run", "/TN", `\ThesisAgentDesktop`)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("Desktop Helper task launch timed out: %w", ctx.Err())
		}
		return fmt.Errorf("run installed ThesisAgentDesktop task: %w", err)
	}
	return nil
}
