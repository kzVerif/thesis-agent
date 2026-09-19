package service

import (
	"log"
	"os"
	"path/filepath"
)

// InitLogging writes diagnostic output to a file so the Agent can run without
// a visible UI. Registration prompts still use the console on first run.
func InitLogging() (func(), error) {
	path := os.Getenv("AGENT_LOG_PATH")
	if path == "" {
		path = filepath.Join("logs", "agent.log")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lmicroseconds)
	log.SetOutput(file)
	log.Printf("agent starting")
	return func() {
		log.Printf("agent stopping")
		_ = file.Close()
	}, nil
}
