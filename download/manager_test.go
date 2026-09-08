package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func checksum(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func commandFor(server *httptest.Server, data []byte) Command {
	return Command{Type: "DOWNLOAD_FILE", JobID: "8b66e1fd-987f-4cb3-a316-a00c5dc47b7c", FileID: "2f07cc9a-9cc7-4ff3-8b40-c38965a25bd9", Filename: "example.bin", Size: int64(len(data)), SHA256: checksum(data), DownloadURL: server.URL, ExpiresAt: time.Now().Add(time.Hour)}
}
func newTestManager(t *testing.T, cfg Config) (*Manager, <-chan any) {
	t.Helper()
	events := make(chan any, 64)
	cfg.Directory = t.TempDir()
	cfg.AllowHTTP = true
	cfg.ProgressInterval = time.Millisecond
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = 1
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 4
	}
	m, err := NewManager(cfg, "agent-1", func(v any) error { events <- v; return nil })
	if err != nil {
		t.Fatal(err)
	}
	return m, events
}
func awaitResult(t *testing.T, events <-chan any) Result {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if result, ok := event.(Result); ok {
				return result
			}
		case <-timer.C:
			t.Fatal("timed out waiting for result")
		}
	}
}

func TestDownloadSHA256Success(t *testing.T) {
	data := []byte("verified payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server, data)
	m.Submit(context.Background(), cmd)
	result := awaitResult(t, events)
	if result.Status != "COMPLETED" || result.SHA256 != checksum(data) {
		t.Fatalf("unexpected result: %+v", result)
	}
	got, err := os.ReadFile(filepath.Join(m.cfg.Directory, cmd.Filename))
	if err != nil || string(got) != string(data) {
		t.Fatalf("downloaded file = %q, %v", got, err)
	}
}
func TestDownloadSHA256Mismatch(t *testing.T) {
	data := []byte("payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server, data)
	cmd.SHA256 = checksum([]byte("different"))
	m.Submit(context.Background(), cmd)
	result := awaitResult(t, events)
	if result.ErrorCode != string(HashMismatch) {
		t.Fatalf("result = %+v", result)
	}
	assertNoPart(t, m, cmd)
}
func TestDownloadSizeMismatch(t *testing.T) {
	data := []byte("small")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server, data)
	cmd.Size++
	m.Submit(context.Background(), cmd)
	result := awaitResult(t, events)
	if result.ErrorCode != string(SizeMismatch) {
		t.Fatalf("result = %+v", result)
	}
	assertNoPart(t, m, cmd)
}
func TestURLExpired(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server, []byte("x"))
	cmd.ExpiresAt = time.Now().Add(-time.Second)
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.ErrorCode != string(URLExpired) {
		t.Fatalf("result = %+v", result)
	}
}
func TestPathTraversalFilename(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server, []byte("x"))
	cmd.Filename = `..\evil.bin`
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.ErrorCode != string(InvalidCommand) {
		t.Fatalf("result = %+v", result)
	}
}
func TestDownloadInterrupted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "20")
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server, make([]byte, 20))
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.ErrorCode != string(DownloadFailed) {
		t.Fatalf("result = %+v", result)
	}
	assertNoPart(t, m, cmd)
}
func TestDuplicateJob(t *testing.T) {
	release := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); <-release; _, _ = w.Write([]byte("x")) }))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server, []byte("x"))
	m.Submit(context.Background(), cmd)
	m.Submit(context.Background(), cmd)
	close(release)
	_ = awaitResult(t, events)
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}
func TestHTTPStatusNotOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusForbidden) }))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server, []byte("x"))
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.ErrorCode != string(HTTPError) {
		t.Fatalf("result = %+v", result)
	}
}
func TestInsufficientDiskSpace(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	m, events := newTestManager(t, Config{SpaceChecker: func(string) (uint64, error) { return 0, nil }})
	cmd := commandFor(server, []byte("x"))
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.ErrorCode != string(InsufficientDiskSpace) {
		t.Fatalf("result = %+v", result)
	}
}
func TestTemporaryFileCleanup(t *testing.T) {
	data := []byte("payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server, data)
	cmd.SHA256 = checksum([]byte("wrong"))
	m.Submit(context.Background(), cmd)
	_ = awaitResult(t, events)
	assertNoPart(t, m, cmd)
}

func TestDownloadSendsAgentHeader(t *testing.T) {
	data := []byte("header payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Agent-ID"); got != "agent-1" {
			t.Errorf("X-Agent-ID = %q", got)
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	m.Submit(context.Background(), commandFor(server, data))
	if result := awaitResult(t, events); result.Status != "COMPLETED" {
		t.Fatalf("result = %+v", result)
	}
}

func TestHTTPPartialContentAccepted(t *testing.T) {
	data := []byte("partial response")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data)
	}))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	m.Submit(context.Background(), commandFor(server, data))
	if result := awaitResult(t, events); result.Status != "COMPLETED" {
		t.Fatalf("result = %+v", result)
	}
}

func TestServerErrorRetries(t *testing.T) {
	data := []byte("retried response")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) < 3 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	m, events := newTestManager(t, Config{})
	m.Submit(context.Background(), commandFor(server, data))
	if result := awaitResult(t, events); result.Status != "COMPLETED" {
		t.Fatalf("result = %+v", result)
	}
	if requests.Load() != 3 {
		t.Fatalf("requests = %d, want 3", requests.Load())
	}
}

func TestSameJobWithNewURLIsNewAttempt(t *testing.T) {
	data := []byte("new attempt")
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server2.Close()
	m, events := newTestManager(t, Config{})
	cmd := commandFor(server1, data)
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.Status != "COMPLETED" {
		t.Fatalf("first result = %+v", result)
	}
	if err := os.Remove(filepath.Join(m.cfg.Directory, cmd.Filename)); err != nil {
		t.Fatal(err)
	}
	cmd.DownloadURL = server2.URL
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.Status != "COMPLETED" {
		t.Fatalf("second result = %+v", result)
	}
}

func TestHTTPIsRestrictedToLoopback(t *testing.T) {
	m, events := newTestManager(t, Config{AllowHTTP: true})
	cmd := Command{Type: "DOWNLOAD_FILE", JobID: "8b66e1fd-987f-4cb3-a316-a00c5dc47b7c", FileID: "2f07cc9a-9cc7-4ff3-8b40-c38965a25bd9", Filename: "example.bin", Size: 1, SHA256: checksum([]byte("x")), DownloadURL: "http://example.com/file", ExpiresAt: time.Now().Add(time.Hour)}
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.ErrorCode != string(InvalidCommand) {
		t.Fatalf("result = %+v", result)
	}
}
func assertNoPart(t *testing.T, m *Manager, cmd Command) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(m.cfg.Directory, "."+cmd.Filename+"-*.part"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files still exist: %v (%v)", matches, err)
	}
}
