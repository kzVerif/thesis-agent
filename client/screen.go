package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const screenFrameInterval = 200 * time.Millisecond

type ScreenCapture func() ([]byte, error)

// ScreenStreamer owns the lifecycle of the single screen-capture goroutine.
type ScreenStreamer struct {
	mu        sync.Mutex
	streaming bool
	cancel    context.CancelFunc
	done      chan struct{}
	capture   ScreenCapture
}

func NewScreenStreamer(capture ScreenCapture) *ScreenStreamer {
	return &ScreenStreamer{capture: capture}
}

// Start is idempotent. Capture and network writes are sequential, so a slow
// connection cannot build an unbounded frame queue (latest opportunity wins).
func (s *ScreenStreamer) Start(ctx context.Context, conn *safeConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streaming || s.capture == nil {
		return
	}

	streamCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.streaming = true
	s.cancel = cancel
	s.done = done
	go s.run(streamCtx, conn, done)
}

func (s *ScreenStreamer) run(ctx context.Context, conn *safeConnection, done chan struct{}) {
	defer func() {
		s.mu.Lock()
		if s.done == done {
			s.streaming = false
			s.cancel = nil
			s.done = nil
		}
		s.mu.Unlock()
		close(done)
	}()

	ticker := time.NewTicker(screenFrameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			frame, err := s.capture()
			if err != nil {
				fmt.Println("Capture screen failed:", err)
				continue
			}
			if err := conn.write(ctx, websocket.MessageBinary, frame); err != nil {
				fmt.Println("Send screen frame failed:", err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// Stop cancels capture/encoding/writes and waits for their goroutine to exit.
func (s *ScreenStreamer) Stop() {
	s.mu.Lock()
	if !s.streaming {
		s.mu.Unlock()
		return
	}
	cancel, done := s.cancel, s.done
	s.mu.Unlock()

	cancel()
	<-done
}

func (s *ScreenStreamer) IsStreaming() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streaming
}
