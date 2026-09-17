package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"ws-agent/antivirus"
	"ws-agent/download"
)

const powerRequestID = "2FC4DEE1-73B3-425B-9D36-E7B167CC8B74"

type fakePowerController struct {
	calls  atomic.Int32
	err    error
	called chan struct{}
}

func (f *fakePowerController) Shutdown(context.Context) error {
	f.calls.Add(1)
	if f.called != nil {
		f.called <- struct{}{}
	}
	return f.err
}

func TestParsePowerCommand(t *testing.T) {
	tests := []struct {
		name string
		data string
		ok   bool
	}{
		{"valid", `{"type":"power","action":"shutdown","request_id":"` + powerRequestID + `"}`, true},
		{"lowercase UUID", `{"type":"power","action":"shutdown","request_id":"` + strings.ToLower(powerRequestID) + `"}`, true},
		{"unrelated type", `{"type":"process","action":"shutdown","request_id":"` + powerRequestID + `"}`, false},
		{"wrong type case", `{"type":"POWER","action":"shutdown","request_id":"` + powerRequestID + `"}`, false},
		{"wrong action case", `{"type":"power","action":"Shutdown","request_id":"` + powerRequestID + `"}`, false},
		{"missing ID", `{"type":"power","action":"shutdown"}`, false},
		{"empty ID", `{"type":"power","action":"shutdown","request_id":""}`, false},
		{"null ID", `{"type":"power","action":"shutdown","request_id":null}`, false},
		{"numeric ID", `{"type":"power","action":"shutdown","request_id":123}`, false},
		{"invalid UUID", `{"type":"power","action":"shutdown","request_id":"ZFC4DEE1-73B3-425B-9D36-E7B167CC8B74"}`, false},
		{"compact UUID", `{"type":"power","action":"shutdown","request_id":"2fc4dee173b3425b9d36e7b167cc8b74"}`, false},
		{"padded UUID", `{"type":"power","action":"shutdown","request_id":" ` + powerRequestID + ` "}`, false},
		{"braced UUID", `{"type":"power","action":"shutdown","request_id":"{` + powerRequestID + `}"}`, false},
		{"URN UUID", `{"type":"power","action":"shutdown","request_id":"urn:uuid:` + powerRequestID + `"}`, false},
		{"missing action", `{"type":"power","request_id":"` + powerRequestID + `"}`, false},
		{"malformed", `{"type":"power","action":"shutdown",`, false},
		{"null", `null`, false},
		{"array", `[]`, false},
	}
	for _, action := range []string{"restart", "sleep", "lock", "shutdown_room", "unknown", "shutdown_result", "start_performance", "kill_process"} {
		tests = append(tests, struct {
			name string
			data string
			ok   bool
		}{action, fmt.Sprintf(`{"type":"power","action":%q,"request_id":%q}`, action, powerRequestID), false})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, ok := parsePowerCommand([]byte(tt.data))
			if ok != tt.ok {
				t.Fatalf("parsed=%v, want %v: %+v", ok, tt.ok, command)
			}
			if ok {
				var input map[string]any
				if err := json.Unmarshal([]byte(tt.data), &input); err != nil {
					t.Fatal(err)
				}
				if command.RequestID != input["request_id"] {
					t.Fatalf("request ID changed: %q", command.RequestID)
				}
			}
		})
	}
}

// localWebSocket supplies a real connection without contacting the configured server.
func localWebSocket(t *testing.T) (string, <-chan *websocket.Conn) {
	t.Helper()
	connections := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept WebSocket: %v", err)
			return
		}
		connections <- conn
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), connections
}

func readPowerTestJSON(t *testing.T, ctx context.Context, conn *websocket.Conn) map[string]any {
	t.Helper()
	typ, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("expected JSON text, got %v", typ)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertPowerResult(t *testing.T, result map[string]any, success bool, message string) {
	t.Helper()
	want := map[string]any{
		"type": "power", "action": "shutdown_result", "request_id": powerRequestID,
		"success": success, "mode": "mock", "message": message,
	}
	if len(result) != len(want) {
		t.Fatalf("unexpected result fields (agent_id must be absent): %#v", result)
	}
	for key, value := range want {
		if result[key] != value {
			t.Fatalf("%s = %#v, want %#v", key, result[key], value)
		}
	}
	if len(result["message"].(string)) > 4096 {
		t.Fatal("message exceeds protocol byte limit")
	}
}

func TestHandlePowerCommand(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		missing bool
		message string
	}{
		{"success", nil, false, "shutdown command accepted"},
		{"error stays private", errors.New(strings.Repeat(`private C:\internal\secret: stack trace; `, 200)), false, "shutdown command failed"},
		{"no controller", nil, true, "power controller unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			url, connections := localWebSocket(t)
			raw, _, err := websocket.Dial(ctx, url, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.CloseNow()
			server := <-connections
			defer server.CloseNow()
			c := New("", time.Second, time.Second, time.Second)
			controller := &fakePowerController{err: tt.err}
			if !tt.missing {
				c.ConfigurePower(controller)
			}
			c.handlePowerCommand(ctx, &safeConnection{conn: raw}, powerCommand{RequestID: powerRequestID})
			assertPowerResult(t, readPowerTestJSON(t, ctx, server), tt.err == nil && !tt.missing, tt.message)
			wantCalls := int32(1)
			if tt.missing {
				wantCalls = 0
			}
			if controller.calls.Load() != wantCalls {
				t.Fatalf("Shutdown calls = %d, want %d", controller.calls.Load(), wantCalls)
			}
			if len(c.pendingResults) != 0 {
				t.Fatal("power result entered reconnect queue")
			}
		})
	}
}

func TestPowerResultUsesWriteMutex(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url, connections := localWebSocket(t)
	raw, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.CloseNow()
	server := <-connections
	defer server.CloseNow()
	c := New("", time.Second, time.Second, time.Second)
	called := make(chan struct{}, 1)
	c.ConfigurePower(&fakePowerController{called: called})
	conn := &safeConnection{conn: raw}
	conn.writeMu.Lock()
	done := make(chan struct{})
	go func() {
		c.handlePowerCommand(ctx, conn, powerCommand{RequestID: powerRequestID})
		close(done)
	}()
	select {
	case <-called:
	case <-ctx.Done():
		conn.writeMu.Unlock()
		t.Fatal(ctx.Err())
	}
	select {
	case <-done:
		conn.writeMu.Unlock()
		t.Fatal("power result bypassed the write mutex")
	case <-time.After(30 * time.Millisecond):
		conn.writeMu.Unlock()
	}
	assertPowerResult(t, readPowerTestJSON(t, ctx, server), true, "shutdown command accepted")
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestPowerResultIsNotReplayedOnReplacementConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url, connections := localWebSocket(t)
	original, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	originalServer := <-connections
	defer originalServer.CloseNow()
	original.CloseNow()
	replacement, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.CloseNow()
	replacementServer := <-connections
	defer replacementServer.CloseNow()
	c := New("", time.Second, time.Second, time.Second)
	c.ConfigurePower(&fakePowerController{})
	c.connection = &safeConnection{conn: replacement}
	c.handlePowerCommand(ctx, &safeConnection{conn: original}, powerCommand{RequestID: powerRequestID})
	if len(c.pendingResults) != 0 {
		t.Fatal("failed power result entered reconnect queue")
	}
	if err := writeJSON(ctx, c.connection, map[string]string{"type": "barrier"}); err != nil {
		t.Fatal(err)
	}
	if result := readPowerTestJSON(t, ctx, replacementServer); result["type"] != "barrier" {
		t.Fatalf("power result sent to replacement connection: %#v", result)
	}
}

func TestPowerCancelledContextDoesNotCallController(t *testing.T) {
	controller := &fakePowerController{}
	c := New("", time.Second, time.Second, time.Second)
	c.ConfigurePower(controller)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.handlePowerCommand(ctx, nil, powerCommand{RequestID: powerRequestID})
	if controller.calls.Load() != 0 {
		t.Fatal("Shutdown called after connection cancellation")
	}
}

func TestPowerRoutingWithExistingFeatures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url, connections := localWebSocket(t)
	c := New(url, 20*time.Millisecond, time.Hour, time.Second)
	controller := &fakePowerController{}
	c.ConfigurePower(controller)
	c.scanManager = antivirus.NewManager(func(context.Context, antivirus.Command) (antivirus.Report, error) {
		return antivirus.Report{}, nil
	}, c.sendDownloadEvent)
	if err := c.ConfigureDownloads(download.Config{Directory: t.TempDir()}, "test-agent"); err != nil {
		t.Fatal(err)
	}
	var kills atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- c.runConnection(ctx, map[string]string{"type": "registration"},
			func() (any, error) { return map[string]string{"type": "performance"}, nil },
			func() (any, error) { return map[string]string{"type": "process"}, nil },
			func(pid int32) error {
				if pid != 1234 {
					return fmt.Errorf("unexpected PID %d", pid)
				}
				kills.Add(1)
				return nil
			}, func() ([]byte, error) { return []byte("fake JPEG"), nil })
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("connection did not stop")
		}
	})
	var server *websocket.Conn
	select {
	case server = <-connections:
	case err := <-done:
		t.Fatalf("connection failed: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	defer server.CloseNow()
	if result := readPowerTestJSON(t, ctx, server); result["type"] != "registration" {
		t.Fatalf("registration changed: %#v", result)
	}
	send := func(data string) {
		t.Helper()
		if err := server.Write(ctx, websocket.MessageText, []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	// Invalid power messages must neither call the controller nor fall through
	// to permissive legacy stream/process aliases. A valid command is the barrier.
	for _, action := range []string{"restart", "sleep", "lock", "shutdown_room", "unknown", "start_performance", "start_process", "start_screen", "kill_process"} {
		send(fmt.Sprintf(`{"type":"power","action":%q,"command":"kill_process","pid":1234,"request_id":%q}`, action, powerRequestID))
	}
	send(`{"type":"power","action":"shutdown"}`)
	send(`{"type":"power","action":"shutdown","request_id":123}`)
	send(`{"type":"power","action":"shutdown","request_id":"invalid"}`)
	send(`{"type":"power","action":"shutdown",`)
	valid := fmt.Sprintf(`{"type":"power","action":"shutdown","request_id":%q}`, powerRequestID)
	send(valid)
	assertPowerResult(t, readPowerTestJSON(t, ctx, server), true, "shutdown command accepted")
	if controller.calls.Load() != 1 || kills.Load() != 0 {
		t.Fatalf("unexpected calls: power=%d kill=%d", controller.calls.Load(), kills.Load())
	}
	// Repeated IDs are handled per message; duplicate tracking belongs to server.
	send(valid)
	send(`{"type":"performance","action":"start"}`)
	send(`{"type":"process","action":"start"}`)
	send(`{"type":"process","action":"kill","pid":1234}`)
	send(`{"type":"virus_scan","request_id":"scan-1","scan_type":"quick"}`)
	send(`{"type":"DOWNLOAD_FILE","job_id":"invalid-download"}`)
	send(`{"type":"screen","action":"start"}`)
	seen := map[string]bool{}
	for len(seen) < 7 {
		typ, data, err := server.Read(ctx)
		if err != nil {
			t.Fatalf("read concurrent results (seen %v): %v", seen, err)
		}
		if typ == websocket.MessageBinary {
			if string(data) != "fake JPEG" {
				t.Fatalf("screen data changed: %q", data)
			}
			seen["screen"] = true
			continue
		}
		var result map[string]any
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		switch result["type"] {
		case "power":
			assertPowerResult(t, result, true, "shutdown command accepted")
			seen["power"] = true
		case "performance":
			seen["performance"] = true
		case "process":
			if result["action"] == "kill_result" {
				if result["success"] != true || result["pid"] != float64(1234) {
					t.Fatalf("kill result changed: %#v", result)
				}
				seen["kill"] = true
			} else {
				seen["process"] = true
			}
		case "virus_scan_result":
			if result["status"] != "completed" || result["request_id"] != "scan-1" {
				t.Fatalf("scan result changed: %#v", result)
			}
			seen["scan"] = true
		case "FILE_DOWNLOAD_RESULT":
			if result["error_code"] != "INVALID_COMMAND" || result["job_id"] != "invalid-download" {
				t.Fatalf("download rejection changed: %#v", result)
			}
			seen["download"] = true
		}
	}
	if controller.calls.Load() != 2 || kills.Load() != 1 {
		t.Fatalf("unexpected calls: power=%d kill=%d", controller.calls.Load(), kills.Load())
	}
}
