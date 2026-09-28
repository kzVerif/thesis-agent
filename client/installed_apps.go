package client

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"ws-agent/installedapps"
)

const (
	installedAppsQueueSize         = 4
	installedAppsResultQueueSize   = 8
	installedAppsCollectionTimeout = 10 * time.Second
	installedAppsWriteTimeout      = 5 * time.Second
	installedAppsMaxCommandBytes   = 1024
)

type installedAppsCommand struct {
	Type      string `json:"type"`
	Action    string `json:"action"`
	RequestID string `json:"request_id"`
	deadline  time.Time
}
type installedAppsResult struct {
	Type      string               `json:"type"`
	Action    string               `json:"action"`
	RequestID string               `json:"request_id"`
	Success   bool                 `json:"success"`
	Apps      *[]installedapps.App `json:"apps,omitempty"`
	Error     string               `json:"error,omitempty"`
}

func parseInstalledAppsCommand(data []byte) (installedAppsCommand, bool) {
	var command installedAppsCommand
	err := json.Unmarshal(data, &command)
	return command, err == nil && len(data) <= installedAppsMaxCommandBytes && command.Type == "installed_apps" && command.Action == "get" && len(command.RequestID) == 36 && uuid.Validate(command.RequestID) == nil
}

// A connection owns one collector and one writer. Queues and duplicate tracking
// are bounded. Inventory is never queued for replay on a replacement socket.
type installedAppsWorker struct {
	jobs    chan installedAppsCommand
	results chan installedAppsResult
	mu      sync.Mutex
	pending map[string]bool
}

func newInstalledAppsWorker() *installedAppsWorker {
	return &installedAppsWorker{jobs: make(chan installedAppsCommand, installedAppsQueueSize), results: make(chan installedAppsResult, installedAppsResultQueueSize), pending: make(map[string]bool)}
}
func appsFailure(id, reason string) installedAppsResult {
	return installedAppsResult{Type: "installed_apps", Action: "result", RequestID: id, Error: reason}
}
func (w *installedAppsWorker) submit(command installedAppsCommand) {
	command.deadline = time.Now().Add(installedAppsCollectionTimeout)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending[command.RequestID] {
		return
	}
	select {
	case w.jobs <- command:
		w.pending[command.RequestID] = true
	default:
		// An abusive peer cannot block the WebSocket reader or create goroutines.
		select {
		case w.results <- appsFailure(command.RequestID, "collection busy"):
		default:
		}
	}
}
func (w *installedAppsWorker) collect(ctx context.Context, collect func(context.Context) ([]installedapps.App, error)) {
	for {
		select {
		case <-ctx.Done():
			return
		case command := <-w.jobs:
			if ctx.Err() != nil {
				return
			}
			jobCtx, cancel := context.WithDeadline(ctx, command.deadline)
			var apps []installedapps.App
			err := jobCtx.Err()
			if err == nil {
				apps, err = collect(jobCtx)
			}
			if err == nil {
				err = jobCtx.Err()
			}
			cancel()
			r := appsFailure(command.RequestID, "collection failed")
			if err == nil {
				if apps == nil {
					apps = make([]installedapps.App, 0)
				}
				r.Success, r.Error, r.Apps = true, "", &apps
				// Marshal the exact result (including [] for an empty inventory).
				if data, e := json.Marshal(r); e != nil || len(data) > installedapps.MaxPayload {
					r = appsFailure(command.RequestID, "inventory exceeds limit")
				}
			}
			select {
			case w.results <- r:
			case <-ctx.Done():
				return
			}
		}
	}
}
func (w *installedAppsWorker) write(ctx context.Context, send func(context.Context, any) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case r := <-w.results:
			writeCtx, cancel := context.WithTimeout(ctx, installedAppsWriteTimeout)
			_ = send(writeCtx, r)
			cancel()
			w.mu.Lock()
			delete(w.pending, r.RequestID)
			w.mu.Unlock()
		}
	}
}
