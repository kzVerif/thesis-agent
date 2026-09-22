package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"ws-agent/internal/agentauth"
	"ws-agent/internal/apppaths"
	"ws-agent/service"
)

func runtimeFixture(t *testing.T) (apppaths.Paths, []byte) {
	t.Helper()
	t.Setenv("TRANSPORT_MODE", "development")
	t.Setenv("ALLOW_LOCAL_HTTP_DOWNLOADS", "false")
	paths, err := apppaths.Console(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := service.AgentConfig{AgentID: "11111111-1111-4111-8111-111111111111", Algorithm: "Ed25519", PublicKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), EncryptedPrivateKey: base64.StdEncoding.EncodeToString([]byte("opaque"))}
	b, _ := json.Marshal(identity)
	if err := os.WriteFile(paths.Identity, b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_LOG_PATH", "")
	t.Setenv("DOWNLOAD_DIRECTORY", "")
	t.Setenv("POWER_MODE", "mock")
	return paths, b
}

func TestServiceRuntimeReadyDuringOutageAndStops(t *testing.T) {
	paths, before := runtimeFixture(t)
	requested := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requested <- struct{}{}:
		default:
		}
		w.WriteHeader(503)
	}))
	defer server.Close()
	t.Setenv("AGENT_API_URL", server.URL)
	t.Setenv("WS_SERVER_URL", "ws://127.0.0.1:1/ws")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- runWithPaths(ctx, Options{Service: true}, paths, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("not locally ready")
	}
	select {
	case <-requested:
	case <-time.After(3 * time.Second):
		t.Fatal("API not attempted")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runtime failed to stop")
	}
	after, _ := os.ReadFile(paths.Identity)
	if string(after) != string(before) {
		t.Fatal("identity changed")
	}
	if _, err := os.Stat(paths.Enrollment); !os.IsNotExist(err) {
		t.Fatal("unknown enrollment incorrectly verified")
	}
}

func TestServiceLegacyIdentityFailsAuthenticationAndStops(t *testing.T) {
	paths, before := runtimeFixture(t)
	initial := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/exists") {
			w.WriteHeader(204)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var message map[string]any
		if err := wsjson.Read(ctx, conn, &message); err != nil {
			return
		}
		initial <- message
		challengeID, _ := agentauth.Random()
		nonce, _ := agentauth.Random()
		_ = wsjson.Write(ctx, conn, agentauth.Challenge{Type: "auth_challenge", Version: agentauth.Version, ChallengeID: challengeID, ServerNonce: nonce})
		// The authoritative loader rejects this legacy fixture. No proof, metadata
		// or privileged traffic may be sent, and the Service must still stop cleanly.
		if _, _, err := conn.Read(ctx); err != nil {
			initial <- nil
		} else {
			t.Error("legacy runtime sent traffic instead of failing closed")
		}
	}))
	defer server.Close()
	t.Setenv("AGENT_API_URL", server.URL)
	t.Setenv("WS_SERVER_URL", "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runWithPaths(ctx, Options{Service: true}, paths, nil) }()
	for i := 0; i < 2; i++ {
		select {
		case message := <-initial:
			if i == 0 && (len(message) != 4 || message["type"] != "auth_hello" || message["agent_id"] != "11111111-1111-4111-8111-111111111111") {
				t.Fatalf("authentication hello invalid: keys=%d", len(message))
			}
		case err := <-done:
			t.Fatalf("runtime exited: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("legacy authentication did not fail closed promptly")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runtime failed to stop")
	}
	after, _ := os.ReadFile(paths.Identity)
	if string(after) != string(before) {
		t.Fatal("identity changed")
	}
}

func TestServiceLocalConfigFailsBeforeReady(t *testing.T) {
	paths, before := runtimeFixture(t)
	t.Setenv("AGENT_API_URL", "http://127.0.0.1:1")
	t.Setenv("WS_SERVER_URL", "ws://127.0.0.1:1/ws")
	t.Setenv("AGENT_LOG_PATH", paths.Identity)
	ready := false
	if err := runWithPaths(context.Background(), Options{Service: true}, paths, func() { ready = true }); err == nil || ready {
		t.Fatal("unsafe logging path accepted")
	}
	after, _ := os.ReadFile(paths.Identity)
	if string(after) != string(before) {
		t.Fatal("log initialization damaged identity")
	}
}

func TestProductionPolicyFailsBeforeIdentityChanges(t *testing.T) {
	paths, before := runtimeFixture(t)
	t.Setenv("TRANSPORT_MODE", "")
	t.Setenv("AGENT_API_URL", "http://127.0.0.1:8080")
	t.Setenv("WS_SERVER_URL", "ws://127.0.0.1:8081/ws")
	ready := false
	err := runWithPaths(context.Background(), Options{Service: true}, paths, func() { ready = true })
	if err == nil || !strings.Contains(err.Error(), "must use https") || ready {
		t.Fatalf("production config not rejected before ready: %v", err)
	}
	after, _ := os.ReadFile(paths.Identity)
	if string(after) != string(before) {
		t.Fatal("identity modified")
	}
}
