package antivirus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func receive(t *testing.T, events <-chan Result) Result {
	t.Helper()
	select {
	case r := <-events:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("missing event")
		return Result{}
	}
}

func TestManagerLifecycleAndBusy(t *testing.T) {
	for _, mode := range []string{"quick", "custom", "full"} {
		t.Run(mode, func(t *testing.T) {
			events := make(chan Result, 4)
			release := make(chan struct{})
			m := NewManager(func(ctx context.Context, c Command) (Report, error) {
				<-release
				return Report{ExitCode: 0, Output: "done"}, nil
			}, func(v any) error { events <- v.(Result); return nil })
			c := Command{RequestID: "job-1", ScanType: mode}
			if mode == "custom" {
				c.Path = t.TempDir()
			}
			m.Submit(context.Background(), c)
			if r := receive(t, events); r.Status != "running" || r.RequestID != "job-1" {
				t.Fatalf("%+v", r)
			}
			m.Submit(context.Background(), Command{RequestID: "job-2", ScanType: "quick"})
			if r := receive(t, events); r.Status != "rejected" {
				t.Fatalf("%+v", r)
			}
			close(release)
			if r := receive(t, events); r.Status != "completed" || r.Report.ExitCode != 0 || r.FinishedAt == "" {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestValidation(t *testing.T) {
	for _, c := range []Command{
		{ScanType: "quick"}, {RequestID: "x", ScanType: "unknown"},
		{RequestID: "x", ScanType: "custom"}, {RequestID: "x", ScanType: "custom", Path: "relative"},
		{RequestID: "x", ScanType: "full", Path: t.TempDir()},
	} {
		if Validate(c) == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
}

func TestScanFailure(t *testing.T) {
	events := make(chan Result, 2)
	m := NewManager(func(context.Context, Command) (Report, error) {
		return Report{ExitCode: 2}, errors.New("scan failed or threat requires action")
	}, func(v any) error { events <- v.(Result); return nil })
	m.Submit(context.Background(), Command{RequestID: "x", ScanType: "quick"})
	receive(t, events)
	if r := receive(t, events); r.Status != "failed" || r.Report.ExitCode != 2 || r.Error == "" {
		t.Fatalf("%+v", r)
	}
}
