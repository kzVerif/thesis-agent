//go:build windows

package servicehost

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"log"
	"time"
)

func IsService() (bool, error) { return svc.IsWindowsService() }
func RequireAdministrator() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return fmt.Errorf("administrative provisioning requires an elevated Administrator terminal")
	}
	return nil
}
func Run(runtime Runtime) error { return svc.Run(Name, &handler{runtime: runtime}) }

type handler struct{ runtime Runtime }

func reportEvent(message string) {
	if events, err := eventlog.Open(Name); err == nil {
		defer events.Close()
		_ = events.Error(1, message)
	}
}

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status := svc.Status{State: svc.StartPending, WaitHint: 10000, CheckPoint: 1}
	statuses <- status
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- h.runtime(ctx, func() { close(ready) }) }()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	startupDeadline := time.NewTimer(30 * time.Second)
	defer startupDeadline.Stop()
	var stopDeadline <-chan time.Time
	var stopTimer *time.Timer
	defer func() {
		if stopTimer != nil {
			stopTimer.Stop()
		}
	}()
	stopping := false
	beginStop := func() {
		if stopping {
			return
		}
		stopping = true
		log.Printf("service stopping; shutdown requested")
		status = svc.Status{State: svc.StopPending, WaitHint: 20000, CheckPoint: 1}
		statuses <- status
		cancel()
		stopTimer = time.NewTimer(20 * time.Second)
		stopDeadline = stopTimer.C
	}
	for {
		select {
		case <-ready:
			ready = nil
			startupDeadline.Stop()
			if !stopping {
				status = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
				statuses <- status
				log.Printf("service running; application connectivity is reported separately")
			}
		case request, ok := <-requests:
			if !ok {
				beginStop()
				requests = nil
				continue
			}
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- status
			case svc.Stop, svc.Shutdown:
				beginStop()
			}
		case <-startupDeadline.C:
			reportEvent("Local Agent initialization exceeded 30 seconds; stopping. Check protected runtime paths and logs.")
			beginStop()
		case <-tick.C:
			if status.State == svc.StartPending || status.State == svc.StopPending {
				status.CheckPoint++
				statuses <- status
			}
		case err := <-done:
			if err != nil {
				reportEvent("Agent startup/runtime failed. Inspect the protected Agent log; repair configuration/provisioning before restarting.")
				return true, 1
			}
			return false, 0
		case <-stopDeadline:
			// Last-resort bound for a stuck OS call. Returning lets the dedicated
			// service process exit. Normal Stop cooperatively joins runtime workers.
			reportEvent("Agent shutdown exceeded 20 seconds; service process will exit. Inspect lifecycle diagnostics.")
			return true, 2
		}
	}
}
