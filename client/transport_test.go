package client

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestTLSFailureReconnectsAndCancels(t *testing.T) {
	var attempts atomic.Int32
	connected := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		if !acceptTestAuth(t, r.Context(), conn) {
			return
		}
		var initial map[string]string
		if err := wsjson.Read(r.Context(), conn, &initial); err != nil {
			return
		}
		if initial["id"] != "same-test-identity" {
			t.Error("initial identity changed")
		}
		connected <- struct{}{}
		_, _, _ = conn.Read(r.Context())
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("simulated TLS endpoint outage")
		}
		return nil, nil
	}}
	server.StartTLS()
	defer server.Close()
	// Test-only server roots. Do not alter a machine trust store.
	before := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = before })
	c := New("wss"+strings.TrimPrefix(server.URL, "https"), time.Hour, time.Hour, time.Second)
	configureTestAuth(c)
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- c.Run(ctx, map[string]string{"id": "same-test-identity"}, nil, nil, nil, nil) }()
	select {
	case <-connected:
		if time.Since(start) < 450*time.Millisecond || attempts.Load() != 2 {
			t.Fatal("TLS failure did not use existing bounded backoff")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WSS reconnect did not recover")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WSS did not stop")
	}
}
