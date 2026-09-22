package service

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogRotationBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	writer, err := newRotatingLog(path, 64, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if _, err = fmt.Fprintf(writer, "%02d %s\n", i, strings.Repeat("x", 25)); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(path + "*")
	if len(files) != 3 {
		t.Fatalf("expected current + 2 backups, got %v", files)
	}
	for _, name := range files {
		info, _ := os.Stat(name)
		if info.Size() > 64 {
			t.Fatal("unbounded log")
		}
	}
	b, _ := os.ReadFile(path)
	if !bytes.Contains(b, []byte("19")) {
		t.Fatal("latest log missing")
	}
}

func TestConsolePreservesFileLogging(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "agent.log")
	closeLogs, err := InitLoggingAt(path, true)
	if err != nil {
		t.Fatal(err)
	}
	log.Print("console-and-file-test")
	closeLogs()
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(b, []byte("console-and-file-test")) {
		t.Fatal("file logging lost")
	}
}
