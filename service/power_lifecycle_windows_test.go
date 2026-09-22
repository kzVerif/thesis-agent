//go:build windows

package service

import (
	"context"
	"testing"
	"time"
)

func TestPowerCloseCancelsPendingTimer(t *testing.T) {
	// A one-hour delay ensures this test never executes shutdown.exe.
	controller := &WindowsPowerController{Delay: time.Hour}
	if err := controller.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { controller.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("power timer was not cancelled")
	}
	if err := controller.Shutdown(context.Background()); err == nil {
		t.Fatal("stopped controller accepted work")
	}
}
