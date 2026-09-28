//go:build windows

package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"ws-agent/internal/protectedpath"
)

func externalManager(t *testing.T, defaultRoot string, roots []string, runtimeRoot string) (*Manager, <-chan any) {
	t.Helper()
	events := make(chan any, 64)
	m, err := NewManager(Config{Directory: defaultRoot, AllowedRoots: roots, AllowHTTP: true, Boundary: &protectedpath.Boundary{Root: runtimeRoot}}, "agent-1", func(event any) error { events <- event; return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m, events
}

func TestExternalServiceDownloads(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "custom"}[custom], func(t *testing.T) {
			base := t.TempDir()
			first, second := filepath.Join(base, "new", "downloads"), filepath.Join(base, "lessons")
			m, events := externalManager(t, first, []string{first, second}, filepath.Join(base, "runtime"))
			data := []byte("external verified download")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
			defer server.Close()
			cmd := commandFor(server, data)
			directory := first
			if custom {
				directory = filepath.Join(second, "Room101")
				cmd.DestinationPath = directory
			}
			m.Submit(context.Background(), cmd)
			if result := awaitResult(t, events); result.Status != "COMPLETED" {
				t.Fatalf("result: %+v", result)
			}
			m.Close()
			got, err := os.ReadFile(filepath.Join(directory, cmd.Filename))
			if err != nil || string(got) != string(data) {
				t.Fatalf("file %q: %v", got, err)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary files remain: %v %v", entries, err)
			}
		})
	}
}

func TestExternalDestinationRejectsOutsideAndHardLinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "downloads")
	m, events := externalManager(t, root, []string{root}, filepath.Join(base, "runtime"))
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("invalid destination contacted server") }))
	defer server.Close()
	cmd := commandFor(server, []byte("data"))
	cmd.DestinationPath = root + "-sibling"
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.ErrorCode != "INVALID_DESTINATION_PATH" {
		t.Fatalf("result: %+v", result)
	}
	outside := filepath.Join(base, "outside.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(root, cmd.Filename)); err != nil {
		t.Fatal(err)
	}
	cmd.JobID = "601f18aa-d8ec-4439-9d3b-ed1037788a18"
	cmd.DestinationPath = root
	m.Submit(context.Background(), cmd)
	if result := awaitResult(t, events); result.ErrorCode != "WRITE_FAILED" {
		t.Fatalf("result: %+v", result)
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "keep" {
		t.Fatal("outside file changed", err)
	}
}

func TestExternalDownloadPinsDirectory(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "downloads")
	m, events := externalManager(t, root, []string{root}, filepath.Join(base, "runtime"))
	data := []byte("pinned download")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := os.Rename(root, root+"-moved"); err == nil {
			t.Error("active download directory could be replaced")
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	m.Submit(context.Background(), commandFor(server, data))
	if result := awaitResult(t, events); result.Status != "COMPLETED" {
		t.Fatalf("result: %+v", result)
	}
}

func TestExternalRootRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "link")
	outside := t.TempDir()
	if err := os.Symlink(outside, root); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	m, err := NewManager(Config{Directory: root, AllowedRoots: []string{root}, Boundary: &protectedpath.Boundary{Root: filepath.Join(base, "runtime")}}, "agent-1", func(any) error { return nil })
	if err == nil {
		m.Close()
		t.Fatal("accepted linked root")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("wrote outside root")
	}
}
