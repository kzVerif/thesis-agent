package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ws-agent/installedapps"
)

const appsTestID = "12345678-1234-4234-8234-123456789abc"

func TestInstalledAppsParsingAndStreamIsolation(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{`{"type":"installed_apps","action":"get","request_id":"` + appsTestID + `"}`, true},
		{`{"type":"installed_apps","action":"get","request_id":"bad"}`, false},
		{`{"type":"installed_apps","action":"start_process","request_id":"` + appsTestID + `"}`, false},
		{`{"type":"installed_apps","action":"get","request_id":12}`, false},
		{`{"type":"process","action":"get","request_id":"` + appsTestID + `"}`, false},
		{`{`, false},
	} {
		_, ok := parseInstalledAppsCommand([]byte(tc.raw))
		if ok != tc.valid {
			t.Fatal(tc.raw)
		}
	}
	for _, action := range []string{"get", "start_process", "stop_screen", "start_performance", "kill"} {
		command := parseStreamCommand([]byte(fmt.Sprintf(`{"type":"installed_apps","action":%q,"command":"start_process","pid":123}`, action)))
		if command.stream != streamUnknown {
			t.Fatal("inventory interpreted as stream", action)
		}
	}
}
func TestInstalledAppsWorkerBoundedAndCancelable(t *testing.T) {
	w := newInstalledAppsWorker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var active, maxActive atomic.Int32
	started := make(chan struct{}, 1)
	collect := func(ctx context.Context) ([]installedapps.App, error) {
		n := active.Add(1)
		maxActive.Store(n)
		defer active.Add(-1)
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); w.collect(ctx, collect) }()
	go func() { defer wg.Done(); w.write(ctx, func(context.Context, any) error { return nil }) }()
	w.submit(installedAppsCommand{RequestID: appsTestID})
	<-started
	for i := 0; i < 1000; i++ {
		w.submit(installedAppsCommand{RequestID: fmt.Sprint(i)})
	}
	w.mu.Lock()
	n := len(w.pending)
	w.mu.Unlock()
	if n > installedAppsQueueSize+1 || maxActive.Load() != 1 {
		t.Fatal("unbounded workers", n, maxActive.Load())
	}
	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker leaked")
	}
}
func TestInstalledAppsWorkerEmptyAndFailureWire(t *testing.T) {
	for _, failed := range []bool{false, true} {
		w := newInstalledAppsWorker()
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan []byte, 1)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			w.collect(ctx, func(context.Context) ([]installedapps.App, error) {
				if failed {
					return nil, fmt.Errorf("private registry path must not escape")
				}
				return nil, nil
			})
		}()
		go func() {
			defer wg.Done()
			w.write(ctx, func(_ context.Context, v any) error { data, _ := json.Marshal(v); result <- data; return nil })
		}()
		w.submit(installedAppsCommand{RequestID: appsTestID})
		select {
		case raw := <-result:
			var r map[string]any
			if json.Unmarshal(raw, &r) != nil {
				t.Fatal(string(raw))
			}
			if r["type"] != "installed_apps" || r["action"] != "result" || r["request_id"] != appsTestID || r["success"] != !failed {
				t.Fatal(r)
			}
			if failed {
				if r["error"] != "collection failed" {
					t.Fatal(r)
				}
			} else {
				apps, ok := r["apps"].([]any)
				if !ok || len(apps) != 0 {
					t.Fatal(r)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("missing result")
		}
		cancel()
		wg.Wait()
	}
}

func TestInstalledAppsExpiredQueuedJobDoesNotCollect(t *testing.T) {
	w := newInstalledAppsWorker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.jobs <- installedAppsCommand{RequestID: appsTestID, deadline: time.Now().Add(-time.Second)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.collect(ctx, func(context.Context) ([]installedapps.App, error) {
			t.Error("expired request collected")
			return nil, nil
		})
	}()
	select {
	case result := <-w.results:
		if result.Success || result.RequestID != appsTestID {
			t.Fatal(result)
		}
	case <-time.After(time.Second):
		t.Fatal("no expiry result")
	}
	cancel()
	<-done
}
