package antivirus

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Command struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	ScanType  string `json:"scan_type"`
	Path      string `json:"path,omitempty"`
}

type Report struct {
	ExitCode        int    `json:"exit_code"`
	Output          string `json:"output"`
	OutputTruncated bool   `json:"output_truncated"`
}

type Result struct {
	Type       string  `json:"type"`
	RequestID  string  `json:"request_id"`
	ScanType   string  `json:"scan_type"`
	Status     string  `json:"status"`
	StartedAt  string  `json:"started_at,omitempty"`
	FinishedAt string  `json:"finished_at,omitempty"`
	Report     *Report `json:"report,omitempty"`
	Error      string  `json:"error,omitempty"`
}

type Scanner func(context.Context, Command) (Report, error)

type Manager struct {
	closed bool
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	busy   bool
	scan   Scanner
	send   func(any) error
}

func NewManager(scan Scanner, send func(any) error) *Manager {
	return &Manager{scan: scan, send: send}
}

func Validate(c Command) error {
	if strings.TrimSpace(c.RequestID) == "" || len(c.RequestID) > 128 {
		return fmt.Errorf("request_id must contain 1-128 bytes")
	}
	switch c.ScanType {
	case "quick", "full":
		if c.Path != "" {
			return fmt.Errorf("path is only supported for custom scans")
		}
	case "custom":
		if !filepath.IsAbs(c.Path) || strings.ContainsAny(c.Path, "\x00*?\"") {
			return fmt.Errorf("custom scan requires an absolute file or directory path without wildcards")
		}
		if _, err := os.Stat(c.Path); err != nil {
			return fmt.Errorf("custom path: %w", err)
		}
	default:
		return fmt.Errorf("scan_type must be quick, custom or full")
	}
	return nil
}

func (m *Manager) emit(r Result) {
	if err := m.send(r); err != nil {
		log.Printf("send antivirus event failed")
	}
}

func (m *Manager) Reject(c Command, reason string) {
	m.emit(Result{Type: "virus_scan_result", RequestID: c.RequestID, ScanType: c.ScanType, Status: "rejected", Error: reason, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)})
}

func (m *Manager) Submit(ctx context.Context, c Command) {
	if err := Validate(c); err != nil {
		m.Reject(c, err.Error())
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	if m.busy {
		m.mu.Unlock()
		m.Reject(c, "another scan is running")
		return
	}
	m.busy = true
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		defer cancel()
		defer func() { m.mu.Lock(); m.busy = false; m.mu.Unlock() }()
		r := Result{Type: "virus_scan_status", RequestID: c.RequestID, ScanType: c.ScanType, Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		m.emit(r)
		report, err := m.scan(ctx, c)
		r.Type, r.Status, r.Report = "virus_scan_result", "completed", &report
		r.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err != nil {
			r.Status, r.Error = "failed", err.Error()
		}
		if ctx.Err() != nil {
			r.Status, r.Error = "cancelled", ctx.Err().Error()
		}
		m.emit(r)
	}()
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
