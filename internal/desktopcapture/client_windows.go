//go:build windows

package desktopcapture

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// Client requests frames from the Desktop Helper running in the interactive
// user's session. The service keeps the WebSocket and authentication; the
// helper only captures pixels.
type Client struct {
	mu         sync.Mutex
	pipe       *os.File
	nextWake   time.Time
	openPipe   func() (*os.File, error)
	wakeHelper func() error
	now        func() time.Time
}

func New() *Client {
	return &Client{
		openPipe:   func() (*os.File, error) { return os.OpenFile(pipeName, os.O_RDWR, 0) },
		wakeHelper: startHelperTask,
		now:        time.Now,
	}
}

func (c *Client) CaptureScreenJPEG() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensurePipe(); err != nil {
		return nil, err
	}
	if _, err := c.pipe.Write([]byte{captureRequest}); err != nil {
		c.closePipe()
		return nil, fmt.Errorf("write desktop capture request: %w", err)
	}
	header := make([]byte, frameHeaderSize)
	if _, err := io.ReadFull(c.pipe, header); err != nil {
		c.closePipe()
		return nil, fmt.Errorf("read desktop capture response: %w", err)
	}
	length := getLength(header)
	if length <= 0 || length > maxFrameSize {
		c.closePipe()
		return nil, fmt.Errorf("desktop helper returned invalid frame length %d", length)
	}
	frame := make([]byte, length)
	if _, err := io.ReadFull(c.pipe, frame); err != nil {
		c.closePipe()
		return nil, fmt.Errorf("read desktop capture frame: %w", err)
	}
	return frame, nil
}

func (c *Client) ensurePipe() error {
	if c.pipe != nil {
		return nil
	}
	pipe, err := c.openPipe()
	if err != nil {
		// Busy/denied pipes do not mean the Helper is absent. Never start another
		// process for those failures. Streaming retries connection on its next tick.
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) && !c.now().Before(c.nextWake) {
			c.nextWake = c.now().Add(15 * time.Second)
			if wakeErr := c.wakeHelper(); wakeErr != nil {
				return fmt.Errorf("desktop helper unavailable: %w; start task: %v", err, wakeErr)
			}
		}
		return fmt.Errorf("desktop helper unavailable: %w", err)
	}
	c.pipe = pipe
	return nil
}

func (c *Client) closePipe() {
	if c.pipe != nil {
		_ = c.pipe.Close()
		c.pipe = nil
	}
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closePipe()
}
