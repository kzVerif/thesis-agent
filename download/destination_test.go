package download

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ws-agent/internal/protectedpath"
)

func TestCustomDestination(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "preserves existing file on hash mismatch"}[mismatch], func(t *testing.T) {
			data := []byte("custom destination payload")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
			defer server.Close()
			directory := filepath.Join(t.TempDir(), "Shared Files", "Lessons")
			var checked string
			m, events := newTestManager(t, Config{SpaceChecker: func(path string) (uint64, error) { checked = path; return 1 << 30, nil }})
			cmd := commandFor(server, data)
			cmd.DestinationPath = directory
			if mismatch {
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, cmd.Filename), []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
				cmd.SHA256 = checksum([]byte("wrong"))
			}
			// Exercise the same JSON decoding used by the socket receiver.
			wire, err := json.Marshal(cmd)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Command
			if err := json.Unmarshal(wire, &decoded); err != nil {
				t.Fatal(err)
			}
			m.Submit(context.Background(), decoded)
			result := awaitResult(t, events)
			want := string(data)
			if mismatch {
				want = "original"
				if result.ErrorCode != "HASH_MISMATCH" {
					t.Fatalf("result = %+v", result)
				}
			} else if result.Status != "COMPLETED" || result.SHA256 != checksum(data) {
				t.Fatalf("result = %+v", result)
			}
			if checked != directory {
				t.Fatalf("space checked at %q", checked)
			}
			got, err := os.ReadFile(filepath.Join(directory, cmd.Filename))
			if err != nil || string(got) != want {
				t.Fatalf("file = %q, %v", got, err)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary files remain: %v, %v", entries, err)
			}
			entries, err = os.ReadDir(m.cfg.Directory)
			if err != nil || len(entries) != 0 {
				t.Fatalf("default directory changed: %v, %v", entries, err)
			}
		})
	}
}

func TestInvalidDestinationPaths(t *testing.T) {
	m, _ := newTestManager(t, Config{})
	paths := []string{"relative", ".", "..", `C:relative`, `\\server\share`, `\\?\C:\folder`, "//server/share", filepath.Join(t.TempDir(), "a") + string(os.PathSeparator) + "..", filepath.Join(t.TempDir(), "bad\npath")}
	if runtime.GOOS == "windows" {
		paths = append(paths, "/tmp/destination", `C:\folder.`, `C:\folder:stream`, `C:\NUL`)
	} else {
		paths = append(paths, `C:\folder`)
	}
	for _, path := range paths {
		if _, err := m.destinationDirectory(path); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
	if got, err := m.destinationDirectory(""); err != nil || got != m.cfg.Directory {
		t.Fatalf("default = %q, %v", got, err)
	}
}

func TestDestinationFailureDoesNotFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected HTTP request") }))
	defer server.Close()
	for _, invalid := range []bool{false, true} {
		m, events := newTestManager(t, Config{})
		cmd := commandFor(server, []byte("x"))
		cmd.DestinationPath = filepath.Join(t.TempDir(), "not-a-directory")
		want := "WRITE_FAILED"
		if invalid {
			cmd.DestinationPath = "relative-secret-path"
			want = "INVALID_DESTINATION_PATH"
		} else if err := os.WriteFile(cmd.DestinationPath, []byte("existing"), 0600); err != nil {
			t.Fatal(err)
		}
		m.Submit(context.Background(), cmd)
		result := awaitResult(t, events)
		if result.Status != "FAILED" || result.ErrorCode != want || strings.Contains(result.ErrorMessage, cmd.DestinationPath) {
			t.Fatalf("result = %+v", result)
		}
		entries, err := os.ReadDir(m.cfg.Directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("default directory changed: %v, %v", entries, err)
		}
	}
}

func TestCustomDestinationRejectsLinkAncestor(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	m, _ := newTestManager(t, Config{})
	if _, err := m.prepareDestination(Command{DestinationPath: filepath.Join(link, "child")}); err == nil {
		t.Fatal("followed link ancestor")
	}
}

func TestServiceCustomDestinationStaysInDownloadTree(t *testing.T) {
	root := t.TempDir()
	m := &Manager{cfg: Config{Directory: filepath.Join(root, "downloads"), Boundary: &protectedpath.Boundary{Root: root}}}
	if _, err := m.destinationDirectory(filepath.Join(root, "identity")); err == nil {
		t.Fatal("allowed write to sibling runtime data")
	}
	if _, err := m.destinationDirectory(filepath.Join(root, "downloads", "lessons")); err != nil {
		t.Fatal(err)
	}
}
