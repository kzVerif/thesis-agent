//go:build windows

package antivirus

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// Keep only a bounded prefix while continuing to drain process output.
type limitedOutput struct {
	data      []byte
	truncated bool
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 32*1024 - len(b.data)
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	b.data = append(b.data, p...)
	return n, nil
}

func defenderExecutable() (string, error) {
	paths, _ := filepath.Glob(filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows Defender", "Platform", "*", "MpCmdRun.exe"))
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	paths = append(paths, filepath.Join(os.Getenv("ProgramFiles"), "Windows Defender", "MpCmdRun.exe"))
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && filepath.IsAbs(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("Microsoft Defender MpCmdRun.exe was not found")
}

func Scan(ctx context.Context, c Command) (Report, error) {
	r := Report{ExitCode: -1}
	if err := Validate(c); err != nil {
		return r, err
	}
	executable, err := defenderExecutable()
	if err != nil {
		return r, err
	}
	scanType := map[string]string{"quick": "1", "full": "2", "custom": "3"}[c.ScanType]
	args := []string{"-Scan", "-ScanType", scanType}
	if c.ScanType == "custom" {
		args = append(args, "-File", c.Path)
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output := &limitedOutput{}
	cmd.Stdout, cmd.Stderr = output, output
	err = cmd.Run()
	r.Output = strings.ToValidUTF8(string(output.data), "�")
	r.OutputTruncated = output.truncated
	if cmd.ProcessState != nil {
		r.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return r, fmt.Errorf("Defender scan did not complete successfully (exit code %d): %w", r.ExitCode, err)
	}
	return r, nil
}
