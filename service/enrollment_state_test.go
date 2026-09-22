package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"ws-agent/model"
)

func TestEnrollmentMarkerBinding(t *testing.T) {
	identity := testIdentity()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteEnrollment(path, identity, "http://example.test", true); err != nil {
		t.Fatal(err)
	}
	valid, err := ReadEnrollment(path, identity, "http://example.test/")
	if err != nil || !valid {
		t.Fatal("valid marker rejected")
	}
	other := identity
	other.AgentID = "22222222-2222-4222-8222-222222222222"
	if valid, _ := ReadEnrollment(path, other, "http://example.test"); valid {
		t.Fatal("marker applied to other identity")
	}
	other = identity
	other.PublicKey = "another key"
	if valid, _ := ReadEnrollment(path, other, "http://example.test"); valid {
		t.Fatal("marker applied to other key")
	}
	if valid, _ := ReadEnrollment(path, identity, "http://other.test"); valid {
		t.Fatal("marker applied to other API")
	}
}

func TestServiceEnrollmentStates(t *testing.T) {
	for _, status := range []int{204, 404, 401, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/exists") {
					t.Errorf("unexpected request %s", r.URL.Path)
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "state.json")
			identity := testIdentity()
			// Even a historical verified marker must not override a fresh 404.
			if status == 404 {
				if err := WriteEnrollment(path, identity, server.URL, true); err != nil {
					t.Fatal(err)
				}
			}
			stdin := os.Stdin
			os.Stdin = nil
			defer func() { os.Stdin = stdin }()
			_, err := EnsureEnrollment(context.Background(), server.URL, identity, path, false)
			if (err == nil) != (status == 204) {
				t.Fatalf("status=%d error=%v", status, err)
			}
			verified, readErr := ReadEnrollment(path, identity, server.URL)
			if readErr != nil || verified != (status == 204) {
				t.Fatalf("unexpected marker: %v %v", verified, readErr)
			}
		})
	}
}

func TestServiceRetriesAPIAndCancels(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "state.json")
	if _, err := EnsureEnrollment(ctx, server.URL, testIdentity(), path, false); err != nil {
		t.Fatal(err)
	}
	if calls.Load() < 2 {
		t.Fatal("no retry")
	}
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer unavailable.Close()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	path2 := filepath.Join(t.TempDir(), "state.json")
	if _, err := EnsureEnrollment(ctx2, unavailable.URL, testIdentity(), path2, false); err != context.DeadlineExceeded {
		t.Fatal(err)
	}
	if _, err := os.Stat(path2); !os.IsNotExist(err) {
		t.Fatal("unknown state incorrectly persisted as verified")
	}
}

func TestExistingRegistrationPayloadAndErrors(t *testing.T) {
	identity := testIdentity()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/agents/register" {
			t.Error("registration contract changed")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["agent_id"] != identity.AgentID || payload["public_key"] != identity.PublicKey || payload["token"] != "test-token" {
			t.Error("registration fields changed")
		}
		if _, exists := payload["encrypted_private_key"]; exists {
			t.Error("private material sent")
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]string{"id": identity.AgentID})
	}))
	defer server.Close()
	if err := registerAgent(context.Background(), server.URL, "test-token", identity, model.SystemInfo{ID: identity.AgentID}); err != nil {
		t.Fatal(err)
	}
	failure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, "sensitive-test-token")
	}))
	defer failure.Close()
	err := registerAgent(context.Background(), failure.URL, "sensitive-test-token", identity, model.SystemInfo{})
	if err == nil || strings.Contains(err.Error(), "sensitive-test-token") {
		t.Fatal("response body/credential leaked")
	}
}
