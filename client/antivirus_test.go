package client

import (
	"context"
	"testing"
	"time"
	"ws-agent/antivirus"
)

func TestVirusScanRoutingAndReconnectQueue(t *testing.T) {
	c := New("", time.Second, time.Second, time.Second)
	c.scanManager = antivirus.NewManager(func(context.Context, antivirus.Command) (antivirus.Report, error) {
		return antivirus.Report{ExitCode: 0}, nil
	}, c.sendDownloadEvent)
	if c.handleVirusScan(context.Background(), []byte(`{"type":"process","action":"start"}`)) {
		t.Fatal("consumed unrelated command")
	}
	if !c.handleVirusScan(context.Background(), []byte(`{"type":"virus_scan","request_id":"scan-1","scan_type":"full"}`)) {
		t.Fatal("not routed")
	}
	select {
	case msg := <-c.pendingResults:
		r := msg.(antivirus.Result)
		if r.Type != "virus_scan_result" || r.Status != "completed" || r.RequestID != "scan-1" {
			t.Fatalf("%+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("final result not queued")
	}
	c.handleVirusScan(context.Background(), []byte(`{"type":"virus_scan","request_id":"bad","scan_type":123}`))
	if r := (<-c.pendingResults).(antivirus.Result); r.Status != "rejected" {
		t.Fatalf("%+v", r)
	}
}
