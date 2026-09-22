package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadRejectsHTTPSDowngradeEvenWithLocalHTTPEnabled(t *testing.T) {
	var hits atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer sink.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	injected := server.Client()
	manager, err := NewManager(Config{Directory: t.TempDir(), AllowHTTP: true, HTTPClient: injected}, "test-agent", func(any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	response, err := manager.doRequest(context.Background(), Command{DownloadURL: server.URL + "/opaque-test-token", ExpiresAt: time.Now().Add(time.Minute)})
	if response != nil {
		response.Body.Close()
	}
	if err == nil || hits.Load() != 0 {
		t.Fatal("download followed insecure redirect")
	}
	if injected.CheckRedirect != nil {
		t.Fatal("caller HTTP client mutated")
	}
}
