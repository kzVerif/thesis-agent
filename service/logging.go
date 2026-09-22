package service

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// InitLogging writes diagnostic output to a file so the Agent can run without
// a visible UI. Registration prompts still use the console on first run.
func InitLogging() (func(), error) {
	path := os.Getenv("AGENT_LOG_PATH")
	if path == "" {
		path = filepath.Join("logs", "agent.log")
	}
	return InitLoggingAt(path, true)
}

// InitLoggingAt retains file logging in both hosts. Console additionally mirrors
// to stderr. Call the returned close only after all runtime workers have stopped.
func InitLoggingAt(path string, console bool) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := newRotatingLog(path, 5<<20, 5)
	if err != nil {
		return nil, err
	}
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lmicroseconds)
	previous := log.Writer()
	var output io.Writer = file
	if console {
		output = io.MultiWriter(os.Stderr, file)
	}
	log.SetOutput(output)
	log.Printf("agent starting")
	return func() {
		log.Printf("agent stopping")
		log.SetOutput(previous)
		_ = file.Close()
	}, nil
}

type rotatingLog struct {
	mu          sync.Mutex
	path        string
	limit, size int64
	backups     int
	file        *os.File
}

func newRotatingLog(path string, limit int64, backups int) (*rotatingLog, error) {
	w := &rotatingLog{path: path, limit: limit, backups: backups}
	err := w.open()
	return w, err
}

func (w *rotatingLog) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file, w.size = f, info.Size()
	return nil
}

func (w *rotatingLog) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	for i := w.backups; i >= 1; i-- {
		dest := fmt.Sprintf("%s.%d", w.path, i)
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			return err
		}
		source := w.path
		if i > 1 {
			source = fmt.Sprintf("%s.%d", w.path, i-1)
		}
		if err := os.Rename(source, dest); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return w.open()
}

func (w *rotatingLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size+int64(len(p)) > w.limit {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	original := len(p)
	if int64(len(p)) > w.limit {
		p = p[:w.limit]
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		return n, err
	}
	return original, nil
}

func (w *rotatingLog) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Sync()
	closeErr := w.file.Close()
	w.file = nil
	if err != nil {
		return err
	}
	return closeErr
}
