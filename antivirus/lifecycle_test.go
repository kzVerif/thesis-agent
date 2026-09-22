package antivirus

import (
	"context"
	"testing"
	"time"
)

func TestCloseJoinsScan(t *testing.T) {
	started := make(chan struct{})
	ended := make(chan struct{})
	m := NewManager(func(ctx context.Context, _ Command) (Report, error) {
		close(started)
		<-ctx.Done()
		close(ended)
		return Report{}, ctx.Err()
	}, func(any) error { return nil })
	m.Submit(context.Background(), Command{Type: "virus_scan", RequestID: "test", ScanType: "quick"})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("scan not started")
	}
	done := make(chan struct{})
	go func() { m.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scan not joined")
	}
	select {
	case <-ended:
	default:
		t.Fatal("Close returned before scan")
	}
	m.Submit(context.Background(), Command{Type: "virus_scan", RequestID: "test2", ScanType: "quick"})
}
