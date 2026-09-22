package client

import (
	"bytes"
	"context"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type lifecyclePower struct{ closed chan struct{} }

func (*lifecyclePower) Mode() string                   { return "mock" }
func (*lifecyclePower) Shutdown(context.Context) error { return nil }
func (p *lifecyclePower) Close()                       { close(p.closed) }

func TestStopCancelsPowerBeforeJoiningProvider(t *testing.T) {
	for _, mode := range []string{"context", "close"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				if !acceptTestAuth(t, r.Context(), conn) {
					return
				}
				var initial map[string]string
				if wsjson.Read(r.Context(), conn, &initial) != nil {
					return
				}
				_ = wsjson.Write(r.Context(), conn, map[string]string{"type": "performance", "action": "start"})
				_, _, _ = conn.Read(r.Context())
			}))
			defer server.Close()
			c := New("ws"+strings.TrimPrefix(server.URL, "http"), time.Hour, time.Hour, time.Second)
			configureTestAuth(c)
			power := &lifecyclePower{closed: make(chan struct{})}
			c.ConfigurePower(power)
			ctx, cancel := context.WithCancel(context.Background())
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			done := make(chan error, 1)
			go func() {
				done <- c.Run(ctx, map[string]string{"id": "lifecycle-test"}, func() (any, error) {
					close(entered)
					<-release
					return map[string]int{"cpu_usage": 0}, nil
				}, nil, nil, nil)
			}()
			defer func() { cancel(); unblock(); c.Close() }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("provider never started")
			}
			if mode == "context" {
				cancel()
			} else {
				c.Close()
			}
			select {
			case <-power.closed:
			case <-time.After(time.Second):
				t.Fatal("power cancellation waited for a blocked provider")
			}
			unblock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("client did not exit after provider cleanup")
			}
		})
	}
}

func TestReconnectKeepsInitialMessageAndCancels(t *testing.T) {
	var connections atomic.Int32
	second := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		if !acceptTestAuth(t, r.Context(), conn) {
			return
		}
		var message map[string]string
		if err := wsjson.Read(r.Context(), conn, &message); err != nil {
			return
		}
		if message["id"] != "stable-agent" {
			t.Error("identity changed on reconnect")
		}
		if connections.Add(1) >= 2 {
			select {
			case second <- struct{}{}:
			default:
			}
		}
	}))
	defer server.Close()
	c := New("ws"+strings.TrimPrefix(server.URL, "http"), time.Hour, time.Hour, time.Second)
	configureTestAuth(c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, map[string]string{"id": "stable-agent"}, nil, nil, nil, nil) }()
	select {
	case <-second:
	case <-time.After(4 * time.Second):
		t.Fatal("did not reconnect")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("reconnect wait did not cancel")
	}
}

func TestJSONLoggingDoesNotExposePayload(t *testing.T) {
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_ = wsjson.Write(r.Context(), conn, map[string]string{"type": "DOWNLOAD_FILE", "download_url": "https://example.test/?token=SECRET_PAYLOAD"})
		_, _, _ = conn.Read(r.Context())
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	conn := &safeConnection{conn: raw}
	err = readJSONMessages(ctx, conn, func([]byte) { cancel() }, func(streamCommand) {})
	raw.CloseNow()
	if err == nil {
		t.Fatal("expected cancelled read")
	}
	if strings.Contains(output.String(), "SECRET_PAYLOAD") || strings.Contains(output.String(), "download_url") {
		t.Fatal("payload leaked to logs")
	}
}
