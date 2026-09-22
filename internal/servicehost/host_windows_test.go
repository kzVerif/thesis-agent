//go:build windows

package servicehost

import (
	"context"
	"golang.org/x/sys/windows/svc"
	"testing"
	"time"
)

func TestSCMLifecycleWithoutSCM(t *testing.T) {
	for _, command := range []svc.Cmd{svc.Stop, svc.Shutdown} {
		requests := make(chan svc.ChangeRequest, 4)
		statuses := make(chan svc.Status, 16)
		finished := make(chan uint32, 1)
		cleaned := make(chan struct{})
		h := handler{runtime: func(ctx context.Context, ready func()) error {
			ready()
			<-ctx.Done()
			close(cleaned)
			return nil
		}}
		go func() { _, code := h.Execute(nil, requests, statuses); finished <- code }()
		waitState := func(expected svc.State) {
			t.Helper()
			select {
			case state := <-statuses:
				if state.State != expected {
					t.Fatalf("state=%v expected=%v", state.State, expected)
				}
			case <-time.After(time.Second):
				t.Fatal("missing SCM state")
			}
		}
		waitState(svc.StartPending)
		waitState(svc.Running)
		requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
		waitState(svc.Running)
		requests <- svc.ChangeRequest{Cmd: command}
		waitState(svc.StopPending)
		select {
		case code := <-finished:
			if code != 0 {
				t.Fatal("intentional stop returned failure")
			}
		case <-time.After(time.Second):
			t.Fatal("SCM stop blocked")
		}
		select {
		case <-cleaned:
		default:
			t.Fatal("runtime was not joined")
		}
	}
}
