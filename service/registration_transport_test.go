package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"ws-agent/model"
)

func TestEnrollmentOverHTTPSAndDowngradeRefusal(t *testing.T) {
	var leaked atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer sink.Close()
	identity := testIdentity()
	var redirect atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirect.Load() {
			http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/exists") {
			w.WriteHeader(204)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["agent_id"] != identity.AgentID || payload["public_key"] != identity.PublicKey || payload["token"] != "test-only" {
			t.Error("registration contract changed")
		}
		if _, ok := payload["encrypted_private_key"]; ok {
			t.Error("private key leaked")
		}
		_ = json.NewEncoder(w).Encode(registerResponse{ID: identity.AgentID})
	}))
	defer server.Close()
	// Test-local roots only; production still uses the platform default transport.
	before := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = before })
	status, err := agentExists(context.Background(), server.URL, identity.AgentID)
	if err != nil || status != 204 {
		t.Fatalf("HTTPS existence check: %v", err)
	}
	if err := registerAgent(context.Background(), server.URL, "test-only", identity, model.SystemInfo{}); err != nil {
		t.Fatal(err)
	}
	redirect.Store(true)
	if _, err := agentExists(context.Background(), server.URL, identity.AgentID); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("existence downgrade accepted: %v", err)
	}
	if err := registerAgent(context.Background(), server.URL, "test-only", identity, model.SystemInfo{}); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("registration downgrade accepted: %v", err)
	}
	if leaked.Load() != 0 {
		t.Fatal("token/request reached insecure destination")
	}
}
